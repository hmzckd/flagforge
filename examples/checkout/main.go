package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/hmzckd/flagforge/sdk"
)

var users = []sdk.User{
	{Key: "u-1", Attributes: map[string]string{"country": "TR"}},
	{Key: "u-2", Attributes: map[string]string{"country": "TR"}},
	{Key: "u-3", Attributes: map[string]string{"country": "DE"}},
	{Key: "u-4", Attributes: map[string]string{"country": "DE"}},
	{Key: "u-5", Attributes: map[string]string{"country": "US"}},
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	client, err := sdk.New(ctx, sdk.Config{
		BaseURL: getenv("FLAGFORGE_URL", "http://localhost:8080"),
		SDKKey:  os.Getenv("FLAGFORGE_SDK_KEY"),
		Logger:  logger,
	})
	if err != nil {
		logger.Warn("starting with default values", "error", err)
	}
	defer client.Close()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		line := make([]string, 0, len(users))
		for _, u := range users {
			line = append(line, fmt.Sprintf("%s:%v", u.Key, client.Bool("new-checkout", u, false)))
		}
		fmt.Printf("%s  version=%d  %s\n", time.Now().Format("15:04:05"), client.Version(), strings.Join(line, " "))

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
