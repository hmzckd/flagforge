package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/hmzckd/flagforge/sdk"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ffbench:", err)
		os.Exit(1)
	}
}

type admin struct {
	baseURL string
	key     string
	http    *http.Client
}

func run() error {
	writeURL := flag.String("write-url", "http://localhost:8081", "instance that receives the changes (A)")
	readURL := flag.String("read-url", "http://localhost:8082", "instance the SDK connects to (B)")
	env := flag.String("env", "production", "environment")
	flagKey := flag.String("flag", "ffbench", "flag to toggle, created if missing")
	changes := flag.Int("changes", 100, "number of changes to measure")
	timeout := flag.Duration("timeout", 5*time.Second, "give up on a change after this long")
	flag.Parse()

	adminKey := os.Getenv("FLAGFORGE_ADMIN_KEY")
	if adminKey == "" {
		return errors.New("set FLAGFORGE_ADMIN_KEY to one of the admin keys")
	}
	a := admin{baseURL: *writeURL, key: adminKey, http: &http.Client{Timeout: 5 * time.Second}}

	if err := a.createFlag(*flagKey); err != nil {
		return err
	}
	sdkKey, keyID, err := a.createSDKKey(*env)
	if err != nil {
		return err
	}
	defer a.revokeSDKKey(*env, keyID)

	client, err := sdk.New(context.Background(), sdk.Config{BaseURL: *readURL, SDKKey: sdkKey})
	if err != nil {
		return err
	}
	defer client.Close()

	settingPath := fmt.Sprintf("/api/flags/%s/environments/%s", *flagKey, *env)
	enabled, etag, err := a.getSetting(settingPath)
	if err != nil {
		return err
	}
	user := sdk.User{Key: "ffbench"}

	durations := make([]time.Duration, 0, *changes)
	for range *changes {
		enabled = !enabled
		start := time.Now()
		etag, err = a.putEnabled(settingPath, etag, enabled)
		if err != nil {
			return err
		}
		for client.Bool(*flagKey, user, !enabled) != enabled {
			if time.Since(start) > *timeout {
				return fmt.Errorf("change %d did not reach the SDK within %v", len(durations)+1, *timeout)
			}
			time.Sleep(100 * time.Microsecond)
		}
		durations = append(durations, time.Since(start))
	}

	fmt.Printf("%d changes made on %s, seen by an SDK connected to %s\n", len(durations), *writeURL, *readURL)
	fmt.Printf("p50 %s  p95 %s  max %s\n",
		ms(percentile(durations, 50)), ms(percentile(durations, 95)), ms(percentile(durations, 100)))
	return nil
}

func percentile(durations []time.Duration, p float64) time.Duration {
	if len(durations) == 0 {
		return 0
	}
	sorted := slices.Clone(durations)
	slices.Sort(sorted)
	rank := int(math.Ceil(p / 100 * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}

func ms(d time.Duration) string {
	return fmt.Sprintf("%.1f ms", float64(d)/float64(time.Millisecond))
}

func (a admin) do(method, path, ifMatch string, body any) (*http.Response, []byte, error) {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, nil, err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, a.baseURL+path, reader)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.key)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return resp, data, err
}

func (a admin) createFlag(key string) error {
	resp, body, err := a.do(http.MethodPost, "/api/flags", "", map[string]string{"key": key, "name": "Benchmark flag"})
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		return fmt.Errorf("create flag: status %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (a admin) createSDKKey(env string) (key string, id int64, err error) {
	resp, body, err := a.do(http.MethodPost, "/api/environments/"+env+"/sdk-keys", "", nil)
	if err != nil {
		return "", 0, err
	}
	if resp.StatusCode != http.StatusCreated {
		return "", 0, fmt.Errorf("create SDK key: status %d: %s", resp.StatusCode, body)
	}
	var created struct {
		ID  int64  `json:"id"`
		Key string `json:"key"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		return "", 0, err
	}
	return created.Key, created.ID, nil
}

func (a admin) revokeSDKKey(env string, id int64) {
	_, _, _ = a.do(http.MethodDelete, fmt.Sprintf("/api/environments/%s/sdk-keys/%d", env, id), "", nil)
}

func (a admin) getSetting(path string) (enabled bool, etag string, err error) {
	resp, body, err := a.do(http.MethodGet, path, "", nil)
	if err != nil {
		return false, "", err
	}
	if resp.StatusCode != http.StatusOK {
		return false, "", fmt.Errorf("get setting: status %d: %s", resp.StatusCode, body)
	}
	var setting struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(body, &setting); err != nil {
		return false, "", err
	}
	return setting.Enabled, resp.Header.Get("ETag"), nil
}

func (a admin) putEnabled(path, etag string, enabled bool) (string, error) {
	resp, body, err := a.do(http.MethodPut, path, etag, map[string]bool{"enabled": enabled})
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("change setting: status %d: %s", resp.StatusCode, body)
	}
	return resp.Header.Get("ETag"), nil
}
