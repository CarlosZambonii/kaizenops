package spc

import (
	"math"
	"testing"
	"time"
)

const floatTolerance = 1e-9

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < floatTolerance
}

func TestComputeIMR(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	pointsFrom := func(values ...float64) []Point {
		pts := make([]Point, len(values))
		for i, v := range values {
			pts[i] = Point{Timestamp: base.Add(time.Duration(i) * time.Hour), Value: v}
		}
		return pts
	}

	tests := []struct {
		name             string
		points           []Point
		wantErr          bool
		wantCenterLine   float64
		wantMRBar        float64
		wantUCL          float64
		wantLCL          float64
		wantMRUCL        float64
		wantMovingRanges []float64
		wantOutOfControl []bool
	}{
		{
			name:    "empty series returns error",
			points:  nil,
			wantErr: true,
		},
		{
			name:    "single point returns error",
			points:  pointsFrom(42),
			wantErr: true,
		},
		{
			name:             "stable series, all in control",
			points:           pointsFrom(10, 12, 9, 15, 11),
			wantCenterLine:   11.4,
			wantMRBar:        3.75,
			wantUCL:          11.4 + 2.66*3.75,
			wantLCL:          11.4 - 2.66*3.75,
			wantMRUCL:        3.267 * 3.75,
			wantMovingRanges: []float64{0, 2, 3, 6, 4},
			wantOutOfControl: []bool{false, false, false, false, false},
		},
		{
			name:             "spike above UCL is flagged out of control",
			points:           pointsFrom(0, 0, 0, 0, 100),
			wantCenterLine:   20,
			wantMRBar:        25,
			wantUCL:          20 + 2.66*25,
			wantLCL:          20 - 2.66*25,
			wantMRUCL:        3.267 * 25,
			wantMovingRanges: []float64{0, 0, 0, 0, 100},
			wantOutOfControl: []bool{false, false, false, false, true},
		},
		{
			name: "drop below LCL is flagged out of control",
			// mean = (50*4 + (-50)) / 5 = 30; MRbar = (0+0+0+100)/4 = 25
			// UCL = 30 + 66.5 = 96.5; LCL = 30 - 66.5 = -36.5; last point -50 < LCL.
			points:           pointsFrom(50, 50, 50, 50, -50),
			wantCenterLine:   30,
			wantMRBar:        25,
			wantUCL:          30 + 2.66*25,
			wantLCL:          30 - 2.66*25,
			wantMRUCL:        3.267 * 25,
			wantMovingRanges: []float64{0, 0, 0, 0, 100},
			wantOutOfControl: []bool{false, false, false, false, true},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chart, err := ComputeIMR(tc.points)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ComputeIMR() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ComputeIMR() unexpected error: %v", err)
			}

			if !almostEqual(chart.CenterLine, tc.wantCenterLine) {
				t.Errorf("CenterLine = %v, want %v", chart.CenterLine, tc.wantCenterLine)
			}
			if !almostEqual(chart.MRBar, tc.wantMRBar) {
				t.Errorf("MRBar = %v, want %v", chart.MRBar, tc.wantMRBar)
			}
			if !almostEqual(chart.UCL, tc.wantUCL) {
				t.Errorf("UCL = %v, want %v", chart.UCL, tc.wantUCL)
			}
			if !almostEqual(chart.LCL, tc.wantLCL) {
				t.Errorf("LCL = %v, want %v", chart.LCL, tc.wantLCL)
			}
			if !almostEqual(chart.MRUCL, tc.wantMRUCL) {
				t.Errorf("MRUCL = %v, want %v", chart.MRUCL, tc.wantMRUCL)
			}

			if len(chart.Points) != len(tc.points) {
				t.Fatalf("len(Points) = %d, want %d", len(chart.Points), len(tc.points))
			}
			for i, p := range chart.Points {
				if !almostEqual(p.MovingRange, tc.wantMovingRanges[i]) {
					t.Errorf("Points[%d].MovingRange = %v, want %v", i, p.MovingRange, tc.wantMovingRanges[i])
				}
				if p.OutOfControl != tc.wantOutOfControl[i] {
					t.Errorf("Points[%d].OutOfControl = %v, want %v", i, p.OutOfControl, tc.wantOutOfControl[i])
				}
				if !p.Timestamp.Equal(tc.points[i].Timestamp) {
					t.Errorf("Points[%d].Timestamp = %v, want %v", i, p.Timestamp, tc.points[i].Timestamp)
				}
				if p.Value != tc.points[i].Value {
					t.Errorf("Points[%d].Value = %v, want %v", i, p.Value, tc.points[i].Value)
				}
			}
		})
	}
}
