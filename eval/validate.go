package eval

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	MaxRules          = 50
	MaxClausesPerRule = 20
	MaxValuesPerRule  = 100
	maxAttributeLen   = 64
	maxValueLen       = 256
)

func ValidateRules(rules []Rule) error {
	if len(rules) > MaxRules {
		return fmt.Errorf("rules: at most %d rules are allowed, got %d", MaxRules, len(rules))
	}
	for i, rule := range rules {
		if err := rule.validate(); err != nil {
			return fmt.Errorf("rules[%d].%w", i, err)
		}
	}
	return nil
}

func (r Rule) validate() error {
	if len(r.Clauses) == 0 || len(r.Clauses) > MaxClausesPerRule {
		return fmt.Errorf("clauses: a rule needs 1 to %d clauses, got %d", MaxClausesPerRule, len(r.Clauses))
	}
	for i, c := range r.Clauses {
		if err := c.validate(); err != nil {
			return fmt.Errorf("clauses[%d].%w", i, err)
		}
	}
	if err := r.Serve.Validate(); err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

func (c Clause) validate() error {
	if c.Attribute == "" || utf8.RuneCountInString(c.Attribute) > maxAttributeLen {
		return fmt.Errorf("attribute: must be 1 to %d characters", maxAttributeLen)
	}
	switch c.Op {
	case OpIn, OpNotIn, OpStartsWith, OpEndsWith, OpContains:
	default:
		return fmt.Errorf("op: unknown operator %q (want in, not_in, starts_with, ends_with or contains)", c.Op)
	}
	if len(c.Values) == 0 || len(c.Values) > MaxValuesPerRule {
		return fmt.Errorf("values: a clause needs 1 to %d values, got %d", MaxValuesPerRule, len(c.Values))
	}
	for i, v := range c.Values {
		if v == "" || utf8.RuneCountInString(v) > maxValueLen {
			return fmt.Errorf("values[%d]: must be 1 to %d characters", i, maxValueLen)
		}
	}
	return nil
}

func (s Serve) Validate() error {
	if s.Value == nil {
		return errors.New("value is required")
	}
	return nil
}
