package github

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func generateTestPrivateKeyPEM(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating RSA key: %v", err)
	}

	block := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}

	return string(pem.EncodeToMemory(block)), key
}

func TestAppAuthenticatorSignJWT(t *testing.T) {
	pemKey, key := generateTestPrivateKeyPEM(t)

	auth, err := NewAppAuthenticator(12345, pemKey)
	if err != nil {
		t.Fatalf("NewAppAuthenticator() error = %v", err)
	}

	tokenStr, err := auth.signJWT()
	if err != nil {
		t.Fatalf("signJWT() error = %v", err)
	}

	claims := &jwt.RegisteredClaims{}
	parsed, err := jwt.ParseWithClaims(tokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		return &key.PublicKey, nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("token failed to verify: err=%v valid=%v", err, parsed.Valid)
	}

	if claims.Issuer != "12345" {
		t.Errorf("Issuer = %q, want %q", claims.Issuer, "12345")
	}
}

func TestNewAppAuthenticatorRejectsInvalidKey(t *testing.T) {
	if _, err := NewAppAuthenticator(1, "not a pem key"); err == nil {
		t.Fatal("NewAppAuthenticator() expected error for invalid key")
	}
}

func TestTokenManagerFetchesAndCachesToken(t *testing.T) {
	pemKey, key := generateTestPrivateKeyPEM(t)
	auth, err := NewAppAuthenticator(1, pemKey)
	if err != nil {
		t.Fatalf("NewAppAuthenticator() error = %v", err)
	}

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++

		wantPath := "/app/installations/99/access_tokens"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q, want %q", r.URL.Path, wantPath)
		}

		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if len(authHeader) < len(prefix) {
			t.Fatalf("missing Authorization header")
		}
		claims := &jwt.RegisteredClaims{}
		_, err := jwt.ParseWithClaims(authHeader[len(prefix):], claims, func(token *jwt.Token) (interface{}, error) {
			return &key.PublicKey, nil
		})
		if err != nil {
			t.Fatalf("invalid app JWT sent to installation token endpoint: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"token": "installation-token-%d", "expires_at": %q}`,
			requests, time.Now().Add(1*time.Hour).Format(time.RFC3339))
	}))
	defer server.Close()

	tm := NewTokenManager(auth, 99, server.Client())
	tm.baseURL = server.URL

	token1, err := tm.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token1 != "installation-token-1" {
		t.Fatalf("token1 = %q, want installation-token-1", token1)
	}

	token2, err := tm.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if token2 != token1 {
		t.Fatalf("expected cached token, got a new one: %q", token2)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1 (second call should use cache)", requests)
	}
}

func TestTokenManagerRefreshesNearExpiry(t *testing.T) {
	pemKey, _ := generateTestPrivateKeyPEM(t)
	auth, err := NewAppAuthenticator(1, pemKey)
	if err != nil {
		t.Fatalf("NewAppAuthenticator() error = %v", err)
	}

	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"token": "token-%d", "expires_at": %q}`,
			requests, time.Now().Add(1*time.Hour).Format(time.RFC3339))
	}))
	defer server.Close()

	tm := NewTokenManager(auth, 1, server.Client())
	tm.baseURL = server.URL

	fakeNow := time.Now()
	tm.now = func() time.Time { return fakeNow }
	auth.now = func() time.Time { return fakeNow }

	if _, err := tm.Token(context.Background()); err != nil {
		t.Fatalf("Token() error = %v", err)
	}

	// Expira em 1h, margem de renovação é 2min: 30 minutos depois ainda
	// está bem antes do limiar de renovação (58min).
	tm.now = func() time.Time { return fakeNow.Add(30 * time.Minute) }
	if _, err := tm.Token(context.Background()); err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1 (still cached before refresh margin)", requests)
	}

	// 59min já passou do limiar de renovação (58min): deve renovar.
	tm.now = func() time.Time { return fakeNow.Add(59 * time.Minute) }
	if _, err := tm.Token(context.Background()); err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2 (should have refreshed)", requests)
	}
}

func TestTokenManagerErrorsOnBadStatus(t *testing.T) {
	pemKey, _ := generateTestPrivateKeyPEM(t)
	auth, err := NewAppAuthenticator(1, pemKey)
	if err != nil {
		t.Fatalf("NewAppAuthenticator() error = %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	tm := NewTokenManager(auth, 1, server.Client())
	tm.baseURL = server.URL

	if _, err := tm.Token(context.Background()); err == nil {
		t.Fatal("Token() expected error on non-201 response")
	}
}
