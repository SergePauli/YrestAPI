package auth

import (
	"YrestAPI/internal/config"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
	"time"
)

type contextKey string

const claimsContextKey contextKey = "jwt_claims"

type JWTValidator struct {
	cfg       config.JWTConfig
	rsaKey    *rsa.PublicKey
	ecdsaKey  *ecdsa.PublicKey
	hmacKey   []byte
	expected  string
	clockFunc func() time.Time
}

func NewJWTValidator(cfg config.JWTConfig) (*JWTValidator, error) {
	if strings.TrimSpace(cfg.Issuer) == "" {
		return nil, errors.New("jwt issuer is required")
	}
	if strings.TrimSpace(cfg.Audience) == "" {
		return nil, errors.New("jwt audience is required")
	}
	alg := strings.ToUpper(strings.TrimSpace(cfg.ValidationType))
	if alg == "" {
		return nil, errors.New("jwt validation type is required")
	}

	v := &JWTValidator{
		cfg:       cfg,
		expected:  alg,
		clockFunc: time.Now,
	}

	switch alg {
	case "HS256":
		if cfg.HMACSecret == "" {
			return nil, errors.New("jwt hmac secret is required for HS256")
		}
		v.hmacKey = []byte(cfg.HMACSecret)
	case "RS256":
		pubKey, err := loadPublicKey(cfg)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := pubKey.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("jwt public key is not RSA")
		}
		v.rsaKey = rsaKey
	case "ES256":
		pubKey, err := loadPublicKey(cfg)
		if err != nil {
			return nil, err
		}
		ecdsaKey, ok := pubKey.(*ecdsa.PublicKey)
		if !ok {
			return nil, errors.New("jwt public key is not ECDSA")
		}
		v.ecdsaKey = ecdsaKey
	default:
		return nil, fmt.Errorf("unsupported jwt validation type: %s", cfg.ValidationType)
	}

	return v, nil
}

func (v *JWTValidator) ValidateToken(token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid jwt format")
	}

	var header map[string]any
	if err := decodeSegment(parts[0], &header); err != nil {
		return nil, fmt.Errorf("invalid jwt header: %w", err)
	}
	alg, _ := header["alg"].(string)
	if strings.ToUpper(alg) != v.expected {
		return nil, fmt.Errorf("unexpected jwt alg: %s", alg)
	}

	signingInput := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("invalid jwt signature encoding")
	}
	if err := v.verifySignature(signingInput, signature); err != nil {
		return nil, err
	}

	var claims map[string]any
	if err := decodeSegment(parts[1], &claims); err != nil {
		return nil, fmt.Errorf("invalid jwt claims: %w", err)
	}
	if err := v.validateClaims(claims); err != nil {
		return nil, err
	}

	return claims, nil
}

func WithClaims(ctx context.Context, claims map[string]any) context.Context {
	return context.WithValue(ctx, claimsContextKey, claims)
}

func ClaimsFromContext(ctx context.Context) (map[string]any, bool) {
	claims, ok := ctx.Value(claimsContextKey).(map[string]any)
	return claims, ok
}

func (v *JWTValidator) verifySignature(signingInput string, signature []byte) error {
	hash := sha256.Sum256([]byte(signingInput))
	switch v.expected {
	case "HS256":
		mac := hmac.New(sha256.New, v.hmacKey)
		_, _ = mac.Write([]byte(signingInput))
		expected := mac.Sum(nil)
		if !hmac.Equal(expected, signature) {
			return errors.New("invalid jwt signature")
		}
	case "RS256":
		if err := rsa.VerifyPKCS1v15(v.rsaKey, crypto.SHA256, hash[:], signature); err != nil {
			return errors.New("invalid jwt signature")
		}
	case "ES256":
		if len(signature) != 64 {
			return errors.New("invalid jwt signature length")
		}
		r := new(big.Int).SetBytes(signature[:32])
		s := new(big.Int).SetBytes(signature[32:])
		if !ecdsa.Verify(v.ecdsaKey, hash[:], r, s) {
			return errors.New("invalid jwt signature")
		}
	}
	return nil
}

func (v *JWTValidator) validateClaims(claims map[string]any) error {
	now := v.clockFunc().Unix()
	skew := v.cfg.ClockSkewSec
	if skew < 0 {
		skew = 0
	}

	iss, _ := claims["iss"].(string)
	if iss != v.cfg.Issuer {
		return errors.New("invalid jwt issuer")
	}

	if !isAudienceValid(claims["aud"], v.cfg.Audience) {
		return errors.New("invalid jwt audience")
	}

	exp, err := numericClaim(claims, "exp")
	if err != nil {
		return err
	}
	if now > exp+skew {
		return errors.New("jwt is expired")
	}

	nbf, err := numericClaim(claims, "nbf")
	if err != nil {
		return err
	}
	if now+skew < nbf {
		return errors.New("jwt is not valid yet")
	}

	iat, err := numericClaim(claims, "iat")
	if err != nil {
		return err
	}
	if iat > now+skew {
		return errors.New("jwt issued in the future")
	}

	return nil
}

func decodeSegment(segment string, out any) error {
	payload, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, out)
}

func numericClaim(claims map[string]any, key string) (int64, error) {
	raw, ok := claims[key]
	if !ok {
		return 0, fmt.Errorf("jwt claim %s is required", key)
	}
	switch v := raw.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0, fmt.Errorf("invalid jwt claim %s", key)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("invalid jwt claim %s", key)
	}
}

func isAudienceValid(raw any, expected string) bool {
	switch aud := raw.(type) {
	case string:
		return aud == expected
	case []any:
		for _, item := range aud {
			if s, ok := item.(string); ok && s == expected {
				return true
			}
		}
	case []string:
		for _, s := range aud {
			if s == expected {
				return true
			}
		}
	}
	return false
}

func loadPublicKey(cfg config.JWTConfig) (any, error) {
	keyPEM := strings.TrimSpace(cfg.PublicKeyPEM)
	if keyPEM == "" && strings.TrimSpace(cfg.PublicKeyPath) != "" {
		data, err := os.ReadFile(cfg.PublicKeyPath)
		if err != nil {
			return nil, fmt.Errorf("read jwt public key: %w", err)
		}
		keyPEM = string(data)
	}
	if keyPEM == "" {
		return nil, errors.New("jwt public key is required")
	}

	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, errors.New("invalid jwt public key pem")
	}

	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		return pub, nil
	}
	if pub, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return pub, nil
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err == nil {
		return cert.PublicKey, nil
	}
	return nil, errors.New("unsupported jwt public key format")
}
