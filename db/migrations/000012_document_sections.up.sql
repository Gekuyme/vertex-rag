CREATE TABLE document_sections (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  section_index INT NOT NULL,
  heading_path TEXT NOT NULL DEFAULT '',
  content TEXT NOT NULL,
  metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
  allowed_role_ids BIGINT[] NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (document_id, section_index)
);

ALTER TABLE document_chunks
ADD COLUMN parent_section_id UUID REFERENCES document_sections(id) ON DELETE CASCADE;

CREATE INDEX idx_document_sections_org_document ON document_sections(org_id, document_id);
CREATE INDEX idx_document_sections_allowed_roles ON document_sections USING GIN (allowed_role_ids);
CREATE INDEX idx_document_chunks_parent_section ON document_chunks(parent_section_id);
