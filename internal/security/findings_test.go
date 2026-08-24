package security

import (
	"os"
	"testing"
	"time"
)

func TestParseGosecReport(t *testing.T) {
	data, err := os.ReadFile("testdata/gosec_sample.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	findings, err := ParseGosecReport(data)
	if err != nil {
		t.Fatalf("ParseGosecReport() error = %v", err)
	}

	want := []Finding{
		{
			Tool:        "gosec",
			Severity:    "HIGH",
			RuleID:      "G201",
			Description: "SQL string formatting",
			File:        "/home/runner/work/kaizenops/kaizenops/internal/storage/postgres.go",
		},
		{
			Tool:        "gosec",
			Severity:    "MEDIUM",
			RuleID:      "G304",
			Description: "Potential file inclusion via variable",
			File:        "/home/runner/work/kaizenops/kaizenops/internal/ingest/worker.go",
		},
		{
			Tool:        "gosec",
			Severity:    "LOW",
			RuleID:      "G104",
			Description: "Errors unhandled.",
			File:        "/home/runner/work/kaizenops/kaizenops/internal/github/client.go",
		},
	}

	if len(findings) != len(want) {
		t.Fatalf("len(findings) = %d, want %d (%+v)", len(findings), len(want), findings)
	}
	for i, got := range findings {
		if got != want[i] {
			t.Errorf("findings[%d] = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestParseGosecReport_InvalidJSON(t *testing.T) {
	if _, err := ParseGosecReport([]byte("not json")); err == nil {
		t.Fatal("ParseGosecReport() error = nil, want error for invalid JSON")
	}
}

func TestParseTrivyReport(t *testing.T) {
	data, err := os.ReadFile("testdata/trivy_sample.json")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}

	findings, err := ParseTrivyReport(data)
	if err != nil {
		t.Fatalf("ParseTrivyReport() error = %v", err)
	}

	want := []Finding{
		{
			Tool:        "trivy",
			Severity:    "HIGH",
			RuleID:      "CVE-2023-5678",
			Description: "openssl: Invalid GF(2^m) parameters",
			File:        "openssl",
		},
		{
			Tool:        "trivy",
			Severity:    "LOW",
			RuleID:      "CVE-2024-0001",
			Description: "busybox: minor denial of service",
			File:        "busybox",
		},
		{
			Tool:        "trivy",
			Severity:    "CRITICAL",
			RuleID:      "CVE-2024-24786",
			Description: "protojson: infinite loop when unmarshaling certain forms of invalid JSON",
			File:        "google.golang.org/protobuf",
		},
	}

	if len(findings) != len(want) {
		t.Fatalf("len(findings) = %d, want %d (%+v)", len(findings), len(want), findings)
	}
	for i, got := range findings {
		if got != want[i] {
			t.Errorf("findings[%d] = %+v, want %+v", i, got, want[i])
		}
	}
}

func TestParseTrivyReport_InvalidJSON(t *testing.T) {
	if _, err := ParseTrivyReport([]byte("not json")); err == nil {
		t.Fatal("ParseTrivyReport() error = nil, want error for invalid JSON")
	}
}

func TestNormalizeSeverity(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"lowercase", "high", "HIGH"},
		{"already uppercase", "CRITICAL", "CRITICAL"},
		{"mixed case with spaces", "  Medium ", "MEDIUM"},
		{"unknown falls back to low", "UNKNOWN", "LOW"},
		{"empty falls back to low", "", "LOW"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeSeverity(tt.raw); got != tt.want {
				t.Errorf("normalizeSeverity(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestComputeRemediations(t *testing.T) {
	fixedFinding := Finding{
		Tool:        "gosec",
		Severity:    "HIGH",
		RuleID:      "G201",
		Description: "SQL string formatting",
		File:        "internal/storage/postgres.go",
	}
	stillOpenFinding := Finding{
		Tool:        "trivy",
		Severity:    "CRITICAL",
		RuleID:      "CVE-2024-24786",
		Description: "protojson infinite loop",
		File:        "google.golang.org/protobuf",
	}

	before := []Finding{fixedFinding, stillOpenFinding}
	after := []Finding{stillOpenFinding}

	beforeAt := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	afterAt := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)

	remediations := ComputeRemediations(before, beforeAt, after, afterAt)

	if len(remediations) != 1 {
		t.Fatalf("len(remediations) = %d, want 1 (%+v)", len(remediations), remediations)
	}

	got := remediations[0]
	if got.Finding != fixedFinding {
		t.Errorf("remediation.Finding = %+v, want %+v", got.Finding, fixedFinding)
	}
	if !got.OpenedAt.Equal(beforeAt) {
		t.Errorf("remediation.OpenedAt = %v, want %v", got.OpenedAt, beforeAt)
	}
	if !got.ClosedAt.Equal(afterAt) {
		t.Errorf("remediation.ClosedAt = %v, want %v", got.ClosedAt, afterAt)
	}
	wantDuration := afterAt.Sub(beforeAt)
	if got.Duration != wantDuration {
		t.Errorf("remediation.Duration = %v, want %v", got.Duration, wantDuration)
	}

	for _, r := range remediations {
		if r.Finding == stillOpenFinding {
			t.Errorf("stillOpenFinding should not appear in remediations, got %+v", r)
		}
	}
}

func TestComputeRemediations_NoneFixed(t *testing.T) {
	f := Finding{Tool: "gosec", RuleID: "G104", File: "internal/x.go"}
	before := []Finding{f}
	after := []Finding{f}

	remediations := ComputeRemediations(before, time.Now(), after, time.Now())
	if len(remediations) != 0 {
		t.Fatalf("len(remediations) = %d, want 0 (%+v)", len(remediations), remediations)
	}
}

func TestComputeRemediations_EmptyBefore(t *testing.T) {
	remediations := ComputeRemediations(nil, time.Now(), nil, time.Now())
	if len(remediations) != 0 {
		t.Fatalf("len(remediations) = %d, want 0", len(remediations))
	}
}
