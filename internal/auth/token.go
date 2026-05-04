package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenManager struct {
	issuer     string
	audience   string
	signing    []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}
type Claims struct {
	UserID    string `json:"uid"`
	TokenType string `json:"typ"`
	jwt.RegisteredClaims
}

func NewTokenManager(secret string, accessTTL, refreshTTL time.Duration) *TokenManager {
	return &TokenManager{issuer: "yalinli-api", audience: "yalinli-bff", signing: []byte(secret), accessTTL: accessTTL, refreshTTL: refreshTTL}
}

func (m *TokenManager) Mint(userID, typ string) (string, time.Time, error) {
	now := time.Now().UTC()
	ttl := m.accessTTL
	if typ == "refresh" {
		ttl = m.refreshTTL
	}
	exp := now.Add(ttl)
	claims := Claims{UserID: userID, TokenType: typ, RegisteredClaims: jwt.RegisteredClaims{Issuer: m.issuer, Subject: userID, Audience: []string{m.audience}, ExpiresAt: jwt.NewNumericDate(exp), IssuedAt: jwt.NewNumericDate(now), NotBefore: jwt.NewNumericDate(now)}}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := t.SignedString(m.signing)
	return s, exp, err
}

func (m *TokenManager) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, errors.New("invalid signing method")
		}
		return m.signing, nil
	}, jwt.WithIssuer(m.issuer), jwt.WithAudience(m.audience))
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
