package spc

import "fmt"

// NelsonViolation descreve uma violação de uma das regras de Nelson
// detectada sobre uma carta I-MR.
type NelsonViolation struct {
	RuleNumber  int // 1, 2, 3 ou 4.
	Index       int // índice em IMRChart.Points onde a violação é detectada (último ponto do padrão).
	Description string
}

const (
	nelsonRule2RunLength    = 9  // pontos consecutivos do mesmo lado da linha central.
	nelsonRule3TrendLength  = 6  // pontos consecutivos em tendência monotônica.
	nelsonRule4ZigzagLength = 14 // pontos consecutivos alternando.
)

// ApplyNelsonRules aplica as regras de Nelson 1 a 4 sobre chart.Points,
// usando chart.CenterLine, UCL e LCL como referência.
//
// Regra 1: um ponto único fora de UCL/LCL (3-sigma).
// Regra 2: 9 pontos consecutivos do mesmo lado da linha central.
// Regra 3: 6 pontos consecutivos em tendência monotônica (crescente ou decrescente).
// Regra 4: 14 pontos consecutivos alternando (zigue-zague) acima/abaixo do ponto anterior.
func ApplyNelsonRules(chart IMRChart) []NelsonViolation {
	var violations []NelsonViolation

	violations = append(violations, applyRule1(chart)...)
	violations = append(violations, applyRule2(chart)...)
	violations = append(violations, applyRule3(chart)...)
	violations = append(violations, applyRule4(chart)...)

	return violations
}

// applyRule1 detecta pontos únicos fora dos limites de controle 3-sigma.
func applyRule1(chart IMRChart) []NelsonViolation {
	var out []NelsonViolation
	for i, p := range chart.Points {
		if p.Value > chart.UCL || p.Value < chart.LCL {
			out = append(out, NelsonViolation{
				RuleNumber:  1,
				Index:       i,
				Description: fmt.Sprintf("point %d (value=%.4f) is beyond control limits [%.4f, %.4f]", i, p.Value, chart.LCL, chart.UCL),
			})
		}
	}
	return out
}

// applyRule2 detecta sequências de nelsonRule2RunLength pontos consecutivos
// do mesmo lado da linha central.
func applyRule2(chart IMRChart) []NelsonViolation {
	var out []NelsonViolation
	run := 0
	sign := 0 // +1 acima da linha central, -1 abaixo, 0 indefinido.
	for i, p := range chart.Points {
		var current int
		switch {
		case p.Value > chart.CenterLine:
			current = 1
		case p.Value < chart.CenterLine:
			current = -1
		default:
			current = 0
		}

		if current != 0 && current == sign {
			run++
		} else {
			run = 1
			sign = current
		}

		if sign != 0 && run >= nelsonRule2RunLength {
			out = append(out, NelsonViolation{
				RuleNumber:  2,
				Index:       i,
				Description: fmt.Sprintf("points %d-%d are %d consecutive points on the same side of the center line", i-nelsonRule2RunLength+1, i, nelsonRule2RunLength),
			})
		}
	}
	return out
}

// applyRule3 detecta sequências de nelsonRule3TrendLength pontos consecutivos
// em tendência monotônica (estritamente crescente ou decrescente).
func applyRule3(chart IMRChart) []NelsonViolation {
	var out []NelsonViolation
	run := 1   // conta pontos na tendência atual; começa em 1 (o próprio primeiro ponto).
	trend := 0 // +1 crescente, -1 decrescente, 0 indefinido.
	for i := 1; i < len(chart.Points); i++ {
		prev := chart.Points[i-1].Value
		curr := chart.Points[i].Value

		var direction int
		switch {
		case curr > prev:
			direction = 1
		case curr < prev:
			direction = -1
		default:
			direction = 0
		}

		if direction != 0 && direction == trend {
			run++
		} else {
			run = 2 // o par (i-1, i) já forma uma tendência de 2 pontos.
			trend = direction
			if direction == 0 {
				run = 1
				trend = 0
			}
		}

		if trend != 0 && run >= nelsonRule3TrendLength {
			out = append(out, NelsonViolation{
				RuleNumber:  3,
				Index:       i,
				Description: fmt.Sprintf("points %d-%d form a monotonic trend of %d consecutive points", i-nelsonRule3TrendLength+1, i, nelsonRule3TrendLength),
			})
		}
	}
	return out
}

// applyRule4 detecta sequências de nelsonRule4ZigzagLength pontos consecutivos
// alternando (zigue-zague) acima/abaixo do ponto anterior.
func applyRule4(chart IMRChart) []NelsonViolation {
	var out []NelsonViolation
	run := 1     // conta pontos na sequência de alternância atual.
	lastDir := 0 // direção do último passo: +1 subiu, -1 desceu, 0 indefinido.
	for i := 1; i < len(chart.Points); i++ {
		prev := chart.Points[i-1].Value
		curr := chart.Points[i].Value

		var direction int
		switch {
		case curr > prev:
			direction = 1
		case curr < prev:
			direction = -1
		default:
			direction = 0
		}

		if direction != 0 && lastDir != 0 && direction == -lastDir {
			run++
		} else if direction != 0 {
			run = 2 // o par (i-1, i) já forma uma alternância de 2 pontos.
		} else {
			run = 1
		}
		lastDir = direction

		if run >= nelsonRule4ZigzagLength {
			out = append(out, NelsonViolation{
				RuleNumber:  4,
				Index:       i,
				Description: fmt.Sprintf("points %d-%d alternate direction for %d consecutive points", i-nelsonRule4ZigzagLength+1, i, nelsonRule4ZigzagLength),
			})
		}
	}
	return out
}
