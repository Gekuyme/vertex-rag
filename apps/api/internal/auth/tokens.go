package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

type Manager struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

type Claims struct {
	UserID    string    `json:"uid"`
	OrgID     string    `json:"oid"`
	TokenType TokenType `json:"typ"`
	jwt.RegisteredClaims
}

func NewManager(secret string, accessTTL, refreshTTL time.Duration) (*Manager, error) {
	if secret == "" {
		return nil, errors.New("jwt secret cannot be empty")
	}

	return &Manager{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}, nil
}

func (m *Manager) NewTokenPair(userID, orgID string) (string, string, error) {
	accessToken, err := m.newToken(userID, orgID, TokenTypeAccess, m.accessTTL)
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := m.newToken(userID, orgID, TokenTypeRefresh, m.refreshTTL)
	if err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

func (m *Manager) ParseToken(rawToken string, expectedType TokenType) (*Claims, error) {
	token, err := jwt.ParseWithClaims(rawToken, &Claims{}, func(_ *jwt.Token) (interface{}, error) {
		return m.secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	if claims.TokenType != expectedType {
		return nil, fmt.Errorf("unexpected token type: %s", claims.TokenType)
	}

	return claims, nil
}

func (m *Manager) AccessTTL() time.Duration {
	return m.accessTTL
}

func (m *Manager) RefreshTTL() time.Duration {
	return m.refreshTTL
}

func (m *Manager) newToken(userID, orgID string, tokenType TokenType, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		OrgID:     orgID,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString(m.secret)
}
