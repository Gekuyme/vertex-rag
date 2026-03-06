CREATE INDEX IF NOT EXISTS idx_document_chunks_embedding_hnsw_256
ON document_chunks
USING hnsw ((embedding::vector(256)) vector_cosine_ops)
WHERE embedding IS NOT NULL AND vector_dims(embedding) = 256;

CREATE INDEX IF NOT EXISTS idx_document_chunks_embedding_hnsw_768
ON document_chunks
USING hnsw ((embedding::vector(768)) vector_cosine_ops)
WHERE embedding IS NOT NULL AND vector_dims(embedding) = 768;
