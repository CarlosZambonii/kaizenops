// Package config carrega e valida a configuração do collector a partir de
// variáveis de ambiente.
package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config reúne tudo que o collector precisa para autenticar no GitHub,
// validar webhooks e persistir dados no TimescaleDB.
type Config struct {
	// GitHubAppID é o ID do GitHub App usado para autenticação.
	GitHubAppID int64
	// GitHubAppPrivateKeyPEM é a chave privada RSA do GitHub App, em PEM.
	GitHubAppPrivateKeyPEM string
	// GitHubWebhookSecret valida a assinatura HMAC SHA-256 dos webhooks.
	GitHubWebhookSecret string
	// PseudonymSalt é o salt usado para pseudonimizar o autor na ingestão.
	PseudonymSalt string
	// DatabaseURL é a connection string do TimescaleDB.
	DatabaseURL string
	// HTTPAddr é o endereço em que o servidor de webhooks escuta.
	HTTPAddr string
}

// Load lê a configuração das variáveis de ambiente e falha rápido se algo
// obrigatório estiver ausente ou malformado.
func Load() (Config, error) {
	cfg := Config{
		GitHubAppPrivateKeyPEM: os.Getenv("GITHUB_APP_PRIVATE_KEY"),
		GitHubWebhookSecret:    os.Getenv("GITHUB_WEBHOOK_SECRET"),
		PseudonymSalt:          os.Getenv("PSEUDONYM_SALT"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		HTTPAddr:               envOrDefault("HTTP_ADDR", ":8080"),
	}

	appID, err := parseInt64Env("GITHUB_APP_ID")
	if err != nil {
		return Config{}, err
	}
	cfg.GitHubAppID = appID

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) validate() error {
	var missing []string

	if c.GitHubAppID == 0 {
		missing = append(missing, "GITHUB_APP_ID")
	}
	if c.GitHubAppPrivateKeyPEM == "" {
		missing = append(missing, "GITHUB_APP_PRIVATE_KEY")
	}
	if c.GitHubWebhookSecret == "" {
		missing = append(missing, "GITHUB_WEBHOOK_SECRET")
	}
	if c.PseudonymSalt == "" {
		missing = append(missing, "PSEUDONYM_SALT")
	}
	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}

	if len(missing) > 0 {
		return fmt.Errorf("loading config: missing required env vars: %v", missing)
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseInt64Env(key string) (int64, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return 0, nil
	}

	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing %s: %w", key, err)
	}

	return v, nil
}
