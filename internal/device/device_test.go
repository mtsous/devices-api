package device

import (
	"testing"

	"github.com/google/uuid"
)

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

func TestDeviceString(t *testing.T) {
	id := uuid.MustParse("01890000-0000-7000-8000-000000000000")
	d := Device{ID: id, Name: "iPhone", Brand: "Apple", State: StateAvailable}

	got := d.String()
	want := `{ID:01890000-0000-7000-8000-000000000000 Name:"iPhone" Brand:"Apple" State:"available" CreatedAt:0001-01-01 00:00:00 +0000 UTC UpdatedAt:<nil> DeletedAt:<nil>}`
	if got != want {
		t.Fatalf("String() = %s, want %s", got, want)
	}
}
