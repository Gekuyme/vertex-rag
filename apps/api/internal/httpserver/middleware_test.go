package httpserver

import "testing"

func TestHasPermission(t *testing.T) {
	permissions := []string{"can_manage_users", "can_upload_docs"}

	if !hasPermission(permissions, "can_manage_users") {
		t.Fatalf("expected permission to exist")
	}

	if hasPermission(permissions, "can_manage_roles") {
		t.Fatalf("expected permission to be missing")
	}
}

func TestExtractBearerToken(t *testing.T) {
	token := extractBearerToken("Bearer abc123")
	if token != "abc123" {
		t.Fatalf("unexpected token: %q", token)
	}

	if extractBearerToken("abc123") != "" {
		t.Fatalf("expected empty token for invalid header")
	}
}
