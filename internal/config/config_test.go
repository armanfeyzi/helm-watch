package config

import "testing"

func TestValidateCredentialsFilePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		want    string
		wantErr bool
	}{
		{name: "absolute ok", path: "/var/run/secrets/creds.json", want: "/var/run/secrets/creds.json"},
		{name: "empty", path: "", wantErr: true},
		{name: "relative", path: "creds.json", wantErr: true},
		{name: "dot dot", path: "/var/run/../etc/passwd", wantErr: true},
		{name: "whitespace trimmed", path: "  /etc/creds.json  ", want: "/etc/creds.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := validateCredentialsFilePath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateCredentialsFilePath(%q) = %q, want error", tt.path, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateCredentialsFilePath(%q): %v", tt.path, err)
			}
			if got != tt.want {
				t.Fatalf("validateCredentialsFilePath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
