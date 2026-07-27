# TccQuic — Streaming de Vídeo Imersivo sobre QUIC

Sistema cliente-servidor para entrega de vídeo em mosaico (_tiled video_) sobre o protocolo QUIC, com escalonamento semântico no servidor. Compara as políticas de fila **FIFO**, **Prioridade Estrita (SP)** e **WFQ com pesos dinâmicos**, além de variantes com descarte por **Valor da Informação (VoI)**.

> 📖 **Para rodar os experimentos de ponta a ponta** (do `git clone` à emulação de rede com Mininet e à geração das figuras), veja o **[COMO_RODAR.md](COMO_RODAR.md)** — inclui instruções para Linux e Windows e como criar o host Mininet do zero.

## Requisitos

- **Go 1.19.x** (obrigatório — a biblioteca `quic-go v0.31` não é compatível com versões superiores)
- Repositório clonado: `https://github.com/quic-streaming/tcc`

## Como Executar

```bash
# Servidor (escolha a política de fila)
go run main.go server fifo
go run main.go server sp
go run main.go server wfq
go run main.go server voi_sp
go run main.go server voi_wfq

# Cliente de teste
go run main.go test-client <ip> <paralelismo> <latência_base_ms>
# Exemplo:
go run main.go test-client localhost 128 250
```

> ⚠️ Requer **Go 1.19.x** — o `quic-go v0.31` não compila em Go ≥ 1.20.

---

## Experimentos em rede emulada (Mininet)

Os resultados do TCC vêm de experimentos em rede emulada com **Mininet**, orquestrados
pelos scripts em `scripts/mininet/`. O fluxo: os scripts cross-compilam o binário,
enviam por SSH a um host Mininet e coletam os CSVs em `logs/`.

```bash
cd scripts/mininet

# Um experimento
./server_scheduler_test.sh --wfq --abr bola --sbw 40 --delay 10 --load 30 \
                           --clients 6 --fov-mix balanced <IP_DO_HOST_MININET>

# Matriz completa (480 execuções — os dados da monografia)
./run_matrix.sh --sbw 40 --reps 5 <IP_DO_HOST_MININET>
```

Análise e figuras (em `scripts/mininet/resources/`):

```bash
python plot_paper_v2.py logs/matrix-014 -o ./figuras_saida   # figuras estilo-paper
python regen_doc_figs.py                                      # figuras da monografia
```

O passo a passo detalhado (host Mininet, SSH, Linux/Windows) está no **[COMO_RODAR.md](COMO_RODAR.md)**.

---

## Arquitetura Geral

![Arquitetura do sistema cliente-servidor QUIC](docs/diagrams/architecture_overview.png)

O **cliente** solicita tiles de vídeo organizados por segmento e tile. O **servidor** recebe as requisições, coloca na fila do escalonador e responde com os dados do tile lido do disco.

---

## Estrutura do Projeto

```
TccQuic/
├── main.go                          # Ponto de entrada
├── src/
│   ├── model/                       # Modelos compartilhados
│   │   ├── constants.go             # Priority, Bitrate
│   │   ├── video-packet.go          # Request/Response (serialização)
│   │   └── voi.go                   # Cálculo do Valor da Informação
│   ├── server/
│   │   ├── server.go                # Listener QUIC
│   │   ├── bandwidth/
│   │   │   └── tracker.go           # Estimativa de vazão
│   │   ├── stream_handler/
│   │   │   ├── stream_handler.go    # Orquestração de streams
│   │   │   ├── task_scheduler.go    # Escalonador (FIFO/SP/WFQ/VoI)
│   │   │   └── scheduler/           # Implementação genérica (legado)
│   │   └── metrics/                 # Coleta de métricas (CSVs)
│   ├── client/                      # Cliente real (reprodutor)
│   └── test_client/                 # Cliente de teste (simulação)
│       ├── test_client.go           # Loop principal de requisições
│       ├── client.go                # Conexão QUIC
│       ├── fov_trace.go             # Leitura de trace de FoV
│       └── playback_simulator.go    # Simulação de reprodução
├── data/                            # Vídeo em tiles (.m4s) + traces de FoV
└── scripts/mininet/                 # Experimentos em rede emulada
    ├── run_matrix.sh                # Matriz completa (480 runs)
    ├── server_scheduler_test.sh     # Um experimento
    ├── upload_ssh_key.sh            # Configura SSH sem senha
    ├── run_legacy_validation.sh     # Validação do ABR legacy
    └── resources/
        ├── server_scheduler_test.py # Runner do Mininet (topologia)
        ├── utils.py                 # Helpers de topologia
        ├── plot_paper_v2.py         # Figuras estilo-paper + camada de dados
        └── regen_doc_figs.py        # Regenera as figuras da monografia
```

