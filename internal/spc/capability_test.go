package spc

import (
	"math"
	"testing"
	"time"
)

func approxEqual(a, b, eps float64) bool {
	return math.Abs(a-b) <= eps
}

func TestComputeCapability(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	twoValues := []ValueAt{
		{Timestamp: base, Value: 10},
		{Timestamp: base.Add(time.Minute), Value: 20},
	}
	// mean = 15, MRbar = 10, sigma = 10/1.128 = 1250/141 ≈ 8.865248226950354
	const wantMean = 15.0
	const wantSigma = 1250.0 / 141.0
	const eps = 1e-9

	usl30 := 30.0
	lsl5 := -5.0

	tests := []struct {
		name    string
		values  []ValueAt
		usl     *float64
		lsl     *float64
		wantErr bool
		wantCp  float64 // ignored (NaN expected) when wantCpNaN is true
		wantCpk float64
		cpIsNaN bool
	}{
		{
			name:    "only upper spec limit",
			values:  twoValues,
			usl:     &usl30,
			lsl:     nil,
			cpIsNaN: true,
			// Cpk = (30-15)/(3*sigma) = 0.564
			wantCpk: 0.564,
		},
		{
			name:    "only lower spec limit",
			values:  twoValues,
			usl:     nil,
			lsl:     &lsl5,
			cpIsNaN: true,
			// Cpk = (15-(-5))/(3*sigma) = 0.752
			wantCpk: 0.752,
		},
		{
			name:   "both spec limits",
			values: twoValues,
			usl:    &usl30,
			lsl:    &lsl5,
			// Cp = (30-(-5))/(6*sigma) = 0.658
			wantCp: 0.658,
			// Cpk = min(0.564, 0.752) = 0.564
			wantCpk: 0.564,
		},
		{
			name:    "error: no spec limit provided",
			values:  twoValues,
			usl:     nil,
			lsl:     nil,
			wantErr: true,
		},
		{
			name:    "error: fewer than two values",
			values:  []ValueAt{{Timestamp: base, Value: 10}},
			usl:     &usl30,
			lsl:     nil,
			wantErr: true,
		},
		{
			name:    "error: no values at all",
			values:  nil,
			usl:     &usl30,
			lsl:     nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeCapability(tt.values, tt.usl, tt.lsl)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ComputeCapability() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ComputeCapability() unexpected error: %v", err)
			}
			if !approxEqual(got.Mean, wantMean, eps) {
				t.Errorf("Mean = %v, want %v", got.Mean, wantMean)
			}
			if !approxEqual(got.Sigma, wantSigma, eps) {
				t.Errorf("Sigma = %v, want %v", got.Sigma, wantSigma)
			}
			if tt.cpIsNaN {
				if !math.IsNaN(got.Cp) {
					t.Errorf("Cp = %v, want NaN", got.Cp)
				}
			} else if !approxEqual(got.Cp, tt.wantCp, eps) {
				t.Errorf("Cp = %v, want %v", got.Cp, tt.wantCp)
			}
			if !approxEqual(got.Cpk, tt.wantCpk, eps) {
				t.Errorf("Cpk = %v, want %v", got.Cpk, tt.wantCpk)
			}
		})
	}
}
