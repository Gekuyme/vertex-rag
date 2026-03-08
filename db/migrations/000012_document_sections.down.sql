DROP INDEX IF EXISTS idx_document_chunks_parent_section;
DROP INDEX IF EXISTS idx_document_sections_allowed_roles;
DROP INDEX IF EXISTS idx_document_sections_org_document;

ALTER TABLE document_chunks
DROP COLUMN IF EXISTS parent_section_id;

DROP TABLE IF EXISTS document_sections;
