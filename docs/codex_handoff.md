# Handoff do Codex — ABR Legacy

Última atualização: 2026-06-26, America/Manaus

## Finalidade

Este é o contexto operacional que deve ser lido no início de uma nova sessão do Codex neste repositório. O escopo do ciclo descrito aqui foi exclusivamente o ABR Legacy. As regras permanentes continuam em `AGENTS.md`.

## Estado atual

O estado mais recente do ABR Legacy é **Throughput-Based**. O buffer Legacy continua sendo calculado, registrado e validado como métrica contígua, mas não gateia mais as decisões `LOW`, `MED` e `HIGH`.

A série reproduzível aceita para essa política é:

- `logs/legacy-validation-009/`

A série `logs/legacy-validation-008/` permanece como baseline histórica aceita do ciclo anterior buffer-aware, mas não deve ser usada como aceite da política Throughput-Based.

Não foi feito commit, push, checkout ou descarte de alterações neste ciclo Throughput-Based. O workspace já continha mudanças antes e durante este trabalho; preserve tudo que não estiver inequivocamente dentro da tarefa.

Não há processo de validação, SSH, SCP ou runner conhecido como ativo ao final do ciclo.

## Atualização Throughput-Based

Arquivos principais alterados neste ciclo:

- `src/test_client/session/abr.go`
- `src/test_client/session/abr_test.go`
- `src/test_client/session/legacy_debug_test.go`
- `scripts/mininet/resources/analyze_legacy_validation.py`
- `scripts/mininet/resources/test_analyze_legacy_validation.py`

Semântica atual:

- `AvgThroughput` continua vindo da EWMA agregada por segmento completo em `src/test_client/netstats`;
- `thresholds(ctx)` continua calculando o custo real da configuração espacial FoV/Near-FoV/background;
- `LOW` se throughput for inválido/zero ou `< threshold_med * 1.05`;
- `MED` se `threshold_med * 1.05 <= throughput < threshold_high * 1.10`;
- `HIGH` se `throughput >= threshold_high * 1.10`;
- `HIGH` promove Near-FoV para `MED`;
- `MED` mantém Near-FoV em `LOW`;
- background permanece `LOW`;
- entradas inválidas (`NaN`, `Inf`, throughput `<=0`, thresholds inválidos) são tratadas conservadoramente no Go e no analisador Python.

O analisador Legacy foi atualizado para validar a regra throughput-based e para reprovar thresholds inválidos. Throughput inválido/zero só é compatível com tier `LOW`.

## Validação aceita da série 009

Runner executado:

```bash
./scripts/mininet/run_legacy_validation.sh 192.168.56.101
```

Resultado:

```text
PASS: Legacy validation criteria satisfied
Legacy validation artifacts: /home/lucas/projects/TccQuic/logs/legacy-validation-009
```

O validador independente revisou a série `009` em modo somente leitura e aprovou sem bloqueadores.

Resumo:

| Cenário | LOW | MED | HIGH | Streak MED máx. | Buffer | Prioridade espacial |
|---|---:|---:|---:|---:|---:|---:|
| good | 2 | 0 | 58 | 0 | 0–2 s | 100% |
| medium | 32 | 22 | 6 | 14 | 0–2 s | 100% |
| bad | 60 | 0 | 0 | 0 | 0 s | 100% |

Delivery:

- good: completion `100%`, on-time `100%`, deadline miss `0%`;
- medium: completion `100%`, on-time `100%`, deadline miss `0%`;
- bad: `5094/5157` tiles completos;
- bad: `5072/5157` tiles on-time;
- bad: `56/60` segmentos completos;
- bad: deadline miss `1.648%`.

Artefatos principais:

- `logs/legacy-validation-009/legacy_validation_summary.csv`
- `logs/legacy-validation-009/legacy_segment_metrics.csv`
- `logs/legacy-validation-009/legacy_timeline.png`
- `logs/legacy-validation-009/legacy_spatial_quality.png`
- `logs/legacy-validation-009/legacy_delivery_timeline.png`

Cada subdiretório `good/`, `medium/` e `bad/` preserva `command.txt`, `experiment.env`, `stdout`, `legacy-decisions.csv` e CSVs brutos necessários para reprodução e auditoria.

## Testes aprovados no ciclo Throughput-Based

