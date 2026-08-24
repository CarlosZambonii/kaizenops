// Package security normaliza achados de ferramentas de segurança do CI
// (gosec, Trivy) em métricas de qualidade, seguindo o princípio "Twist Lean"
// do KaizenOps: segurança é medida como processo, não como checklist. MTTR
// de vulnerabilidade e taxa de defeito por release entram nas cartas de
// controle do motor SPC a partir dos tipos definidos aqui.
package security

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Finding é um achado de segurança normalizado, independente da ferramenta
// que o produziu.
type Finding struct {
	Tool        string // "gosec" ou "trivy"
	Severity    string // normalizado para: "LOW", "MEDIUM", "HIGH", "CRITICAL"
	RuleID      string // ID da regra gosec, ou o CVE do Trivy
	Description string
	File        string // caminho do arquivo afetado (gosec) ou pacote afetado (trivy); pode ser vazio
}

// key identifica um Finding de forma estável entre dois snapshots, para
// permitir comparação independente da ordem em que os achados aparecem no
// relatório da ferramenta.
func (f Finding) key() string {
	return f.Tool + "\x00" + f.RuleID + "\x00" + f.File
}

// normalizeSeverity mapeia a severidade crua reportada pela ferramenta para
// um dos quatro níveis normalizados do contrato do pacote. Qualquer valor
// não reconhecido (por exemplo "UNKNOWN", reportado pelo Trivy quando o
// advisory upstream não classifica a vulnerabilidade) é tratado como "LOW":
// suposição deliberada para nunca inflar a taxa de defeito com severidade
// desconhecida, e nunca descartar o achado silenciosamente.
func normalizeSeverity(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "LOW":
		return "LOW"
	case "MEDIUM":
		return "MEDIUM"
	case "HIGH":
		return "HIGH"
	case "CRITICAL":
		return "CRITICAL"
	default:
		return "LOW"
	}
}

// gosecReport reflete apenas os campos de "gosec -fmt=json" que este pacote
// precisa. O restante do documento (Stats, Golang errors, cwe, confidence
// etc.) é ignorado sem erro, para tolerar variações de versão da ferramenta.
type gosecReport struct {
	Issues []gosecIssue `json:"Issues"`
}

type gosecIssue struct {
	Severity string `json:"severity"`
	RuleID   string `json:"rule_id"`
	Details  string `json:"details"`
	File     string `json:"file"`
}

// ParseGosecReport lê bytes de um relatório "gosec -fmt=json" e retorna os
// Finding normalizados.
func ParseGosecReport(data []byte) ([]Finding, error) {
	var report gosecReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parsing gosec report: %w", err)
	}

	findings := make([]Finding, 0, len(report.Issues))
	for _, issue := range report.Issues {
		findings = append(findings, Finding{
			Tool:        "gosec",
			Severity:    normalizeSeverity(issue.Severity),
			RuleID:      issue.RuleID,
			Description: issue.Details,
			File:        issue.File,
		})
	}

	return findings, nil
}

// trivyReport reflete apenas os campos do JSON padrão de "trivy image -f
// json" ou "trivy fs -f json" que este pacote precisa. Achados sem
// vulnerabilidades (ex: secret scanning, misconfig) em Results são
// simplesmente ignorados porque Vulnerabilities fica vazio.
type trivyReport struct {
	Results []trivyResult `json:"Results"`
}

type trivyResult struct {
	Target          string          `json:"Target"`
	Vulnerabilities []trivyVulnerab `json:"Vulnerabilities"`
}

type trivyVulnerab struct {
	VulnerabilityID string `json:"VulnerabilityID"`
	PkgName         string `json:"PkgName"`
	Severity        string `json:"Severity"`
	Title           string `json:"Title"`
	Description     string `json:"Description"`
}

// ParseTrivyReport lê bytes de um relatório Trivy (fs ou image, formato JSON
// padrão do Trivy) e retorna os Finding normalizados. O pacote afetado
// (PkgName) é usado como File; quando ausente, cai para o Target do
// resultado, para nunca perder a localização do achado.
func ParseTrivyReport(data []byte) ([]Finding, error) {
	var report trivyReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, fmt.Errorf("parsing trivy report: %w", err)
	}

	var findings []Finding
	for _, result := range report.Results {
		for _, vuln := range result.Vulnerabilities {
			description := vuln.Title
			if description == "" {
				description = vuln.Description
			}

			affected := vuln.PkgName
			if affected == "" {
				affected = result.Target
			}

			findings = append(findings, Finding{
				Tool:        "trivy",
				Severity:    normalizeSeverity(vuln.Severity),
				RuleID:      vuln.VulnerabilityID,
				Description: description,
				File:        affected,
			})
		}
	}

	return findings, nil
}

// Remediation representa o tempo que um finding ficou aberto entre dois
// snapshots.
type Remediation struct {
	Finding  Finding
	OpenedAt time.Time // timestamp do snapshot "before"
	ClosedAt time.Time // timestamp do snapshot "after" em que ele já não aparece mais
	Duration time.Duration
}

// ComputeRemediations recebe os findings de dois snapshots (before, capturado
// em beforeAt; after, capturado em afterAt) e retorna um Remediation para
// cada finding presente em "before" que não está mais presente em "after"
// (ou seja, foi corrigido entre os dois snapshots). A comparação de
// identidade usa Tool+RuleID+File, não a Description, porque o texto
// descritivo pode variar entre versões da ferramenta sem que o achado em si
// tenha mudado.
func ComputeRemediations(before []Finding, beforeAt time.Time, after []Finding, afterAt time.Time) []Remediation {
	stillOpen := make(map[string]struct{}, len(after))
	for _, f := range after {
		stillOpen[f.key()] = struct{}{}
	}

	remediations := make([]Remediation, 0, len(before))
	for _, f := range before {
		if _, open := stillOpen[f.key()]; open {
			continue
		}
		remediations = append(remediations, Remediation{
			Finding:  f,
			OpenedAt: beforeAt,
			ClosedAt: afterAt,
			Duration: afterAt.Sub(beforeAt),
		})
	}

	return remediations
}
