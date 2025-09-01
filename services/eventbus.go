package services

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type EventBus struct{ DB *pgxpool.Pool }

type EventRow struct {
	ID      int64
	Topic   string
	Payload json.RawMessage
}

func NewEventBus(db *pgxpool.Pool) *EventBus { return &EventBus{DB: db} }

func (b *EventBus) Publish(ctx context.Context, topic string, payload any, key *string) error {
	jb, _ := json.Marshal(payload)

	// Если задан event_key — идемпотентная вставка без индекса
	if key != nil {
		k := strings.TrimSpace(*key)
		if k != "" {
			_, err := b.DB.Exec(ctx, `
				INSERT INTO public.integration_events(topic, payload, event_key)
				SELECT $1, $2::jsonb, $3
				WHERE NOT EXISTS (
					SELECT 1 FROM public.integration_events
					WHERE topic = $1 AND event_key = $3
				)
			`, topic, string(jb), k)
			return err
		}
	}

	// Обычная вставка без ключа
	_, err := b.DB.Exec(ctx, `
		INSERT INTO public.integration_events(topic, payload)
		VALUES ($1, $2::jsonb)
	`, topic, string(jb))
	return err
}

func (b *EventBus) Claim(ctx context.Context, topics []string, batch int) ([]EventRow, error) {
	if batch <= 0 || batch > 500 { batch = 100 }
	rows, err := b.DB.Query(ctx, `
    WITH cte AS (
      SELECT id
      FROM public.integration_events
      WHERE status='NEW'
        AND topic = ANY($1)
        AND available_at <= now()
      ORDER BY id
      LIMIT $2
      FOR UPDATE SKIP LOCKED
    )
    UPDATE public.integration_events e
    SET status='PROCESSING', tries = e.tries + 1
    FROM cte
    WHERE e.id = cte.id
    RETURNING e.id, e.topic, e.payload
  `, topics, batch)
	if err != nil { return nil, err }
	defer rows.Close()

	out := make([]EventRow, 0, batch)
	for rows.Next() {
		var r EventRow
		if err := rows.Scan(&r.ID, &r.Topic, &r.Payload); err == nil {
			out = append(out, r)
		}
	}
	return out, nil
}

func (b *EventBus) Ack(ctx context.Context, id int64) error {
	_, err := b.DB.Exec(ctx, `UPDATE public.integration_events SET status='DONE' WHERE id=$1`, id)
	return err
}

func (b *EventBus) Fail(ctx context.Context, id int64, errMsg string, backoff time.Duration) error {
	if backoff <= 0 { backoff = time.Minute }
	_, err := b.DB.Exec(ctx, `
    UPDATE public.integration_events
    SET status='FAILED',
        last_error=$2,
        available_at=now()+$3::interval
    WHERE id=$1
  `, id, errMsg, fmt.Sprintf("%f seconds", backoff.Seconds()))
	return err
}