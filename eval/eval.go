// Package eval decides which value a flag serves to a user.
//
// It is pure: it never touches the network, the database or the clock, so the
// server and the Go SDK can share it and always agree on the answer.
package eval

import (
	"encoding/json"
	"strings"
)

type User struct {
	Key        string            `json:"key"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type Flag struct {
	Key         string `json:"key"`
	Enabled     bool   `json:"enabled"`
	Rules       []Rule `json:"rules"`
	Fallthrough Serve  `json:"fallthrough"`
}

type Rule struct {
	Clauses []Clause `json:"clauses"`
	Serve   Serve    `json:"serve"`
}

type Clause struct {
	Attribute string   `json:"attribute"`
	Op        Op       `json:"op"`
	Values    []string `json:"values"`
}

type Op string

const (
	OpIn         Op = "in"
	OpNotIn      Op = "not_in"
	OpStartsWith Op = "starts_with"
	OpEndsWith   Op = "ends_with"
	OpContains   Op = "contains"
)

type Serve struct {
	Value *bool `json:"value,omitempty"`
}

func ServeValue(v bool) Serve {
	return Serve{Value: &v}
}

type ReasonKind string

const (
	ReasonOff         ReasonKind = "OFF"
	ReasonRuleMatch   ReasonKind = "RULE_MATCH"
	ReasonFallthrough ReasonKind = "FALLTHROUGH"
)

type Reason struct {
	Kind      ReasonKind
	RuleIndex int
}

func (r Reason) MarshalJSON() ([]byte, error) {
	if r.Kind == ReasonRuleMatch {
		return json.Marshal(struct {
			Kind      ReasonKind `json:"kind"`
			RuleIndex int        `json:"rule_index"`
		}{r.Kind, r.RuleIndex})
	}
	return json.Marshal(struct {
		Kind ReasonKind `json:"kind"`
	}{r.Kind})
}

type Result struct {
	Value  bool   `json:"value"`
	Reason Reason `json:"reason"`
}

func Evaluate(flag Flag, user User) Result {
	if !flag.Enabled {
		return Result{Value: false, Reason: Reason{Kind: ReasonOff}}
	}
	for i, rule := range flag.Rules {
		if rule.matches(user) {
			return Result{Value: rule.Serve.value(), Reason: Reason{Kind: ReasonRuleMatch, RuleIndex: i}}
		}
	}
	return Result{Value: flag.Fallthrough.value(), Reason: Reason{Kind: ReasonFallthrough}}
}

func (r Rule) matches(user User) bool {
	for _, c := range r.Clauses {
		if !c.matches(user) {
			return false
		}
	}
	return true
}

func (c Clause) matches(user User) bool {
	actual, ok := user.attribute(c.Attribute)
	if !ok {
		return false
	}
	switch c.Op {
	case OpIn:
		return anyValue(c.Values, func(v string) bool { return actual == v })
	case OpNotIn:
		return !anyValue(c.Values, func(v string) bool { return actual == v })
	case OpStartsWith:
		return anyValue(c.Values, func(v string) bool { return strings.HasPrefix(actual, v) })
	case OpEndsWith:
		return anyValue(c.Values, func(v string) bool { return strings.HasSuffix(actual, v) })
	case OpContains:
		return anyValue(c.Values, func(v string) bool { return strings.Contains(actual, v) })
	}
	return false
}

func (u User) attribute(name string) (string, bool) {
	if name == "key" {
		return u.Key, true
	}
	v, ok := u.Attributes[name]
	return v, ok
}

func anyValue(values []string, match func(string) bool) bool {
	for _, v := range values {
		if match(v) {
			return true
		}
	}
	return false
}

func (s Serve) value() bool {
	return s.Value != nil && *s.Value
}
