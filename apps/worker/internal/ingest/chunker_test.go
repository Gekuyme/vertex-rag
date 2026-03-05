package ingest

import "testing"

func TestNormalizeText_PreservesMarkdownHeadingLineBreak(t *testing.T) {
	normalized := normalizeText("# Title\nSome wrapped\ntext", normalizeOptions{})
	expected := "# Title\nSome wrapped text"
	if normalized != expected {
		t.Fatalf("unexpected normalized text:\n%q\nexpected:\n%q", normalized, expected)
	}
}

func TestNormalizeText_JoinsHyphenatedLineBreaks(t *testing.T) {
	normalized := normalizeText("exam-\nple", normalizeOptions{})
	if normalized != "example" {
		t.Fatalf("expected hyphenated break to be joined, got %q", normalized)
	}
}

func TestNormalizeText_RemovesSoftHyphenArtifacts(t *testing.T) {
	normalized := normalizeText("со\u00ad\nдержащихся", normalizeOptions{})
	if normalized != "содержащихся" {
		t.Fatalf("expected soft hyphen artifact to be removed, got %q", normalized)
	}
}

func TestChunkDocumentText_PDFAddsPageMetadata(t *testing.T) {
	raw := "Page1 token\fPage2 token"
	chunks := chunkDocumentText(raw, "application/pdf", "test.pdf")
	if len(chunks) != 1 {
		t.Fatalf("expected a single chunk, got %d", len(chunks))
	}

	page, ok := chunks[0].Metadata["page"].(int)
	if !ok || page != 1 {
		t.Fatalf("expected page metadata=1, got %#v", chunks[0].Metadata["page"])
	}
	pageEnd, ok := chunks[0].Metadata["page_end"].(int)
	if !ok || pageEnd != 2 {
		t.Fatalf("expected page_end metadata=2, got %#v", chunks[0].Metadata["page_end"])
	}
}