```bash
go test -count=1 ./src/test_client/session
go test -count=1 ./src/test_client/netstats
python3 -m unittest scripts/mininet/resources/test_analyze_legacy_validation.py
go test -count=1 ./...
python3 scripts/mininet/resources/analyze_legacy_validation.py logs/legacy-validation-009
git diff --check
```

O comando do analisador emitiu apenas aviso do Matplotlib sobre cache não gravável em `/home/lucas/.config/matplotlib`, usou `/tmp` e passou.

## Problemas confirmados e corrigidos

### 1. Semântica e amostragem do buffer Legacy

O buffer antigo podia usar estado obsoleto/não contíguo e, em uma tentativa intermediária, exigia que todos os 169 tiles estivessem on-time para considerar a rodada no buffer. Isso zerava o buffer mesmo quando a rodada havia terminado e tornava a política degenerada.

A correção usa o prefixo contíguo de rodadas concluídas para representar o buffer útil. Misses e atrasos de tiles continuam contabilizados separadamente nas métricas de delivery.

Implementação e cobertura principais:

- `src/test_client/session/legacy_buffer.go`
- `src/test_client/session/legacy_buffer_test.go`
- integração em `src/test_client/session/`

### 2. Calibração LOW/MED/HIGH

Os limiares anteriores eram incompatíveis com a EWMA observada e produziam comportamento binário ou sempre LOW. A política foi recalibrada somente depois da correção do buffer.

O cenário intermediário final usa no runner Legacy-only:

- bandwidth: `145 Mbps`
- loss: `1.8%`
- delay: `14 ms`
- background load: `5%`
- base latency: `335 ms`

Esses parâmetros foram escolhidos por calibração reproduzível, não para mascarar falha da política. A série final contém LOW, MED e HIGH com sequências sustentadas.

### 3. Prioridade espacial

Em empate de qualidade, a ordem Legacy agora preserva FoV e coloca Near-FoV antes do background.

Essa ordenação foi isolada do BOLA. O teste discriminante fixa:

- BOLA histórico: ordem `[5,1,3]`
- Legacy: ordem `[5,3,1]`

A prioridade espacial medida foi `100%` nos três cenários finais.

### 4. Eixo tile/segment e isolamento do BOLA

Foi confirmado que a mídia Legacy usa nomes `track<TILE>_<SEGMENT>`. Uma correção inicial aplicou esse layout globalmente e foi reprovada porque alterava rotas compartilhadas com o BOLA.

A solução final separa explicitamente os layouts:

- BOLA preserva o layout histórico `track<SEGMENT>_<TILE>`;
- Legacy/default/threshold usa `track<TILE>_<SEGMENT>`;
- o wire BOLA permanece byte-a-byte compatível e sem o novo cabeçalho;
- requisições Legacy sinalizam `TileFirstLayout=true`;
- `EstimateTileSize`, `readFile`, media bounds, IDs e universo de tiles escolhem o layout correto por modo.

Cobertura discriminante principal:

- `src/model/video-packet_test.go`: wire BOLA e lookup dos dois layouts;
- `src/server/stream_handler/stream_handler_layout_test.go`: leitura dos dois layouts;
- `src/test_client/media_bounds_test.go`: bounds, IDs, universo e reconhecimento dos modos;
- testes de scheduler/ABR para ordenação BOLA versus Legacy.

O validador independente confirmou que o bloqueador de não regressão BOLA foi resolvido.

### 5. Instrumentação e análise

Foram adicionados/atualizados:

- `scripts/mininet/run_legacy_validation.sh`
- `scripts/mininet/resources/analyze_legacy_validation.py`
- `scripts/mininet/resources/test_analyze_legacy_validation.py`
- instrumentação Legacy em `src/test_client/session/legacy_debug.go`
- testes em `src/test_client/session/legacy_debug_test.go`

O analisador final exige, entre outros pontos:

- 60 decisões por cenário;
- presença de LOW, MED e HIGH no cenário intermediário;
- sequência MED sustentada;
- comportamento favorável/intermediário/degradado coerente e monotônico;
- prioridade espacial integral;
- reconciliação de completion, on-time e deadline miss.

## Execuções e decisões de aceite

As séries `001`–`003` foram incompletas por uma estratégia de transferência ineficiente e não são evidência válida.

A série `004` foi reprovada porque o buffer exigia 169/169 tiles on-time e fazia medium/bad ficarem degenerados.

