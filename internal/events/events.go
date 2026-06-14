// Package events bridges the worker and API processes using PostgreSQL
// LISTEN/NOTIFY, so status and metric updates produced by the worker reach
// WebSocket clients connected to the API.
package events

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
)

// channel is the Postgres NOTIFY channel used for all Porque events.
const channel = "porque_events"

// envelope wraps a topic with its JSON payload.
type envelope struct {
	Topic   string          `json:"topic"`
	Payload json.RawMessage `json:"payload"`
}

// Notifier publishes events via pg_notify. It satisfies the StatusPublisher
// interface used by the lifecycle controller and tunnel/worker code.
type Notifier struct {
	db *sqlx.DB
}

// NewNotifier creates a Notifier over an existing pool.
func NewNotifier(db *sqlx.DB) *Notifier { return &Notifier{db: db} }

// PublishStatus marshals payload and emits it on the Porque NOTIFY channel.
func (n *Notifier) PublishStatus(topic string, payload any) {
	pb, err := json.Marshal(payload)
	if err != nil {
		return
	}
	body, err := json.Marshal(envelope{Topic: topic, Payload: pb})
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := n.db.ExecContext(ctx, "SELECT pg_notify($1, $2)", channel, string(body)); err != nil {
		log.Printf("events: notify failed: %v", err)
	}
}

// Sink receives decoded events (the API's ws.Hub satisfies this).
type Sink interface {
	PublishStatus(topic string, payload any)
}

// Listen subscribes to the NOTIFY channel and forwards each event to sink until
// ctx is cancelled. It reconnects on transient failures.
func Listen(ctx context.Context, dsn string, sink Sink) {
	for ctx.Err() == nil {
		if err := listenOnce(ctx, dsn, sink); err != nil && ctx.Err() == nil {
			log.Printf("events: listener error, reconnecting in 2s: %v", err)
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
			}
		}
	}
}

func listenOnce(ctx context.Context, dsn string, sink Sink) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "LISTEN "+channel); err != nil {
		return err
	}
	log.Printf("events: listening on %q", channel)

	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var env envelope
		if err := json.Unmarshal([]byte(n.Payload), &env); err != nil {
			continue
		}
		// Forward the raw payload; the hub re-marshals it unchanged.
		sink.PublishStatus(env.Topic, env.Payload)
	}
}
