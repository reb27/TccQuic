package model

import (
	"math"
	"os"
	"strconv"
	"time"
)

// Prioridades semânticas pi conforme o documento (Valor da Informação):
// - Tiles no centro do FoV: pi = 1.0
// - Tiles em Near-FoV: pi = 0.6
// - Tiles em Background: pi = 0.2
// - Tiles irrelevantes ou fora do campo estimado: pi = 0
const (
	SemanticPiFoV        = 1.0
	SemanticPiNearFoV    = 0.6
	SemanticPiBackground = 0.2
	SemanticPiIrrelevant = 0.0
)

// WFQWeightScale é o fator de escala para φ(pi). Pesos inteiros ficam 10, 6, 2 para 1.0, 0.6, 0.2.
const WFQWeightScale = 10

// Parâmetros do VoI conforme o documento:
// - α: fator de escala da semântica (padrão 10)
// - λ: penalização por atraso (padrão 0.01)
// Podem ser configurados via variáveis de ambiente VOI_ALPHA e VOI_LAMBDA
var (
	VoIAlphaDefault  = getEnvFloat("VOI_ALPHA", 10.0)  // α: escala da semântica
	VoILambdaDefault = getEnvFloat("VOI_LAMBDA", 0.01) // λ: penalização por atraso (ms^-1)
)

// SemanticPiFromPriority mapeia a prioridade da fila (classe) para a prioridade semântica pi
// usada no documento: HIGH→FoV, MEDIUM→Near-FoV, LOW→Background.
func SemanticPiFromPriority(p Priority) float64 {
	switch p {
	case HIGH_PRIORITY:
		return SemanticPiFoV
	case MEDIUM_PRIORITY:
		return SemanticPiNearFoV
	case LOW_PRIORITY:
		return SemanticPiBackground
	default:
		return SemanticPiBackground
	}
}

// WFQWeightFromSemantic calcula o peso WFQ a partir da prioridade semântica pi:
// Wi = φ(pi). Função φ(pi) retorna inteiro >= 1 (evita peso zero).
// Documento: "WFQ: Wi = φ(pi) ou Wi = max(VoIi, 0)".
func WFQWeightFromSemantic(pi float64) int {
	if pi <= 0 {
		return 1
	}
	if pi > 1 {
		pi = 1
	}
	w := int(math.Round(float64(WFQWeightScale) * pi))
	if w < 1 {
		w = 1
	}
	return w
}

// WFQWeightFromVoI calcula o peso WFQ a partir do Valor da Informação
// Documento: Wi = max(VoIi, 0). VoI negativo → tile descartado; para WFQ usamos max(VoI, 0)
// e convertemos para inteiro (mínimo 1 para não zerar a participação da fila).
// VoIi = α·pi − λ·(μi + tnow − di)
func WFQWeightFromVoI(voi float64) int {
	if voi <= 0 {
		return 1
	}
	w := int(math.Round(voi))
	if w < 1 {
		w = 1
	}
	return w
}

// WFQWeightForClass retorna o peso WFQ da classe de prioridade p, usando φ(pi).
// Usado pelo scheduler para definir os pesos iniciais das filas.
func WFQWeightForClass(p Priority) int {
	pi := SemanticPiFromPriority(p)
	return WFQWeightFromSemantic(pi)
}

// SemanticPiFromRequest retorna a prioridade semântica pi da requisição.
// Se SemanticPriority > 0, usa esse valor; senão deriva de FoV (true→1.0, false→0.6).
// Útil para cálculo futuro de VoI por requisição.
func SemanticPiFromRequest(fov bool, semanticPriority float32) float64 {
	if semanticPriority > 0 {
		pi := float64(semanticPriority)
		if pi > 1 {
			pi = 1
		}
		return pi
	}
	if fov {
		return SemanticPiFoV
	}
	return SemanticPiNearFoV
}

// EstimateDeliveryTime calcula μi = tamanho / taxa (em ms).
func EstimateDeliveryTime(tileSizeBytes int64, throughputBytesPerSec float64) float64 {
	if tileSizeBytes <= 0 || throughputBytesPerSec <= 0 {
		return 100.0 // padrão se valores inválidos
	}
	return (float64(tileSizeBytes) / throughputBytesPerSec) * 1000.0
}

// CalculateVoI calcula o Valor da Informação (VoI) conforme o documento.
// VoIi = α·pi − λ·(μi + tnow − di)
// Parâmetros:
//   - pi: prioridade semântica (0..1)
//   - muI: tempo estimado de entrega em milissegundos
//   - tNow: tempo atual
//   - deadline: deadline absoluto do tile
//   - alpha: fator de escala da semântica (usa VoIAlphaDefault se <= 0)
//   - lambda: penalização por atraso em ms^-1 (usa VoILambdaDefault se <= 0)
//
// Retorna o VoI calculado. Valores negativos indicam que o tile não deve ser transmitido.
func CalculateVoI(pi float64, muI float64, tNow time.Time, deadline time.Time, alpha float64, lambda float64) float64 {
	if alpha <= 0 {
		alpha = VoIAlphaDefault
	}
	if lambda <= 0 {
		lambda = VoILambdaDefault
	}

	// Calcular diferença entre deadline e tempo atual (em ms)
	timeToDeadlineMs := deadline.Sub(tNow).Milliseconds()

	// VoI = α·pi − λ·(μi + tnow − di)
	// onde (tnow − di) = -timeToDeadlineMs (se deadline > tNow, timeToDeadlineMs > 0)
	voi := alpha*pi - lambda*(muI-float64(timeToDeadlineMs))

	return voi
}

// CalculateVoIForRequest calcula VoI para uma requisição.
func CalculateVoIForRequest(req *VideoPacketRequest, tNow time.Time, deadline time.Time, tileSizeBytes int64, throughputBytesPerSec float64) float64 {
	pi := SemanticPiFromRequest(req.FoV, req.SemanticPriority)
	muI := EstimateDeliveryTime(tileSizeBytes, throughputBytesPerSec)
	return CalculateVoI(pi, muI, tNow, deadline, 0, 0)
}

// getEnvFloat lê uma variável de ambiente como float64, ou retorna o padrão.
func getEnvFloat(key string, defaultValue float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultValue
}
