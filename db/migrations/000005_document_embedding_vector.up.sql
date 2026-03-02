ALTER TABLE document_chunks
ALTER COLUMN embedding TYPE vector USING embedding::vector;