A série `005` foi reprovada pelo validador por inversão tile/segment, possível vazamento da ordenação para BOLA, critérios frouxos do analisador e ambiguidade de completion.

A série `006` foi reprovada porque o cenário medium não sustentava MED por pelo menos três decisões.

A série `007` passou nas métricas Legacy, mas foi reprovada porque a correção tile-first ainda vazava por rotas compartilhadas do BOLA.

A série **`008` é a única candidata final aceita**. Ela foi revisada independentemente em modo somente leitura e recebeu PASS sem bloqueadores.

## Resultados finais da série 008

Cada cenário contém 60 decisões e 5.157 tiles.

| Cenário | LOW | MED | HIGH | Streak MED máx. | Buffer | Prioridade espacial |
|---|---:|---:|---:|---:|---:|---:|
| good | 3 | 1 | 56 | 1 | 0–2 s | 100% |
| medium | 8 | 22 | 30 | 6 | 0–2 s | 100% |
| bad | 60 | 0 | 0 | 0 | 0–1 s | 100% |

Delivery:

- good: completion `100%`, on-time `100%`, deadline miss `0%`;
- medium: completion `100%`, on-time `100%`, deadline miss `0%`;
- bad: `5156/5157` tiles completos, `5154/5157` on-time, 3 misses;
- bad: 2 tiles completos atrasados e 1 tile não completo;
- bad: `59/60` segmentos completos;
- bad: completion por tile `99.9806%`, on-time `99.9418%`, deadline miss `0.0582%`.

O cenário medium não é binário nem degenerado. O cenário bad rebaixa integralmente para LOW e registra misses reais, coerentes com a condição degradada.

## Artefatos aceitos

Diretório principal:

- `logs/legacy-validation-008/`

Agregados:

- `logs/legacy-validation-008/legacy_validation_summary.csv`
- `logs/legacy-validation-008/legacy_segment_metrics.csv`

Gráficos completos e inspecionados visualmente:

- `logs/legacy-validation-008/legacy_timeline.png`
- `logs/legacy-validation-008/legacy_spatial_quality.png`
- `logs/legacy-validation-008/legacy_delivery_timeline.png`

Cada subdiretório `good/`, `medium/` e `bad/` preserva `command.txt`, `experiment.env`, `stdout`, `legacy-decisions.csv` e os CSVs brutos de estatísticas/delivery necessários para reprodução e auditoria.

O validador reconciliou os agregados com os dados brutos sem divergências, duplicatas, skips ou timeouts.

## Testes finais aprovados

Os seguintes comandos passaram após o isolamento final entre Legacy e BOLA:

```bash
go test -count=1 ./src/test_client/session
go test -count=1 ./src/test_client/netstats
python3 -m unittest scripts/mininet/resources/test_analyze_legacy_validation.py
go test -count=1 ./...
```

Também passaram os pacotes diretamente afetados em `src/model`, `src/server/stream_handler` e `src/test_client`.

## Como reproduzir a validação Legacy

Pré-requisito: VM Mininet acessível por SSH em `192.168.56.101` com o ambiente esperado pelos scripts.

Runner:

```bash
./scripts/mininet/run_legacy_validation.sh 192.168.56.101
```

O runner cria uma nova pasta numerada em `logs/legacy-validation-NNN`, preserva comandos e ambiente e gera CSVs, resumo e gráficos. Nunca sobrescreva nem trate as séries reprovadas como aceitas.

## Próximo passo seguro

O objetivo solicitado neste ciclo está concluído. Em uma nova sessão:

1. leia `AGENTS.md` e este handoff integralmente;
2. confira `git status --short` antes de qualquer edição;
3. trate `legacy-validation-009` como baseline aceita da política Throughput-Based;
4. não altere o Legacy ou BOLA sem uma nova solicitação concreta;
5. trate `legacy-validation-008` apenas como baseline histórica buffer-aware;
6. se houver nova mudança de runtime, execute novamente testes focados, suíte completa e uma série Legacy-only completa, sem reutilizar os números `008` ou `009`;
7. submeta qualquer nova candidata a validação independente antes de declarar conclusão;
8. atualize este arquivo ao final do novo ciclo.

## Documentação histórica

`docs/abr_bola_andamento.md` contém o histórico anterior sobre BOLA, Qmax e Near-FoV, mas está datado de 2026-06-16 e não substitui este handoff para o estado Legacy atual.
