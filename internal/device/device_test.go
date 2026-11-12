package device

import "testing"

func TestStateIsValid(t *testing.T) {
	tests := []struct {
		state State
		want  bool
	}{
		{StateAvailable, true},
		{StateInUse, true},
		{StateInactive, true},
		{"", false},
		{"in_use", false},
		{"AVAILABLE", false},
	}

	for _, tc := range tests {
		t.Run(string(tc.state), func(t *testing.T) {
			if got := tc.state.IsValid(); got != tc.want {
				t.Fatalf("State(%q).IsValid() = %v, want %v", tc.state, got, tc.want)
			}
		})
	}
}