---

## Fluxo do Cliente

![Fluxo de decisão do cliente de teste](docs/diagrams/client_flow.png)

Para cada segmento de vídeo, o cliente consulta o **trace de FoV** para identificar quais tiles estão no campo de visão. Tiles no FoV recebem prioridade alta e bitrate adaptativo; os demais recebem prioridade baixa.

---

## Fluxo do Servidor

![Fluxo de processamento do servidor](docs/diagrams/server_flow.png)

O servidor recebe requisições via QUIC, calcula o deadline e, dependendo da política, pode calcular o VoI antes de enfileirar. Se o VoI for negativo, o tile é descartado imediatamente.

---

## Políticas de Escalonamento

![Comparação das 5 políticas de escalonamento](docs/diagrams/scheduling_policies.png)

| Política | Comando | Descarte VoI | Ordenação |
|----------|---------|:---:|-----------|
| `fifo` | `server fifo` | ✗ | Ordem de chegada |
| `sp` | `server sp` | ✗ | HIGH → MED → LOW |
| `wfq` | `server wfq` | ✗ | Relógio virtual com pesos **dinâmicos** (âncora 3:2:1, recalculados por volume de bytes a cada rodada) |
| `voi_sp` | `server voi_sp` | ✓ | HIGH → MED → LOW |
| `voi_wfq` | `server voi_wfq` | ✓ | Round-robin com pesos semânticos |

---

## Valor da Informação (VoI)

![Fórmula e fluxo do VoI](docs/diagrams/voi_formula.png)

O VoI determina se vale a pena transmitir um tile, considerando sua importância perceptual e o estado atual da rede.

### Fórmula

```
VoI_i = α · p_i − λ · (μ_i + t_now − d_i)
```

### Parâmetros

| Símbolo | Significado | Valor padrão |
|---------|-------------|:---:|
| `p_i` | Prioridade semântica (FoV=1.0, Near=0.6, BG=0.2) | — |
| `α` | Peso da semântica | 10 |
| `λ` | Penalização por atraso | 0.01 |
| `μ_i` | Tempo estimado de entrega = tamanho / vazão × 1000 (ms) | — |
| `d_i` | Deadline absoluto do tile | — |

### Estimativa de Vazão (μ_i)

O servidor mantém um **rastreador de banda** (`bandwidth/tracker.go`) com janela deslizante de 2 segundos:

1. Cada tile enviado com sucesso registra `(timestamp, bytes)`
2. Amostras fora da janela são descartadas
3. `Vazão = Σ bytes / Δt`
4. `μ_i = tamanho_tile / vazão × 1000 ms`

---

## Métricas Geradas

### Lado Servidor (CSVs em `/tmp/server_scheduler_test/`)

| Arquivo | Conteúdo |
|---------|----------|
| `reqlog.csv` | Tempos por requisição (qd, svc, rsp), on-time, drop |
| `class_agg.csv` | Agregados por classe (bytes, delays, on-time ratio) |
| `queue_len.csv` | Tamanho das filas por classe (amostrado a cada 100ms) |
| `fairness.csv` | Índice de Jain entre classes (a cada 1s) |
| `wfq_utilization.csv` | Share real vs peso teórico por classe (a cada 1s) |
| `work_conserving.csv` | Tempo ocioso com fila não-vazia (a cada 1s) |
| `server_summary.csv` | Resumo final (shares, Jain, throughput, drops) |

### Lado Cliente

| Arquivo | Conteúdo |
|---------|----------|
| `statistics-<pid>.csv` | Detalhes por requisição |
| `statistics-summary-<pid>.csv` | Resumo (join latency, completion rate, stale bytes) |
| `fov-delivery-<pid>.csv` | Taxa de entrega FoV por segmento |
| `fov-goodput-<pid>.csv` | Goodput útil (FoV on-time) por janela |
