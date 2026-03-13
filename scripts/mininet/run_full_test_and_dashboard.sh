#!/usr/bin/env bash
#
# Roda a matriz de testes (políticas × redes) no Mininet e gera o dashboard.
# Uso: ./run_full_test_and_dashboard.sh [--open] <IP_MININET>
#
# Exemplo: ./run_full_test_and_dashboard.sh --open 192.168.0.111
#
# Testes: WFQ dinâmico em 4 cenários reais:
# Rede #1 / Rede #6 × FoV narrow / FoV wide.
# Os CSVs vão para logs/server_scheduler_test/. O dashboard é gerado a partir dessa pasta.
#

set -e
SCRIPT_DIR="$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
IP=""
OPEN_DASHBOARD=""

while [[ "$#" -gt 0 ]]; do
    case "$1" in
        --open) OPEN_DASHBOARD=1 ; shift ;;
        -*)     echo "Opção desconhecida: $1" ; exit 1 ;;
        *)      IP="$1" ; shift ;;
    esac
done

if [[ -z "$IP" ]]; then
    echo "Uso: $0 [--open] <IP_MININET>"
    echo "  --open   Abre o dashboard no navegador ao final"
    echo "  IP       IP do host Mininet (ex.: 192.168.0.111)"
    exit 1
fi

# Remove /24 ou outro sufixo se o usuário passou CIDR
IP="${IP%%/*}"

echo "=============================================="
echo " Teste completo + Dashboard"
echo " Mininet IP: $IP"
echo "=============================================="

cd "$SCRIPT_DIR"

# Pastas com nomes que o dashboard entende: net1/net6 (Rede) e narrow/wide (FoV).
LOG_BASE="$PROJECT_ROOT/logs/server_scheduler_test"
mkdir -p "$LOG_BASE"

# Limpa apenas os runs-alvo para evitar misturar resultados antigos com o dashboard novo.
rm -rf "$LOG_BASE/wfq_net1_narrow" "$LOG_BASE/wfq_net1_wide" "$LOG_BASE/wfq_net6_narrow" "$LOG_BASE/wfq_net6_wide"

# WFQ dinâmico — 4 cenários reais (rede × FoV)
echo ""
echo "[1/4] WFQ (dinâmico) - Rede #1 - FoV narrow"
./server_scheduler_test.sh --wfq --fov narrow --loss 10 --delay 24 -o "$LOG_BASE/wfq_net1_narrow" "$IP"

echo ""
echo "[2/4] WFQ (dinâmico) - Rede #1 - FoV wide"
./server_scheduler_test.sh --wfq --fov wide --loss 10 --delay 24 -o "$LOG_BASE/wfq_net1_wide" "$IP"

echo ""
echo "[3/4] WFQ (dinâmico) - Rede #6 - FoV narrow"
./server_scheduler_test.sh --wfq --fov narrow --loss 30 --delay 10 -o "$LOG_BASE/wfq_net6_narrow" "$IP"

echo ""
echo "[4/4] WFQ (dinâmico) - Rede #6 - FoV wide"
./server_scheduler_test.sh --wfq --fov wide --loss 30 --delay 10 -o "$LOG_BASE/wfq_net6_wide" "$IP"

# Gerar dashboard a partir de logs/server_scheduler_test
echo ""
echo "=============================================="
echo " Gerando dashboard..."
echo "=============================================="
cd "$PROJECT_ROOT"
if [[ ! -d "$LOG_BASE" ]]; then
    echo "Aviso: pasta $LOG_BASE não encontrada. Dashboard não gerado."
    exit 1
fi
python dashboard.py --base-dir "$LOG_BASE" --output "$PROJECT_ROOT/dashboard.html"
echo "Dashboard salvo em: $PROJECT_ROOT/dashboard.html"

if [[ -n "$OPEN_DASHBOARD" ]]; then
    if command -v xdg-open &>/dev/null; then
        xdg-open "$PROJECT_ROOT/dashboard.html"
    elif command -v open &>/dev/null; then
        open "$PROJECT_ROOT/dashboard.html"
    elif command -v start &>/dev/null; then
        start "$PROJECT_ROOT/dashboard.html"
    else
        echo "Abra manualmente: dashboard.html"
    fi
fi

echo ""
echo "Concluído. Logs em: $LOG_BASE"
