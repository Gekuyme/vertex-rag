CREATE TABLE documents (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  filename TEXT NOT NULL,
  mime TEXT NOT NULL,
  storage_key TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'uploaded',
  allowed_role_ids BIGINT[] NOT NULL,
  created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE document_chunks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  chunk_index INT NOT NULL,
  content TEXT NOT NULL,
  content_tsv TSVECTOR,
  embedding vector(1536),
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  allowed_role_ids BIGINT[] NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (document_id, chunk_index)
);

CREATE INDEX idx_documents_org ON documents(org_id);
CREATE INDEX idx_documents_status ON documents(status);
CREATE INDEX idx_documents_allowed_roles ON documents USING GIN (allowed_role_ids);
CREATE INDEX idx_document_chunks_org_document ON document_chunks(org_id, document_id);
CREATE INDEX idx_document_chunks_allowed_roles ON document_chunks USING GIN (allowed_role_ids);
CREATE INDEX idx_document_chunks_content_tsv ON document_chunks USING GIN (content_tsv);

