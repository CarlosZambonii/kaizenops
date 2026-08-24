package spc

import "testing"

// chartFromValues monta uma IMRChart "à mão" a partir de valores e dos
// limites de controle desejados, sem passar por ComputeIMR. Isso permite
// testar cada regra de Nelson isoladamente, com séries sintéticas pequenas e
// conhecidas.
func chartFromValues(centerLine, ucl, lcl float64, values ...float64) IMRChart {
	pts := make([]IMRPoint, len(values))
	for i, v := range values {
		pts[i] = IMRPoint{
			Point:        Point{Value: v},
			OutOfControl: v > ucl || v < lcl,
		}
	}
	return IMRChart{
		CenterLine: centerLine,
		UCL:        ucl,
		LCL:        lcl,
		Points:     pts,
	}
}

func countViolations(violations []NelsonViolation, rule int) int {
	n := 0
	for _, v := range violations {
		if v.RuleNumber == rule {
			n++
		}
	}
	return n
}

func TestApplyNelsonRules_Rule1(t *testing.T) {
	tests := []struct {
		name        string
		chart       IMRChart
		wantIndexes []int
	}{
		{
			name:        "no point beyond 3-sigma limits",
			chart:       chartFromValues(10, 20, 0, 10, 12, 8, 15, 5),
			wantIndexes: nil,
		},
		{
			name:        "single point above UCL",
			chart:       chartFromValues(10, 20, 0, 10, 12, 8, 25, 5),
			wantIndexes: []int{3},
		},
		{
			name:        "single point below LCL",
			chart:       chartFromValues(10, 20, 0, 10, -5, 8, 15, 5),
			wantIndexes: []int{1},
		},
		{
			name:        "multiple points beyond limits",
			chart:       chartFromValues(10, 20, 0, 25, 12, -5, 15, 5),
			wantIndexes: []int{0, 2},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ApplyNelsonRules(tc.chart)
			var gotIndexes []int
			for _, v := range got {
				if v.RuleNumber == 1 {
					gotIndexes = append(gotIndexes, v.Index)
				}
			}
			if !intSlicesEqual(gotIndexes, tc.wantIndexes) {
				t.Errorf("rule 1 violations at %v, want %v", gotIndexes, tc.wantIndexes)
			}
		})
	}
}

func TestApplyNelsonRules_Rule2(t *testing.T) {
	// Limites de controle bem largos para não interferir (nenhum ponto
	// dispara a regra 1 nestes casos).
	const ucl, lcl = 1000.0, -1000.0
	const centerLine = 10.0

	tests := []struct {
		name    string
		values  []float64
		wantHit bool
	}{
		{
			name:    "9 consecutive points above center line triggers",
			values:  []float64{11, 12, 13, 11, 12, 13, 11, 12, 13},
			wantHit: true,
		},
		{
			name:    "9 consecutive points below center line triggers",
			values:  []float64{9, 8, 7, 9, 8, 7, 9, 8, 7},
			wantHit: true,
		},
		{
			name:    "8 consecutive points same side does not trigger",
			values:  []float64{11, 12, 13, 11, 12, 13, 11, 12},
			wantHit: false,
		},
		{
			name:    "points alternate sides, never triggers",
			values:  []float64{11, 9, 11, 9, 11, 9, 11, 9, 11, 9},
			wantHit: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chart := chartFromValues(centerLine, ucl, lcl, tc.values...)
			got := ApplyNelsonRules(chart)
			hit := countViolations(got, 2) > 0
			if hit != tc.wantHit {
				t.Errorf("rule 2 triggered = %v, want %v (violations: %+v)", hit, tc.wantHit, got)
			}
			if tc.wantHit {
				// A violação deve ser reportada no último ponto da sequência de 9.
				lastIdx := len(tc.values) - 1
				found := false
				for _, v := range got {
					if v.RuleNumber == 2 && v.Index == lastIdx {
						found = true
					}
				}
				if !found {
					t.Errorf("expected rule 2 violation at index %d, got %+v", lastIdx, got)
				}
			}
		})
	}
}

func TestApplyNelsonRules_Rule3(t *testing.T) {
	const ucl, lcl = 1000.0, -1000.0
	const centerLine = 10.0

	tests := []struct {
		name    string
		values  []float64
		wantHit bool
	}{
		{
			name:    "6 consecutive strictly increasing points triggers",
			values:  []float64{1, 2, 3, 4, 5, 6},
			wantHit: true,
		},
		{
			name:    "6 consecutive strictly decreasing points triggers",
			values:  []float64{6, 5, 4, 3, 2, 1},
			wantHit: true,
		},
		{
			name:    "5 consecutive increasing points does not trigger",
			values:  []float64{1, 2, 3, 4, 5},
			wantHit: false,
		},
		{
			name:    "trend broken by a plateau does not trigger",
			values:  []float64{1, 2, 3, 3, 4, 5},
			wantHit: false,
		},
		{
			name:    "non-monotonic series does not trigger",
			values:  []float64{1, 5, 2, 6, 3, 7},
			wantHit: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chart := chartFromValues(centerLine, ucl, lcl, tc.values...)
			got := ApplyNelsonRules(chart)
			hit := countViolations(got, 3) > 0
			if hit != tc.wantHit {
				t.Errorf("rule 3 triggered = %v, want %v (violations: %+v)", hit, tc.wantHit, got)
			}
		})
	}
}

func TestApplyNelsonRules_Rule4(t *testing.T) {
	const ucl, lcl = 1000.0, -1000.0
	const centerLine = 10.0

	// 14 pontos alternando estritamente acima/abaixo do anterior (zigue-zague).
	zigzag14 := []float64{1, 5, 2, 6, 3, 7, 4, 8, 5, 9, 6, 10, 7, 11}
	// 13 pontos alternando: um a menos que o necessário.
	zigzag13 := zigzag14[:13]
	// Série monotônica crescente: nunca alterna.
	monotonic := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14}

	tests := []struct {
		name    string
		values  []float64
		wantHit bool
	}{
		{
			name:    "14 consecutive alternating points triggers",
			values:  zigzag14,
			wantHit: true,
		},
		{
			name:    "13 consecutive alternating points does not trigger",
			values:  zigzag13,
			wantHit: false,
		},
		{
			name:    "monotonic series never alternates, does not trigger",
			values:  monotonic,
			wantHit: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chart := chartFromValues(centerLine, ucl, lcl, tc.values...)
			got := ApplyNelsonRules(chart)
			hit := countViolations(got, 4) > 0
			if hit != tc.wantHit {
				t.Errorf("rule 4 triggered = %v, want %v (violations: %+v)", hit, tc.wantHit, got)
			}
		})
	}
}

func TestApplyNelsonRules_NormalSeriesNoViolations(t *testing.T) {
	// Série "de manual": ruído pequeno em torno da linha central, sem runs,
	// tendências ou zigue-zague longos o suficiente, e sem pontos fora dos
	// limites de controle.
	chart := chartFromValues(10, 20, 0,
		10, 11, 9, 10, 12, 9, 10, 11, 10, 9, 11, 10,
	)

	got := ApplyNelsonRules(chart)
	if len(got) != 0 {
		t.Errorf("expected no violations for a normal series, got %+v", got)
	}
}

func intSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
