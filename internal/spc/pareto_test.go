package spc

import (
	"testing"
)

func TestPareto(t *testing.T) {
	tests := []struct {
		name   string
		causes []FailureCause
		want   []ParetoEntry
	}{
		{
			name:   "empty list",
			causes: nil,
			want:   []ParetoEntry{},
		},
		{
			name: "single cause is 100 percent",
			causes: []FailureCause{
				{Name: "flaky_test_login", Count: 5},
			},
			want: []ParetoEntry{
				{Name: "flaky_test_login", Count: 5, CumulativePercent: 100},
			},
		},
		{
			name: "sorted by count descending",
			causes: []FailureCause{
				{Name: "job_lint", Count: 2},
				{Name: "job_build", Count: 10},
				{Name: "job_test", Count: 8},
			},
			// total = 20
			want: []ParetoEntry{
				{Name: "job_build", Count: 10, CumulativePercent: 50},
				{Name: "job_test", Count: 8, CumulativePercent: 90},
				{Name: "job_lint", Count: 2, CumulativePercent: 100},
			},
		},
		{
			name: "tie broken alphabetically by name",
			causes: []FailureCause{
				{Name: "test_zebra", Count: 3},
				{Name: "test_alpha", Count: 3},
				{Name: "test_mango", Count: 3},
			},
			// total = 9, each 3 -> cumulative 1/3, 2/3, 3/3
			want: []ParetoEntry{
				{Name: "test_alpha", Count: 3, CumulativePercent: 100.0 / 3.0},
				{Name: "test_mango", Count: 3, CumulativePercent: 200.0 / 3.0},
				{Name: "test_zebra", Count: 3, CumulativePercent: 100},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Pareto(tt.causes)
			if len(got) != len(tt.want) {
				t.Fatalf("Pareto() returned %d entries, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i].Name != tt.want[i].Name {
					t.Errorf("entry %d: Name = %q, want %q", i, got[i].Name, tt.want[i].Name)
				}
				if got[i].Count != tt.want[i].Count {
					t.Errorf("entry %d: Count = %d, want %d", i, got[i].Count, tt.want[i].Count)
				}
				if !approxEqual(got[i].CumulativePercent, tt.want[i].CumulativePercent, 1e-9) {
					t.Errorf("entry %d: CumulativePercent = %v, want %v", i, got[i].CumulativePercent, tt.want[i].CumulativePercent)
				}
			}
			if len(got) > 0 {
				last := got[len(got)-1].CumulativePercent
				if !approxEqual(last, 100, 1e-9) {
					t.Errorf("last entry CumulativePercent = %v, want 100", last)
				}
			}
		})
	}
}
