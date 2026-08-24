package dora

import (
	"testing"
	"time"
)

func TestCompute(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		now         time.Time
		deployments []Deployment
		incidents   []Incident
		window      time.Duration
		want        Metrics
	}{
		{
			name:        "janela vazia de dados",
			now:         now,
			deployments: nil,
			incidents:   nil,
			window:      24 * time.Hour,
			want: Metrics{
				DeploymentFrequencyPerDay: 0,
				MedianLeadTime:            0,
				ChangeFailureRate:         0,
				MTTR:                      0,
			},
		},
		{
			name: "deployments com sucesso e falha misturados",
			now:  now,
			deployments: []Deployment{
				{DeployedAt: now.Add(-1 * time.Hour), LeadTime: 2 * time.Hour, Success: true},
				{DeployedAt: now.Add(-2 * time.Hour), LeadTime: 4 * time.Hour, Success: false},
				{DeployedAt: now.Add(-3 * time.Hour), LeadTime: 6 * time.Hour, Success: true},
				{DeployedAt: now.Add(-4 * time.Hour), LeadTime: 8 * time.Hour, Success: false},
			},
			incidents: nil,
			window:    24 * time.Hour,
			want: Metrics{
				DeploymentFrequencyPerDay: 4.0,           // 4 deployments em janela de 1 dia
				MedianLeadTime:            5 * time.Hour, // par: (4h+6h)/2
				ChangeFailureRate:         0.5,           // 2 de 4
				MTTR:                      0,
			},
		},
		{
			name: "mediana de lead time com número ímpar de amostras",
			now:  now,
			deployments: []Deployment{
				{DeployedAt: now.Add(-1 * time.Hour), LeadTime: 1 * time.Hour, Success: true},
				{DeployedAt: now.Add(-2 * time.Hour), LeadTime: 5 * time.Hour, Success: true},
				{DeployedAt: now.Add(-3 * time.Hour), LeadTime: 3 * time.Hour, Success: true},
			},
			incidents: nil,
			window:    24 * time.Hour,
			want: Metrics{
				DeploymentFrequencyPerDay: 3.0,
				MedianLeadTime:            3 * time.Hour, // ordenado: 1,3,5 -> mediana 3
				ChangeFailureRate:         0,
				MTTR:                      0,
			},
		},
		{
			name: "incidents fora da janela são ignorados",
			now:  now,
			deployments: []Deployment{
				{DeployedAt: now.Add(-1 * time.Hour), LeadTime: time.Hour, Success: false},
			},
			incidents: []Incident{
				{StartedAt: now.Add(-1 * time.Hour), ResolvedAt: now.Add(-30 * time.Minute)}, // dentro: 30min
				{StartedAt: now.Add(-2 * time.Hour), ResolvedAt: now.Add(-1 * time.Hour)},    // dentro: 1h
				{StartedAt: now.Add(-48 * time.Hour), ResolvedAt: now.Add(-47 * time.Hour)},  // fora da janela de 24h
			},
			window: 24 * time.Hour,
			want: Metrics{
				DeploymentFrequencyPerDay: 1.0,
				MedianLeadTime:            time.Hour,
				ChangeFailureRate:         1.0,
				MTTR:                      45 * time.Minute, // média de 30min e 1h
			},
		},
		{
			name: "deployments fora da janela também são ignorados",
			now:  now,
			deployments: []Deployment{
				{DeployedAt: now.Add(-1 * time.Hour), LeadTime: 2 * time.Hour, Success: true},
				{DeployedAt: now.Add(-25 * time.Hour), LeadTime: 100 * time.Hour, Success: false},
			},
			incidents: nil,
			window:    24 * time.Hour,
			want: Metrics{
				DeploymentFrequencyPerDay: 1.0,
				MedianLeadTime:            2 * time.Hour,
				ChangeFailureRate:         0,
				MTTR:                      0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Compute(tt.now, tt.deployments, tt.incidents, tt.window)
			if err != nil {
				t.Fatalf("Compute() erro inesperado: %v", err)
			}
			if got != tt.want {
				t.Errorf("Compute() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCompute_InvalidWindow(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		window time.Duration
	}{
		{name: "window zero", window: 0},
		{name: "window negativa", window: -time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Compute(now, nil, nil, tt.window)
			if err == nil {
				t.Fatalf("Compute() esperava erro para window %v, obteve nil", tt.window)
			}
		})
	}
}
