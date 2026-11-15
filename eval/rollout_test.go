package eval

import (
	"fmt"
	"testing"
)

const population = 100_000

func userKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("user-%d", i)
	}
	return keys
}

func rolloutFlag(key string, percent int) Flag {
	return Flag{Key: key, Enabled: true, Fallthrough: ServeRollout(percent)}
}

func trueUsers(flag Flag, keys []string) map[string]bool {
	out := make(map[string]bool)
	for _, k := range keys {
		if Evaluate(flag, User{Key: k}).Value {
			out[k] = true
		}
	}
	return out
}

func TestBucketIsInRangeAndStable(t *testing.T) {
	for _, k := range userKeys(1000) {
		b := Bucket("new-checkout", k)
		if b < 0 || b >= Buckets {
			t.Fatalf("Bucket(%q) = %d, outside [0, %d)", k, b, Buckets)
		}
		if again := Bucket("new-checkout", k); again != b {
			t.Fatalf("Bucket(%q) changed from %d to %d", k, b, again)
		}
	}
}

func TestBucketKnownValues(t *testing.T) {
	tests := []struct {
		flag, user string
		want       int
	}{
		{"new-checkout", "u-1", 6473},
		{"new-checkout", "u-2", 3478},
		{"dark-mode", "u-1", 8051},
	}
	for _, tc := range tests {
		if got := Bucket(tc.flag, tc.user); got != tc.want {
			t.Fatalf("Bucket(%q, %q) = %d, want %d", tc.flag, tc.user, got, tc.want)
		}
	}
}

func TestRolloutServesAboutTheRequestedShare(t *testing.T) {
	keys := userKeys(population)
	for _, percent := range []int{0, 1, 10, 50, 100} {
		got := len(trueUsers(rolloutFlag("new-checkout", percent), keys))
		want := population * percent / 100
		if diff := got - want; diff < -500 || diff > 500 {
			t.Errorf("%d%% rollout gave %d true users out of %d, want about %d", percent, got, population, want)
		}
	}
}

func TestRaisingTheRolloutKeepsEarlierUsers(t *testing.T) {
	keys := userKeys(population)
	at10 := trueUsers(rolloutFlag("new-checkout", 10), keys)
	at20 := trueUsers(rolloutFlag("new-checkout", 20), keys)

	for k := range at10 {
		if !at20[k] {
			t.Fatalf("%s was in the 10%% rollout but not in the 20%% rollout", k)
		}
	}
	if len(at20) <= len(at10) {
		t.Fatalf("20%% rollout has %d users, 10%% has %d", len(at20), len(at10))
	}
}

func TestDifferentFlagsPickDifferentUsers(t *testing.T) {
	keys := userKeys(population)
	checkout := trueUsers(rolloutFlag("new-checkout", 10), keys)
	darkMode := trueUsers(rolloutFlag("dark-mode", 10), keys)

	both := 0
	for k := range checkout {
		if darkMode[k] {
			both++
		}
	}
	if both < 700 || both > 1300 {
		t.Fatalf("%d users are in both 10%% rollouts, want about 1000 (10%% of 10%%)", both)
	}
}

func TestRolloutInsideARule(t *testing.T) {
	flag := Flag{
		Key:     "new-checkout",
		Enabled: true,
		Rules: []Rule{{
			Clauses: []Clause{{Attribute: "country", Op: OpIn, Values: []string{"TR"}}},
			Serve:   ServeRollout(50),
		}},
		Fallthrough: ServeValue(false),
	}

	trueCount := 0
	for _, k := range userKeys(10_000) {
		res := Evaluate(flag, User{Key: k, Attributes: map[string]string{"country": "TR"}})
		if res.Reason.Kind != ReasonRuleMatch {
			t.Fatalf("reason = %+v, want RULE_MATCH", res.Reason)
		}
		if res.Value {
			trueCount++
		}
	}
	if trueCount < 4700 || trueCount > 5300 {
		t.Fatalf("%d of 10000 matching users got true, want about 5000", trueCount)
	}
}

func BenchmarkBucket(b *testing.B) {
	for b.Loop() {
		Bucket("new-checkout", "user-12345")
	}
}

func BenchmarkEvaluate(b *testing.B) {
	flag := Flag{
		Key:     "new-checkout",
		Enabled: true,
		Rules: []Rule{
			{Clauses: []Clause{{Attribute: "email", Op: OpEndsWith, Values: []string{"@firma.com"}}}, Serve: ServeValue(true)},
			{Clauses: []Clause{{Attribute: "country", Op: OpIn, Values: []string{"TR", "AZ"}}}, Serve: ServeRollout(25)},
		},
		Fallthrough: ServeValue(false),
	}
	user := User{Key: "user-12345", Attributes: map[string]string{"country": "TR", "email": "ali@example.com"}}

	for b.Loop() {
		Evaluate(flag, user)
	}
}
