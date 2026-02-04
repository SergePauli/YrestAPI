package auth

import (
	"YrestAPI/internal/config"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"testing"
	"time"
)

func TestHS256ValidateToken(t *testing.T) {
	now := time.Unix(1730000000, 0)
	cfg := config.JWTConfig{
		ValidationType: "HS256",
		Issuer:         "auth-service",
		Audience:       "yrest-api",
		HMACSecret:     "super-secret",
		ClockSkewSec:   0,
	}

	v, err := NewJWTValidator(cfg)
	if err != nil {
		t.Fatalf("NewJWTValidator failed: %v", err)
	}
	v.clockFunc = func() time.Time { return now }

	token := buildHS256Token(t, cfg.HMACSecret, map[string]any{
		"iss": cfg.Issuer,
		"aud": cfg.Audience,
		"iat": now.Unix() - 10,
		"nbf": now.Unix() - 5,
		"exp": now.Unix() + 30,
		"sub": "user-1",
	})

	claims, err := v.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
	if claims["sub"] != "user-1" {
		t.Fatalf("unexpected sub: %v", claims["sub"])
	}
}

func TestValidateTokenExpired(t *testing.T) {
	now := time.Unix(1730000000, 0)
	cfg := config.JWTConfig{
		ValidationType: "HS256",
		Issuer:         "auth-service",
		Audience:       "yrest-api",
		HMACSecret:     "super-secret",
		ClockSkewSec:   0,
	}

	v, err := NewJWTValidator(cfg)
	if err != nil {
		t.Fatalf("NewJWTValidator failed: %v", err)
	}
	v.clockFunc = func() time.Time { return now }

	token := buildHS256Token(t, cfg.HMACSecret, map[string]any{
		"iss": cfg.Issuer,
		"aud": cfg.Audience,
		"iat": now.Unix() - 20,
		"nbf": now.Unix() - 20,
		"exp": now.Unix() - 1,
	})

	if _, err := v.ValidateToken(token); err == nil {
		t.Fatalf("expected expired error")
	}
}

func TestRS256ValidateToken(t *testing.T) {
	now := time.Unix(1730000000, 0)
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey failed: %v", err)
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	cfg := config.JWTConfig{
		ValidationType: "RS256",
		Issuer:         "auth-service",
		Audience:       "yrest-api",
		PublicKeyPEM:   string(pubPEM),
		ClockSkewSec:   0,
	}

	v, err := NewJWTValidator(cfg)
	if err != nil {
		t.Fatalf("NewJWTValidator failed: %v", err)
	}
	v.clockFunc = func() time.Time { return now }

	token := buildRS256Token(t, priv, map[string]any{
		"iss": cfg.Issuer,
		"aud": cfg.Audience,
		"iat": now.Unix() - 10,
		"nbf": now.Unix() - 10,
		"exp": now.Unix() + 60,
	})

	if _, err := v.ValidateToken(token); err != nil {
		t.Fatalf("ValidateToken failed: %v", err)
	}
}

func buildHS256Token(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	headerPart := encodePart(t, header)
	claimsPart := encodePart(t, claims)
	signingInput := headerPart + "." + claimsPart

	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(signingInput))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return signingInput + "." + signature
}

func buildRS256Token(t *testing.T, priv *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "typ": "JWT"}
	headerPart := encodePart(t, header)
	claimsPart := encodePart(t, claims)
	signingInput := headerPart + "." + claimsPart

	hash := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15 failed: %v", err)
	}
	signature := base64.RawURLEncoding.EncodeToString(sig)
	return signingInput + "." + signature
}

func encodePart(t *testing.T, data map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}
