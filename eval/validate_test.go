package eval

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestValidateRulesAcceptsGoodRules(t *testing.T) {
	rules := []Rule{
		{Clauses: []Clause{countryIn("TR", "AZ")}, Serve: ServeValue(true)},
		{Clauses: []Clause{{Attribute: "email", Op: OpEndsWith, Values: []string{"@firma.com"}}}, Serve: ServeValue(false)},
	}
	if err := ValidateRules(rules); err != nil {
		t.Fatalf("ValidateRules: %v", err)
	}
	if err := ValidateRules(nil); err != nil {
		t.Fatalf("no rules must be valid: %v", err)
	}
}

func TestValidateRulesRejectsBadRules(t *testing.T) {
	good := Clause{Attribute: "country", Op: OpIn, Values: []string{"TR"}}
	tests := map[string]struct {
		rules []Rule
		want  string
	}{
		"unknown operator":   {[]Rule{{Clauses: []Clause{{Attribute: "country", Op: "equals", Values: []string{"TR"}}}, Serve: ServeValue(true)}}, `rules[0].clauses[0].op: unknown operator "equals"`},
		"empty attribute":    {[]Rule{{Clauses: []Clause{{Op: OpIn, Values: []string{"TR"}}}, Serve: ServeValue(true)}}, "rules[0].clauses[0].attribute"},
		"no values":          {[]Rule{{Clauses: []Clause{{Attribute: "country", Op: OpIn}}, Serve: ServeValue(true)}}, "rules[0].clauses[0].values"},
		"empty value":        {[]Rule{{Clauses: []Clause{{Attribute: "country", Op: OpIn, Values: []string{""}}}, Serve: ServeValue(true)}}, "rules[0].clauses[0].values[0]"},
		"no clauses":         {[]Rule{{Serve: ServeValue(true)}}, "rules[0].clauses"},
		"missing serve":      {[]Rule{{Clauses: []Clause{good}}}, "rules[0].serve: value is required"},
		"second rule is bad": {[]Rule{{Clauses: []Clause{good}, Serve: ServeValue(true)}, {Clauses: []Clause{good}}}, "rules[1].serve"},
		"too many rules":     {make([]Rule, MaxRules+1), "at most 50 rules"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := ValidateRules(tc.rules)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateRules = %v, want an error containing %q", err, tc.want)
			}
		})
	}
}

func FuzzValidateRules(f *testing.F) {
	f.Add(`[{"clauses":[{"attribute":"country","op":"in","values":["TR"]}],"serve":{"value":true}}]`)
	f.Add(`[{"clauses":[{"attribute":"email","op":"ends_with","values":["@firma.com"]}],"serve":{"value":false}}]`)
	f.Add(`[{"clauses":[],"serve":{}}]`)
	f.Add(`[{"clauses":[{"attribute":"","op":"??","values":[""]}]}]`)
	f.Add(`[]`)
	f.Add(`null`)

	user := User{Key: "u-1", Attributes: map[string]string{"country": "TR", "email": "ali@firma.com"}}

	f.Fuzz(func(t *testing.T, input string) {
		var rules []Rule
		if err := json.Unmarshal([]byte(input), &rules); err != nil {
			return
		}
		if err := ValidateRules(rules); err != nil {
			return
		}

		for _, rule := range rules {
			if rule.Serve.Value == nil {
				t.Fatalf("validator accepted a rule without a value: %s", input)
			}
		}

		result := Evaluate(Flag{Enabled: true, Rules: rules, Fallthrough: ServeValue(false)}, user)
		if result.Reason.Kind == ReasonRuleMatch && (result.Reason.RuleIndex < 0 || result.Reason.RuleIndex >= len(rules)) {
			t.Fatalf("rule index %d out of range for %d rules", result.Reason.RuleIndex, len(rules))
		}

		encoded, err := json.Marshal(rules)
		if err != nil {
			t.Fatalf("valid rules must encode: %v", err)
		}
		var again []Rule
		if err := json.Unmarshal(encoded, &again); err != nil {
			t.Fatalf("encoded rules must decode: %v", err)
		}
		if err := ValidateRules(again); err != nil {
			t.Fatalf("rules stopped being valid after a round trip: %v", err)
		}
		if !reflect.DeepEqual(Evaluate(Flag{Enabled: true, Rules: again, Fallthrough: ServeValue(false)}, user), result) {
			t.Fatalf("round trip changed the evaluation result for %s", input)
		}
	})
}
