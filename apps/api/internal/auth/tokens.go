package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
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
	SessionID string    `json:"sid,omitempty"`
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

func (m *Manager) NewTokenPair(userID, orgID, sessionID string) (string, string, error) {
	accessToken, err := m.newToken(userID, orgID, "", TokenTypeAccess, m.accessTTL)
	if err != nil {
		return "", "", fmt.Errorf("generate access token: %w", err)
	}

	refreshToken, err := m.newToken(userID, orgID, sessionID, TokenTypeRefresh, m.refreshTTL)
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

func NewSessionID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}

	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		bytes[0:4],
		bytes[4:6],
		bytes[6:8],
		bytes[8:10],
		bytes[10:16],
	), nil
}

func HashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func VerifyTokenHash(rawToken, expectedHash string) bool {
	actualHash := HashToken(rawToken)
	return subtle.ConstantTimeCompare([]byte(actualHash), []byte(expectedHash)) == 1
}

func (m *Manager) newToken(userID, orgID, sessionID string, tokenType TokenType, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		OrgID:     orgID,
		SessionID: sessionID,
		TokenType: tokenType,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString(m.secret)
}
