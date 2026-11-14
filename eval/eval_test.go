package eval

import (
	"encoding/json"
	"testing"
)

func countryIn(values ...string) Clause {
	return Clause{Attribute: "country", Op: OpIn, Values: values}
}

func TestEvaluate(t *testing.T) {
	tr := User{Key: "u-1", Attributes: map[string]string{"country": "TR", "email": "ali@firma.com"}}
	de := User{Key: "u-2", Attributes: map[string]string{"country": "DE", "email": "eva@example.de"}}

	onlyTurkey := Flag{
		Key:         "new-checkout",
		Enabled:     true,
		Rules:       []Rule{{Clauses: []Clause{countryIn("TR")}, Serve: ServeValue(true)}},
		Fallthrough: ServeValue(false),
	}

	tests := map[string]struct {
		flag Flag
		user User
		want Result
	}{
		"off flag serves false even when a rule would match": {
			flag: Flag{Rules: onlyTurkey.Rules, Fallthrough: ServeValue(true)},
			user: tr,
			want: Result{Value: false, Reason: Reason{Kind: ReasonOff}},
		},
		"on flag without rules serves the fallthrough": {
			flag: Flag{Enabled: true, Fallthrough: ServeValue(true)},
			user: tr,
			want: Result{Value: true, Reason: Reason{Kind: ReasonFallthrough}},
		},
		"matching rule wins": {
			flag: onlyTurkey,
			user: tr,
			want: Result{Value: true, Reason: Reason{Kind: ReasonRuleMatch, RuleIndex: 0}},
		},
		"no rule matches so the fallthrough is used": {
			flag: onlyTurkey,
			user: de,
			want: Result{Value: false, Reason: Reason{Kind: ReasonFallthrough}},
		},
		"first matching rule wins": {
			flag: Flag{Enabled: true, Fallthrough: ServeValue(true), Rules: []Rule{
				{Clauses: []Clause{{Attribute: "email", Op: OpEndsWith, Values: []string{"@example.de"}}}, Serve: ServeValue(false)},
				{Clauses: []Clause{countryIn("DE")}, Serve: ServeValue(true)},
			}},
			user: de,
			want: Result{Value: false, Reason: Reason{Kind: ReasonRuleMatch, RuleIndex: 0}},
		},
		"all clauses of a rule must match": {
			flag: Flag{Enabled: true, Fallthrough: ServeValue(false), Rules: []Rule{
				{Clauses: []Clause{countryIn("TR"), {Attribute: "email", Op: OpEndsWith, Values: []string{"@other.com"}}}, Serve: ServeValue(true)},
			}},
			user: tr,
			want: Result{Value: false, Reason: Reason{Kind: ReasonFallthrough}},
		},
		"missing attribute never matches, even not_in": {
			flag: Flag{Enabled: true, Fallthrough: ServeValue(false), Rules: []Rule{
				{Clauses: []Clause{{Attribute: "plan", Op: OpNotIn, Values: []string{"free"}}}, Serve: ServeValue(true)},
			}},
			user: tr,
			want: Result{Value: false, Reason: Reason{Kind: ReasonFallthrough}},
		},
		"key attribute is the user key": {
			flag: Flag{Enabled: true, Fallthrough: ServeValue(false), Rules: []Rule{
				{Clauses: []Clause{{Attribute: "key", Op: OpIn, Values: []string{"u-2"}}}, Serve: ServeValue(true)},
			}},
			user: de,
			want: Result{Value: true, Reason: Reason{Kind: ReasonRuleMatch}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Evaluate(tc.flag, tc.user); got != tc.want {
				t.Fatalf("Evaluate = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestOperators(t *testing.T) {
	user := User{Key: "u-1", Attributes: map[string]string{"email": "ali@firma.com"}}
	tests := []struct {
		op     Op
		values []string
		want   bool
	}{
		{OpIn, []string{"x@y.com", "ali@firma.com"}, true},
		{OpIn, []string{"ALI@firma.com"}, false},
		{OpNotIn, []string{"x@y.com"}, true},
		{OpNotIn, []string{"ali@firma.com"}, false},
		{OpStartsWith, []string{"veli", "ali@"}, true},
		{OpStartsWith, []string{"firma"}, false},
		{OpEndsWith, []string{"@firma.com"}, true},
		{OpEndsWith, []string{"@firma"}, false},
		{OpContains, []string{"firma"}, true},
		{OpContains, []string{"veli"}, false},
	}
	for _, tc := range tests {
		c := Clause{Attribute: "email", Op: tc.op, Values: tc.values}
		if got := c.matches(user); got != tc.want {
			t.Errorf("%s %v on %q = %v, want %v", tc.op, tc.values, user.Attributes["email"], got, tc.want)
		}
	}
}

func TestReasonJSON(t *testing.T) {
	tests := map[Reason]string{
		{Kind: ReasonOff}:                     `{"kind":"OFF"}`,
		{Kind: ReasonFallthrough}:             `{"kind":"FALLTHROUGH"}`,
		{Kind: ReasonRuleMatch, RuleIndex: 0}: `{"kind":"RULE_MATCH","rule_index":0}`,
		{Kind: ReasonRuleMatch, RuleIndex: 3}: `{"kind":"RULE_MATCH","rule_index":3}`,
	}
	for reason, want := range tests {
		got, err := json.Marshal(reason)
		if err != nil || string(got) != want {
			t.Errorf("Marshal(%+v) = %s, %v, want %s", reason, got, err, want)
		}
	}
}
