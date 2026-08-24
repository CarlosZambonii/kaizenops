package spc

import (
	"fmt"
	"math"
	"time"
)

// ValueAt é o valor bruto de uma observação (ex: duração de build em
// segundos), já em ordem temporal.
type ValueAt struct {
	Timestamp time.Time
	Value     float64
}

// CapabilityResult é o resultado do cálculo de capacidade de processo.
type CapabilityResult struct {
	Cp    float64 // NaN se não computável (falta um dos limites de spec).
	Cpk   float64
	Mean  float64
	Sigma float64
}

// capabilityD2ForN2 é a constante clássica d2 para subgrupo de n=2, usada
// para estimar sigma a partir da moving range de amostras consecutivas
// (mesma constante usada na carta I-MR, ver ADR-1).
const capabilityD2ForN2 = 1.128

// ComputeCapability calcula Cp/Cpk contra os limites de especificação
// fornecidos. upperSpecLimit e lowerSpecLimit são ponteiros: nil significa
// "sem limite naquele lado". Pelo menos um dos dois deve ser não-nil.
//
// Sigma é estimado via MRbar/1.128 (moving range de amostras consecutivas,
// constante d2 para n=2), consistente com a carta I-MR usada no restante do
// motor SPC.
//
//   - Se só USL: Cp = NaN (não computável sem os dois lados),
//     Cpk = (USL - mean) / (3*sigma).
//   - Se só LSL: Cp = NaN, Cpk = (mean - LSL) / (3*sigma).
//   - Se os dois: Cp = (USL-LSL)/(6*sigma),
//     Cpk = min( (USL-mean)/(3*sigma), (mean-LSL)/(3*sigma) ).
func ComputeCapability(values []ValueAt, upperSpecLimit, lowerSpecLimit *float64) (CapabilityResult, error) {
	if upperSpecLimit == nil && lowerSpecLimit == nil {
		return CapabilityResult{}, fmt.Errorf("computing capability: at least one spec limit (upper or lower) must be provided")
	}
	if len(values) < 2 {
		return CapabilityResult{}, fmt.Errorf("computing capability: need at least 2 values, got %d", len(values))
	}

	mean := capabilityMean(values)
	sigma := capabilitySigma(values)

	result := CapabilityResult{
		Mean:  mean,
		Sigma: sigma,
	}

	switch {
	case upperSpecLimit != nil && lowerSpecLimit != nil:
		usl, lsl := *upperSpecLimit, *lowerSpecLimit
		result.Cp = (usl - lsl) / (6 * sigma)
		cpu := (usl - mean) / (3 * sigma)
		cpl := (mean - lsl) / (3 * sigma)
		result.Cpk = math.Min(cpu, cpl)
	case upperSpecLimit != nil:
		result.Cp = math.NaN()
		result.Cpk = (*upperSpecLimit - mean) / (3 * sigma)
	default: // lowerSpecLimit != nil
		result.Cp = math.NaN()
		result.Cpk = (mean - *lowerSpecLimit) / (3 * sigma)
	}

	return result, nil
}

func capabilityMean(values []ValueAt) float64 {
	var sum float64
	for _, v := range values {
		sum += v.Value
	}
	return sum / float64(len(values))
}

func capabilitySigma(values []ValueAt) float64 {
	var mrSum float64
	for i := 1; i < len(values); i++ {
		mr := values[i].Value - values[i-1].Value
		if mr < 0 {
			mr = -mr
		}
		mrSum += mr
	}
	mrBar := mrSum / float64(len(values)-1)
	return mrBar / capabilityD2ForN2
}
