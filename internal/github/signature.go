// Package github contém autenticação do GitHub App, validação e parsing de
// webhooks, e um client REST para backfill/reconciliação.
package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

const signaturePrefix = "sha256="

// ValidateSignature confere a assinatura HMAC SHA-256 enviada pelo GitHub no
// header X-Hub-Signature-256 contra o corpo bruto do webhook.
func ValidateSignature(secret string, payload []byte, header string) error {
	if header == "" {
		return fmt.Errorf("validating webhook signature: missing signature header")
	}

	if !strings.HasPrefix(header, signaturePrefix) {
		return fmt.Errorf("validating webhook signature: unsupported signature format")
	}

	expected, err := hex.DecodeString(header[len(signaturePrefix):])
	if err != nil {
		return fmt.Errorf("validating webhook signature: decoding signature: %w", err)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	got := mac.Sum(nil)

	if !hmac.Equal(got, expected) {
		return fmt.Errorf("validating webhook signature: signature mismatch")
	}

	return nil
}
