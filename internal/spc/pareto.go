package spc

import "sort"

// FailureCause é uma causa de falha de pipeline agrupada por job ou por
// teste. Name NUNCA deve ser o nome ou login de uma pessoa — identidade de
// contribuidor é pseudonimizada na ingestão e não tem lugar aqui (ver
// princípio central do KaizenOps: melhorar o processo, nunca vigiar pessoas).
type FailureCause struct {
	Name  string
	Count int
}

// ParetoEntry é uma linha do diagrama de Pareto: a causa, sua contagem, e o
// percentual acumulado até ela (inclusive) sobre o total de ocorrências.
type ParetoEntry struct {
	Name              string
	Count             int
	CumulativePercent float64 // 0-100
}

// Pareto ordena causes por Count decrescente e calcula o percentual
// acumulado. Em caso de Count igual, o desempate é por Name em ordem
// alfabética, para garantir saída determinística.
func Pareto(causes []FailureCause) []ParetoEntry {
	if len(causes) == 0 {
		return []ParetoEntry{}
	}

	sorted := make([]FailureCause, len(causes))
	copy(sorted, causes)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Count != sorted[j].Count {
			return sorted[i].Count > sorted[j].Count
		}
		return sorted[i].Name < sorted[j].Name
	})

	var total int
	for _, c := range sorted {
		total += c.Count
	}

	entries := make([]ParetoEntry, len(sorted))
	var cumulative int
	for i, c := range sorted {
		cumulative += c.Count
		var pct float64
		if total > 0 {
			pct = float64(cumulative) / float64(total) * 100
		}
		entries[i] = ParetoEntry{
			Name:              c.Name,
			Count:             c.Count,
			CumulativePercent: pct,
		}
	}

	return entries
}
