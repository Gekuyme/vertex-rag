package httpserver

import "testing"

func TestBuildRetrievalCacheKey_ChangesOnKBVersion(t *testing.T) {
	s := &Server{}

	keyV1 := s.buildRetrievalCacheKey("org-1", 10, "strict", 1, "policy update", 8, 32)
	keyV2 := s.buildRetrievalCacheKey("org-1", 10, "strict", 2, "policy update", 8, 32)

	if keyV1 == keyV2 {
		t.Fatalf("expected retrieval cache key to include kb_version")
	}
}

func TestBuildRetrievalCacheKey_ChangesOnRoleID(t *testing.T) {
	s := &Server{}

	keyRoleA := s.buildRetrievalCacheKey("org-1", 10, "strict", 5, "policy update", 8, 32)
	keyRoleB := s.buildRetrievalCacheKey("org-1", 11, "strict", 5, "policy update", 8, 32)

	if keyRoleA == keyRoleB {
		t.Fatalf("expected retrieval cache key to isolate role_id")
	}
}

func TestBuildAnswerCacheKey_NormalizesQuery(t *testing.T) {
	s := &Server{}

	keyA := s.buildAnswerCacheKey("org-1", 10, "strict", 7, "  What   Is Policy  ", 8, 32)
	keyB := s.buildAnswerCacheKey("org-1", 10, "strict", 7, "what is policy", 8, 32)

	if keyA != keyB {
		t.Fatalf("expected answer cache key query normalization to be stable")
	}
}
