package config

import "testing"

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		port     string
		wantPort string
		wantErr  bool
	}{
		{name: "defaults when unset", port: "", wantPort: "8080"},
		{name: "uses PORT when set", port: "9090", wantPort: "9090"},
		{name: "rejects non-numeric port", port: "abc", wantErr: true},
		{name: "rejects zero port", port: "0", wantErr: true},
		{name: "rejects out of range port", port: "70000", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PORT", tc.port)

			cfg, err := Load()

			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got config %+v", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cfg.Port != tc.wantPort {
				t.Fatalf("port: got %q, want %q", cfg.Port, tc.wantPort)
			}
		})
	}
}
