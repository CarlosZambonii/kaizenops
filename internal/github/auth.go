package github

import (
	"context"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// appJWTTTL é o tempo de vida do JWT de App. O GitHub aceita no máximo 10
// minutos; usamos um pouco menos para dar margem de segurança.
const appJWTTTL = 9 * time.Minute

// tokenRefreshMargin é quanto antes do vencimento renovamos o installation
// token, para nunca fazer uma chamada à API do GitHub com um token expirado.
const tokenRefreshMargin = 2 * time.Minute

// AppAuthenticator assina JWTs de GitHub App com a chave privada RSA do App.
type AppAuthenticator struct {
	appID      int64
	privateKey *rsa.PrivateKey
	now        func() time.Time
}

// NewAppAuthenticator cria um AppAuthenticator a partir do ID do App e da
// chave privada em PEM (formato PKCS#1 ou PKCS#8).
func NewAppAuthenticator(appID int64, privateKeyPEM string) (*AppAuthenticator, error) {
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(privateKeyPEM))
	if err != nil {
		return nil, fmt.Errorf("parsing GitHub App private key: %w", err)
	}

	return &AppAuthenticator{
		appID:      appID,
		privateKey: key,
		now:        time.Now,
	}, nil
}

func (a *AppAuthenticator) signJWT() (string, error) {
	now := a.now()
	claims := jwt.RegisteredClaims{
		// tolera alguma diferença de relógio com o servidor do GitHub.
		IssuedAt:  jwt.NewNumericDate(now.Add(-30 * time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(appJWTTTL)),
		Issuer:    strconv.FormatInt(a.appID, 10),
	}

	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(a.privateKey)
	if err != nil {
		return "", fmt.Errorf("signing GitHub App JWT: %w", err)
	}

	return token, nil
}

// TokenManager mantém um installation access token válido, renovando-o
// automaticamente antes do vencimento.
type TokenManager struct {
	auth           *AppAuthenticator
	installationID int64
	httpClient     *http.Client
	baseURL        string
	now            func() time.Time

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

// NewTokenManager cria um TokenManager para a instalação installationID do
// App autenticado por auth.
func NewTokenManager(auth *AppAuthenticator, installationID int64, httpClient *http.Client) *TokenManager {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &TokenManager{
		auth:           auth,
		installationID: installationID,
		httpClient:     httpClient,
		baseURL:        "https://api.github.com",
		now:            time.Now,
	}
}

type installationTokenResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Token retorna um installation access token válido, buscando um novo se o
// atual estiver ausente ou perto do vencimento.
func (tm *TokenManager) Token(ctx context.Context) (string, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := tm.now()
	if tm.token != "" && now.Before(tm.expiresAt.Add(-tokenRefreshMargin)) {
		return tm.token, nil
	}

	jwtStr, err := tm.auth.signJWT()
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/app/installations/%d/access_tokens", tm.baseURL, tm.installationID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return "", fmt.Errorf("building installation token request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwtStr)
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := tm.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting installation token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("requesting installation token: unexpected status %d", resp.StatusCode)
	}

	var result installationTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decoding installation token response: %w", err)
	}

	tm.token = result.Token
	tm.expiresAt = result.ExpiresAt

	return tm.token, nil
}
