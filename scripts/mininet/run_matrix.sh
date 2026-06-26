#!/usr/bin/env bash
#
# run_matrix.sh — Matriz experimental completa
#
# 3 políticas × 2 ABR × 2 clientes × 2 FoV mix × 4 cenários × 5 reps = 480 runs
#
# Uso:
#   ./run_matrix.sh [--reps N] [--sbw N] [--cbw N] [--parallelism N] [--dry-run] <IP>
#
# --reps N          Repetições por config (padrão: 5)
# --sbw N           Bandwidth do servidor em Mbps (padrão: 60)
# --cbw N           Bandwidth do cliente em Mbps (padrão: 60)
# --parallelism N   Paralelismo interno do cliente (padrão: 120)
# --dry-run         Mostra o que seria executado sem rodar
#
# Saída: logs/matrix-NNN/<label>/rep<N>/
#   experiment.env  — parâmetros da run
#   statistics-*.csv, statistics-summary-*.csv  — dados do cliente (1 por cliente)
#   server_summary.csv, class_agg.csv  — dados do servidor

set -euo pipefail

PROGRAM_NAME=$0
show_usage() {
    echo "Uso: $PROGRAM_NAME [OPTIONS] <IP>"
    echo "  --reps N          Repetições por config (padrão: 5)"
    echo "  --sbw N           Bandwidth servidor Mbps (padrão: 60)"
    echo "  --cbw N           Bandwidth cliente Mbps (padrão: 60)"
    echo "  --parallelism N   Paralelismo do cliente (padrão: 120)"
    echo "  --dry-run         Mostra runs sem executar"
}

IP=
REPS=5
SERVER_BW=60
CLIENT_BW=60
PARALLELISM=120
DRY_RUN=0

while [[ "$#" -gt 0 ]]; do
    case "$1" in
    --reps)        REPS="$2"        ; shift 2 ;;
    --sbw)         SERVER_BW="$2"   ; shift 2 ;;
    --cbw)         CLIENT_BW="$2"   ; shift 2 ;;
    --parallelism) PARALLELISM="$2" ; shift 2 ;;
    --dry-run)     DRY_RUN=1        ; shift   ;;
    -h|--help)     show_usage ; exit 0 ;;
    -*)            show_usage ; exit 1 ;;
    *)             IP="$1"          ; shift   ;;
    esac
done

if [[ -z "$IP" ]]; then show_usage ; exit 1; fi

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

# Diretório de saída com auto-incremento
LOG_NUMBER=1
while true; do
    MATRIX_DIR=$(printf "../../logs/matrix-%03d" "$LOG_NUMBER")
    [[ ! -e "$MATRIX_DIR" ]] && break
    LOG_NUMBER=$((LOG_NUMBER + 1))
done
mkdir -p "$MATRIX_DIR"

PURPLE='\033[0;35m'
GREEN='\033[0;32m'
NC='\033[0m'

echo -e "${PURPLE}Saída: $(cd "$MATRIX_DIR" && pwd)${NC}"

# ─── Fatores experimentais (Tabela 6 do professor) ───────────────────────────
#
# Cenários de rede (Tabela 5): apenas #1 #2 #5 #6
# formato: id|delay_ms|load_pct
SCENARIOS=(
    "1|24|10"   # Still-working, 10% background
    "2|24|30"   # Still-working, 30% background
    "5|10|10"   # Instantaneous, 10% background
    "6|10|30"   # Instantaneous, 30% background
)

POLICIES=(fifo sp wfq)
ABRS=(bola legacy)
CLIENTS_LIST=(1 6)
MIXES=(balanced wide_heavy)

# ─── Contagem total ───────────────────────────────────────────────────────────
n_scenarios=${#SCENARIOS[@]}
n_policies=${#POLICIES[@]}
n_abrs=${#ABRS[@]}
n_clients=${#CLIENTS_LIST[@]}
n_mixes=${#MIXES[@]}
TOTAL=$(( n_scenarios * n_policies * n_abrs * n_clients * n_mixes * REPS ))
echo -e "${PURPLE}Total de runs: $TOTAL (${n_scenarios} cenários × ${n_policies} políticas × ${n_abrs} ABR × ${n_clients} clientes × ${n_mixes} mixes × ${REPS} reps)${NC}"

if [[ "$DRY_RUN" == 1 ]]; then
    echo ""
    echo "=== DRY RUN — configurações que seriam executadas ==="
fi

run_idx=0
fail_count=0

for scenario_spec in "${SCENARIOS[@]}"; do
    IFS='|' read -r SC_ID SC_DELAY SC_LOAD <<< "$scenario_spec"

    for POLICY in "${POLICIES[@]}"; do
        for ABR in "${ABRS[@]}"; do
            for NC in "${CLIENTS_LIST[@]}"; do
                for MIX in "${MIXES[@]}"; do
                    for rep in $(seq 1 "$REPS"); do
                        run_idx=$((run_idx + 1))
                        label="s${SC_ID}_${POLICY}_${ABR}_${NC}c_${MIX}"
                        run_dir="$MATRIX_DIR/$label/rep${rep}"

                        echo -e "\n${PURPLE}=== [$run_idx/$TOTAL] $label rep${rep} ===${NC}"

                        if [[ "$DRY_RUN" == 1 ]]; then
                            echo "  policy=$POLICY abr=$ABR clients=$NC mix=$MIX"
                            echo "  delay=${SC_DELAY}ms load=${SC_LOAD}% rep=$rep"
                            continue
                        fi

                        mkdir -p "$run_dir"

                        # Primeiro run faz o build; os demais reutilizam o binário
                        NO_BUILD_FLAG=""
                        [[ "$run_idx" -gt 1 ]] && NO_BUILD_FLAG="--no-build"

                        set +e
                        ./server_scheduler_test.sh \
                            "--${POLICY}" \
                            --abr      "$ABR" \
                            --sbw      "$SERVER_BW" \
                            --cbw      "$CLIENT_BW" \
                            --delay    "$SC_DELAY" \
                            --load     "$SC_LOAD" \
                            --loss     0 \
                            --clients  "$NC" \
                            --fov-mix  "$MIX" \
                            -p         "$PARALLELISM" \
                            -o         "$run_dir" \
                            $NO_BUILD_FLAG \
                            "$IP"
                        RC=$?
                        set -e

                        if [[ "$RC" != 0 ]]; then
                            fail_count=$((fail_count + 1))
                            echo -e "${PURPLE}[WARN] run $run_idx falhou (rc=$RC) — continuando${NC}"
                        else
                            echo -e "${GREEN}[OK] $label rep${rep}${NC}"
                        fi

                        # Adiciona metadados de matriz ao experiment.env
                        cat >> "$run_dir/experiment.env" <<EOF
scenario_id=$SC_ID
policy=$POLICY
abr=$ABR
num_clients=$NC
fov_mix=$MIX
rep=$rep
matrix_run=$run_idx
EOF

                    done
                done
            done
        done
    done
done

echo ""
if [[ "$DRY_RUN" == 1 ]]; then
    echo "=== DRY RUN concluído: $run_idx configurações listadas ==="
else
    echo -e "${PURPLE}=== Matriz concluída: $run_idx runs, $fail_count falhas — $(cd "$MATRIX_DIR" && pwd) ===${NC}"
fi
