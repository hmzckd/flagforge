package stream

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

type Listener struct {
	databaseURL string
	channel     string
	hub         *Hub
	logger      *slog.Logger

	MinBackoff time.Duration
	MaxBackoff time.Duration

	listening atomic.Bool
	connects  atomic.Int64
}

func NewListener(databaseURL, channel string, hub *Hub, logger *slog.Logger) *Listener {
	return &Listener{
		databaseURL: databaseURL,
		channel:     channel,
		hub:         hub,
		logger:      logger,
		MinBackoff:  time.Second,
		MaxBackoff:  30 * time.Second,
	}
}

func (l *Listener) Listening() bool {
	return l.listening.Load()
}

func (l *Listener) Connects() int64 {
	return l.connects.Load()
}

func (l *Listener) Run(ctx context.Context) {
	backoff := l.MinBackoff
	for {
		start := time.Now()
		err := l.listen(ctx)
		l.listening.Store(false)
		if ctx.Err() != nil {
			return
		}
		if time.Since(start) > l.MaxBackoff {
			backoff = l.MinBackoff
		}
		l.logger.Warn("change listener disconnected, reconnecting", "error", err, "retry_in", backoff.String())

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, l.MaxBackoff)
	}
}

func (l *Listener) listen(ctx context.Context) error {
	cfg, err := pgx.ParseConfig(l.databaseURL)
	if err != nil {
		return err
	}
	cfg.RuntimeParams["application_name"] = "flagforge-listener"

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(context.Background()) }()

	if _, err := conn.Exec(ctx, "LISTEN "+pgx.Identifier{l.channel}.Sanitize()); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	l.listening.Store(true)
	l.connects.Add(1)
	l.logger.Info("change listener connected", "channel", l.channel)

	l.hub.NotifyAll()

	for {
		n, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		l.hub.Notify(n.Payload)
	}
}
