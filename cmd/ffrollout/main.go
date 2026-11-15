package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "ffrollout:", err)
		os.Exit(1)
	}
}

func run() error {
	baseURL := flag.String("url", "http://localhost:8080", "FlagForge address")
	env := flag.String("env", "production", "environment")
	flagKey := flag.String("flag", "new-checkout", "flag key")
	users := flag.Int("users", 1000, "number of users to evaluate (user-0, user-1, ...)")
	out := flag.String("out", "", "write the users that got true to this file")
	mustInclude := flag.String("must-include", "", "fail if a user listed in this file did not get true")
	flag.Parse()

	adminKey := os.Getenv("FLAGFORGE_ADMIN_KEY")
	if adminKey == "" {
		return errors.New("set FLAGFORGE_ADMIN_KEY to one of the admin keys")
	}

	client := &http.Client{Timeout: 5 * time.Second}
	var trueUsers []string
	for i := range *users {
		user := fmt.Sprintf("user-%d", i)
		value, err := evaluate(client, *baseURL, adminKey, *env, *flagKey, user)
		if err != nil {
			return err
		}
		if value {
			trueUsers = append(trueUsers, user)
		}
	}
	fmt.Printf("%d of %d users got true (%.1f%%)\n", len(trueUsers), *users, 100*float64(len(trueUsers))/float64(*users))

	if *out != "" {
		if err := os.WriteFile(*out, []byte(strings.Join(trueUsers, "\n")+"\n"), 0o644); err != nil {
			return err
		}
		fmt.Printf("wrote them to %s\n", *out)
	}
	if *mustInclude != "" {
		return checkIncluded(*mustInclude, trueUsers)
	}
	return nil
}

func evaluate(client *http.Client, baseURL, adminKey, env, flagKey, user string) (bool, error) {
	body, _ := json.Marshal(map[string]any{"flag": flagKey, "user": map[string]string{"key": user}})
	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/environments/"+env+"/evaluate", bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+adminKey)
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("evaluate %s: status %d", user, resp.StatusCode)
	}
	var result struct {
		Value bool `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, err
	}
	return result.Value, nil
}

func checkIncluded(path string, trueUsers []string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	now := make(map[string]bool, len(trueUsers))
	for _, u := range trueUsers {
		now[u] = true
	}
	missing, total := 0, 0
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		u := strings.TrimSpace(scanner.Text())
		if u == "" {
			continue
		}
		total++
		if !now[u] {
			missing++
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if missing > 0 {
		return fmt.Errorf("%d of the %d users in %s no longer get true", missing, total, path)
	}
	fmt.Printf("all %d users from %s still get true\n", total, path)
	return nil
}
