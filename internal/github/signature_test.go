package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func sign(t *testing.T, secret string, payload []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return signaturePrefix + hex.EncodeToString(mac.Sum(nil))
}

func TestValidateSignature(t *testing.T) {
	secret := "s3cr3t"
	payload := []byte(`{"action":"completed"}`)
	validHeader := sign(t, secret, payload)

	tests := []struct {
		name    string
		secret  string
		payload []byte
		header  string
		wantErr bool
	}{
		{name: "valid signature", secret: secret, payload: payload, header: validHeader},
		{name: "missing header", secret: secret, payload: payload, header: "", wantErr: true},
		{name: "unsupported format", secret: secret, payload: payload, header: "sha1=deadbeef", wantErr: true},
		{name: "invalid hex", secret: secret, payload: payload, header: "sha256=not-hex", wantErr: true},
		{name: "wrong secret", secret: "other-secret", payload: payload, header: validHeader, wantErr: true},
		{name: "tampered payload", secret: secret, payload: []byte(`{"action":"tampered"}`), header: validHeader, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSignature(tt.secret, tt.payload, tt.header)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateSignature() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
