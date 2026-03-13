# WFQ com Pesos Dinâmicos

## O Problema

O WFQ (Weighted Fair Queuing) distribui banda entre filas proporcionalmente aos seus pesos.
Hoje os pesos são **fixos**: HIGH=3, MED=2, LOW=1.

Isso funciona bem quando a carga é estável. Mas quando a proporção de tiles muda
(ex: muitos tiles FoV de uma vez), os pesos fixos podem causar **priority inversion** —
a fila menos importante entrega melhor do que a mais importante.

## A Solução

Ajustar os pesos **automaticamente** com base no tráfego real observado.

### Quando recalcular

**A cada rounding** — ou seja, toda vez que o escalonador tira um item da fila (`pickWFQ`).

### Como recalcular (5 passos)

```
1. MEDIR     x_p = bytes_da_fila / total_bytes
              → Qual fração do tráfego cada fila está realmente usando?

2. CANDIDATO  φ_cand = K · x_p^β · φ_atual
              → Propor um novo "share" baseado no tráfego real

3. CLAMP      φ_cand = min(0.95, max(0.05, φ_cand))
              → Garantir que nenhuma fila zere ou domine

4. SUAVIZAR   φ_novo = 0.7 · φ_atual + 0.3 · φ_cand
              → Evitar mudanças bruscas

5. CONVERTER  W_p = (φ_novo / (1 - φ_novo)) · W_outras
              → Traduzir o share de volta em peso para o WFQ
```

### Parâmetros

| Parâmetro | Valor | Função |
|:---:|:---:|---|
| K | 1.0 | Ganho do ajuste |
| β | 1.0 | Sensibilidade ao workload |
| α | 0.3 | Suavização (EMA) |
| ε_min | 0.05 | Share mínimo por fila |
| ε_max | 0.05 | Headroom máximo |

### Exemplo prático

```
Pesos iniciais: HIGH=3, LOW=1 → φ_high = 3/4 = 0.75

Num segundo, o servidor transmitiu:
  HIGH: 900KB, LOW: 100KB → x_high = 0.90

Candidato: 1.0 × 0.90 × 0.75 = 0.675
Clamp:     0.675 (ok, entre 0.05 e 0.95)
Suavizar:  0.7 × 0.75 + 0.3 × 0.675 = 0.728
Converter: W_high = (0.728 / 0.272) × 1 = 2.68

Resultado: peso de HIGH baixou de 3.0 → 2.68
           (porque HIGH estava "comendo" banda demais)
```

### O que muda no código

| Arquivo | Mudança |
|---|---|
| `task_scheduler.go` | Contador de bytes por fila + função `recalcWFQWeights()` |
| `task_scheduler.go` | `pickWFQ()` chama `recalcWFQWeights()` antes de cada dequeue |
| `stream_handler.go` | Informa bytes enviados ao scheduler |
| `metrics/wfq_utilization.go` | Coluna `adapted` no CSV |

### O que NÃO muda

- O mecanismo interno do WFQ (`scheduler/wfq_scheduler.go`) — continua com virtual finish time
- Outras políticas (FIFO, SP, VoI_SP) — não são afetadas
