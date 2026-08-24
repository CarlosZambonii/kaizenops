// Package dora calcula as quatro DORA metrics — deployment frequency, lead
// time for changes, change failure rate e MTTR — como funções puras sobre
// dados já coletados. Nenhuma função deste pacote faz IO: quem chama decide
// a fonte dos dados e o instante "agora".
package dora

import (
	"fmt"
	"sort"
	"time"
)

// Deployment representa uma entrega de mudança em produção.
type Deployment struct {
	DeployedAt time.Time
	LeadTime   time.Duration // tempo do primeiro commit até o deploy
	Success    bool          // false = essa entrega causou falha/incidente (change failure)
}

// Incident representa um incidente causado por uma entrega, usado para
// calcular o MTTR (Mean Time To Restore/Repair).
type Incident struct {
	StartedAt  time.Time
	ResolvedAt time.Time
}

// Metrics agrega as quatro DORA metrics calculadas por Compute.
type Metrics struct {
	DeploymentFrequencyPerDay float64
	MedianLeadTime            time.Duration
	ChangeFailureRate         float64 // 0.0-1.0, = deployments com Success=false / total de deployments
	MTTR                      time.Duration
}

// Compute calcula as DORA metrics considerando apenas deployments e
// incidents cujo instante relevante (DeployedAt / StartedAt) caia dentro da
// janela [now-window, now].
//
// "now" é sempre passado explicitamente pelo chamador — Compute nunca lê o
// relógio do sistema, o que torna o cálculo determinístico e testável.
//
// Retorna erro se window <= 0.
func Compute(now time.Time, deployments []Deployment, incidents []Incident, window time.Duration) (Metrics, error) {
	if window <= 0 {
		return Metrics{}, fmt.Errorf("computing dora metrics: window inválida: %v", window)
	}

	windowStart := now.Add(-window)

	inWindow := make([]Deployment, 0, len(deployments))
	for _, d := range deployments {
		if isWithin(d.DeployedAt, windowStart, now) {
			inWindow = append(inWindow, d)
		}
	}

	metrics := Metrics{}

	if len(inWindow) > 0 {
		windowDays := window.Hours() / 24
		metrics.DeploymentFrequencyPerDay = float64(len(inWindow)) / windowDays

		metrics.MedianLeadTime = medianDuration(leadTimes(inWindow))

		failures := 0
		for _, d := range inWindow {
			if !d.Success {
				failures++
			}
		}
		metrics.ChangeFailureRate = float64(failures) / float64(len(inWindow))
	}

	var totalRestore time.Duration
	restoredCount := 0
	for _, inc := range incidents {
		if isWithin(inc.StartedAt, windowStart, now) {
			totalRestore += inc.ResolvedAt.Sub(inc.StartedAt)
			restoredCount++
		}
	}
	if restoredCount > 0 {
		metrics.MTTR = totalRestore / time.Duration(restoredCount)
	}

	return metrics, nil
}

// isWithin reporta se t está no intervalo fechado [start, end].
func isWithin(t, start, end time.Time) bool {
	return !t.Before(start) && !t.After(end)
}

func leadTimes(deployments []Deployment) []time.Duration {
	out := make([]time.Duration, len(deployments))
	for i, d := range deployments {
		out[i] = d.LeadTime
	}
	return out
}

// medianDuration calcula a mediana de uma lista de durações. Para número par
// de amostras, retorna a média das duas centrais.
func medianDuration(durations []time.Duration) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(durations))
	copy(sorted, durations)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid]
	}
	return (sorted[mid-1] + sorted[mid]) / 2
}
