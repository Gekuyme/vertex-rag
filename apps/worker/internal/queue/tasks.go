package queue

const TypeIngestDocument = "ingest:document"

type IngestDocumentPayload struct {
	DocumentID string `json:"document_id"`
}
