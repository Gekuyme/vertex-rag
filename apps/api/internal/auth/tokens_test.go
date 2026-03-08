package auth

import (
	"testing"
	"time"
)

func TestNewTokenPairIncludesRefreshSessionIDOnlyOnRefreshToken(t *testing.T) {
	manager, err := NewManager("test-secret", time.Minute, time.Hour)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	accessToken, refreshToken, err := manager.NewTokenPair("user-1", "org-1", "session-1")
	if err != nil {
		t.Fatalf("NewTokenPair() error = %v", err)
	}

	accessClaims, err := manager.ParseToken(accessToken, TokenTypeAccess)
	if err != nil {
		t.Fatalf("ParseToken(access) error = %v", err)
	}
	if accessClaims.SessionID != "" {
		t.Fatalf("access SessionID = %q, want empty", accessClaims.SessionID)
	}

	refreshClaims, err := manager.ParseToken(refreshToken, TokenTypeRefresh)
	if err != nil {
		t.Fatalf("ParseToken(refresh) error = %v", err)
	}
	if refreshClaims.SessionID != "session-1" {
		t.Fatalf("refresh SessionID = %q, want session-1", refreshClaims.SessionID)
	}
}

func TestVerifyTokenHash(t *testing.T) {
	token := "sample-refresh-token"
	hash := HashToken(token)

	if !VerifyTokenHash(token, hash) {
		t.Fatalf("VerifyTokenHash() = false, want true")
	}
	if VerifyTokenHash("other-token", hash) {
		t.Fatalf("VerifyTokenHash() = true for mismatched token, want false")
	}
}
