package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Gekuyme/vertex-rag/apps/api/internal/config"
	"github.com/redis/go-redis/v9"
)

type Client struct {
	enabled               bool
	retrievalTTL          time.Duration
	answerTTL             time.Duration
	unstrictAnswerEnabled bool
	redis                 *redis.Client
}

type TopDocStat struct {
	DocumentID string  `json:"document_id"`
	Score      float64 `json:"score"`
}

func NewClient(redisCfg config.RedisConfig, cacheCfg config.CacheConfig) *Client {
	client := &Client{
		enabled:               cacheCfg.Enabled,
		retrievalTTL:          cacheCfg.RetrievalTTL,
		answerTTL:             cacheCfg.AnswerTTL,
		unstrictAnswerEnabled: cacheCfg.UnstrictAnswerEnabled,
	}

	if !cacheCfg.Enabled {
		return client
	}

	client.redis = redis.NewClient(&redis.Options{
		Addr:     redisCfg.Addr,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
	})

	return client
}

func (c *Client) Close() error {
	if c == nil || c.redis == nil {
		return nil
	}

	return c.redis.Close()
}

func (c *Client) Enabled() bool {
	return c != nil && c.enabled && c.redis != nil
}

func (c *Client) RetrievalTTL() time.Duration {
	if c == nil {
		return 0
	}
	return c.retrievalTTL
}

func (c *Client) AnswerTTL() time.Duration {
	if c == nil {
		return 0
	}
	return c.answerTTL
}

func (c *Client) UnstrictAnswerEnabled() bool {
	return c != nil && c.unstrictAnswerEnabled
}

func (c *Client) GetJSON(ctx context.Context, key string, destination any) (bool, error) {
	if !c.Enabled() {
		return false, nil
	}

	payload, err := c.redis.Get(ctx, key).Result()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, fmt.Errorf("cache get %s: %w", key, err)
	}

	if err := json.Unmarshal([]byte(payload), destination); err != nil {
		return false, fmt.Errorf("cache decode %s: %w", key, err)
	}

	return true, nil
}

func (c *Client) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	if !c.Enabled() || ttl <= 0 {
		return nil
	}

	payload, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache encode %s: %w", key, err)
	}

	if err := c.redis.Set(ctx, key, payload, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %s: %w", key, err)
	}

	return nil
}

func (c *Client) IncrementTopDocCounter(ctx context.Context, orgID, documentID string, increment float64) error {
	if !c.Enabled() {
		return nil
	}

	if orgID == "" || documentID == "" || increment == 0 {
		return nil
	}

	key := fmt.Sprintf("rag:v1:top_docs:%s", orgID)
	if err := c.redis.ZIncrBy(ctx, key, increment, documentID).Err(); err != nil {
		return fmt.Errorf("cache incr top doc %s: %w", documentID, err)
	}

	return nil
}

func (c *Client) GetTopDocStats(ctx context.Context, orgID string, limit int64) ([]TopDocStat, error) {
	if !c.Enabled() || orgID == "" {
		return []TopDocStat{}, nil
	}
	if limit <= 0 {
		limit = 10
	}

	key := fmt.Sprintf("rag:v1:top_docs:%s", orgID)
	items, err := c.redis.ZRevRangeWithScores(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, fmt.Errorf("cache get top docs: %w", err)
	}

	stats := make([]TopDocStat, 0, len(items))
	for _, item := range items {
		member, ok := item.Member.(string)
		if !ok {
			continue
		}
		stats = append(stats, TopDocStat{
			DocumentID: member,
			Score:      item.Score,
		})
	}

	return stats, nil
}
