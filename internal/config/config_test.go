package config

import (
	"testing"
)

func setValidEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GITHUB_APP_ID", "12345")
	t.Setenv("GITHUB_APP_PRIVATE_KEY", "-----BEGIN RSA PRIVATE KEY-----\nfake\n-----END RSA PRIVATE KEY-----")
	t.Setenv("GITHUB_WEBHOOK_SECRET", "s3cr3t")
	t.Setenv("PSEUDONYM_SALT", "salt")
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/kaizenops")
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T)
		wantErr bool
	}{
		{
			name:  "valid config",
			setup: setValidEnv,
		},
		{
			name: "missing app id",
			setup: func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("GITHUB_APP_ID", "")
			},
			wantErr: true,
		},
		{
			name: "invalid app id",
			setup: func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("GITHUB_APP_ID", "not-a-number")
			},
			wantErr: true,
		},
		{
			name: "missing private key",
			setup: func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("GITHUB_APP_PRIVATE_KEY", "")
			},
			wantErr: true,
		},
		{
			name: "missing webhook secret",
			setup: func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("GITHUB_WEBHOOK_SECRET", "")
			},
			wantErr: true,
		},
		{
			name: "missing pseudonym salt",
			setup: func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("PSEUDONYM_SALT", "")
			},
			wantErr: true,
		},
		{
			name: "missing database url",
			setup: func(t *testing.T) {
				setValidEnv(t)
				t.Setenv("DATABASE_URL", "")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setup(t)

			cfg, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			if cfg.GitHubAppID != 12345 {
				t.Errorf("GitHubAppID = %d, want 12345", cfg.GitHubAppID)
			}
			if cfg.HTTPAddr != ":8080" {
				t.Errorf("HTTPAddr = %q, want default :8080", cfg.HTTPAddr)
			}
		})
	}
}

func TestLoadHTTPAddrOverride(t *testing.T) {
	setValidEnv(t)
	t.Setenv("HTTP_ADDR", ":9090")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if cfg.HTTPAddr != ":9090" {
		t.Errorf("HTTPAddr = %q, want :9090", cfg.HTTPAddr)
	}
}
