package sdk

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

func (c *Client) runStream(ctx context.Context) {
	defer c.wg.Done()

	attempt := 0
	for {
		connectedAt := time.Now()
		err := c.streamOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, errKeyRejected) {
			c.cfg.Logger.Error("flagforge stopped live updates, using last known flags", "error", err)
			return
		}
		if time.Since(connectedAt) > c.cfg.ReconnectMaxDelay {
			attempt = 0
		}

		delay := backoff(attempt, c.cfg.ReconnectMinDelay, c.cfg.ReconnectMaxDelay)
		attempt++
		c.cfg.Logger.Warn("flagforge stream disconnected, using last known flags",
			"error", err, "retry_in", delay.Round(time.Millisecond).String())

		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
}

func backoff(attempt int, minDelay, maxDelay time.Duration) time.Duration {
	ceiling := maxDelay
	if attempt < 30 {
		ceiling = min(minDelay<<attempt, maxDelay)
	}
	return rand.N(ceiling + 1)
}

func (c *Client) streamOnce(ctx context.Context) error {
	connCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(connCtx, http.MethodGet, c.cfg.BaseURL+"/sdk/v1/stream", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.SDKKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return errKeyRejected
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("stream returned status %d", resp.StatusCode)
	}
	c.cfg.Logger.Info("flagforge stream connected")

	idle := time.AfterFunc(c.cfg.StreamIdleTimeout, cancel)
	defer idle.Stop()

	reader := bufio.NewReader(resp.Body)
	var event, data string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if connCtx.Err() != nil && ctx.Err() == nil {
				return errors.New("no data or heartbeat within the idle timeout")
			}
			return err
		}
		idle.Reset(c.cfg.StreamIdleTimeout)

		line = strings.TrimRight(line, "\r\n")
		switch {
		case line == "":
			if event == "snapshot" && data != "" {
				c.applySnapshot(data)
			}
			event, data = "", ""
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data += strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
		}
	}
}

func (c *Client) applySnapshot(data string) {
	snap, err := parseSnapshot([]byte(data))
	if err != nil {
		c.cfg.Logger.Error("ignoring a broken snapshot from the stream", "error", err)
		return
	}
	c.snapshot.Store(snap)
}
