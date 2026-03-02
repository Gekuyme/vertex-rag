package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
	"github.com/hibiken/asynq"
)

const TypeIngestDocument = "ingest:document"

type IngestDocumentPayload struct {
	DocumentID string `json:"document_id"`
}

type Client struct {
	client *asynq.Client
}

func NewClient(cfg config.RedisConfig) *Client {
	return &Client{
		client: asynq.NewClient(asynq.RedisClientOpt{
			Addr:     cfg.Addr,
			Password: cfg.Password,
			DB:       cfg.DB,
		}),
	}
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) EnqueueDocumentIngest(ctx context.Context, documentID string) error {
	payload, err := json.Marshal(IngestDocumentPayload{DocumentID: documentID})
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	_, err = c.client.EnqueueContext(ctx, asynq.NewTask(TypeIngestDocument, payload))
	if err != nil {
		return fmt.Errorf("enqueue ingest document task: %w", err)
	}

	return nil
}
