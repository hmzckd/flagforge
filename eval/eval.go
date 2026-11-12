// Package eval decides which value a flag serves to a user.
//
// It is pure: it never touches the network, the database or the clock, so the
// server and the Go SDK can share it and always agree on the answer.
package eval

type User struct {
	Key        string            `json:"key"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type Flag struct {
	Key     string `json:"key"`
	Enabled bool   `json:"enabled"`
}

type ReasonKind string

const (
	ReasonOff         ReasonKind = "OFF"
	ReasonFallthrough ReasonKind = "FALLTHROUGH"
)

type Reason struct {
	Kind ReasonKind `json:"kind"`
}

type Result struct {
	Value  bool   `json:"value"`
	Reason Reason `json:"reason"`
}

func Evaluate(flag Flag, user User) Result {
	if !flag.Enabled {
		return Result{Value: false, Reason: Reason{Kind: ReasonOff}}
	}
	return Result{Value: true, Reason: Reason{Kind: ReasonFallthrough}}
}
