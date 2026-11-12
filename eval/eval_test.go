package eval

import "testing"

func TestEvaluate(t *testing.T) {
	user := User{Key: "u-1"}
	tests := map[string]struct {
		flag Flag
		want Result
	}{
		"off flag serves false": {
			flag: Flag{Key: "new-checkout", Enabled: false},
			want: Result{Value: false, Reason: Reason{Kind: ReasonOff}},
		},
		"on flag without rules serves true": {
			flag: Flag{Key: "new-checkout", Enabled: true},
			want: Result{Value: true, Reason: Reason{Kind: ReasonFallthrough}},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := Evaluate(tc.flag, user); got != tc.want {
				t.Fatalf("Evaluate = %+v, want %+v", got, tc.want)
			}
		})
	}
}
