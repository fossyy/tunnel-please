package version

import (
	"fmt"
	"testing"
)

func TestVersionFunctions(t *testing.T) {
	origVersion := Version
	origBuildDate := BuildDate
	origCommit := Commit
	defer func() {
		Version = origVersion
		BuildDate = origBuildDate
		Commit = origCommit
	}()

	tests := []struct {
		name      string
		version   string
		buildDate string
		commit    string
		wantFull  string
		wantShort string
	}{
		{
			name:      "Default dev version",
			version:   "dev",
			buildDate: "unknown",
			commit:    "unknown",
			wantFull:  "tunnel_pls dev (commit: unknown, built: unknown)",
			wantShort: "dev",
		},
		{
			name:      "Release version",
			version:   "v1.0.0",
			buildDate: "2026-01-23",
			commit:    "abcdef123",
			wantFull:  "tunnel_pls v1.0.0 (commit: abcdef123, built: 2026-01-23)",
			wantShort: "v1.0.0",
		},
		{
			name:      "Empty values",
			version:   "",
			buildDate: "",
			commit:    "",
			wantFull:  "tunnel_pls  (commit: , built: )",
			wantShort: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Version = tt.version
			BuildDate = tt.buildDate
			Commit = tt.commit

			gotFull := GetVersion()
			if gotFull != tt.wantFull {
				t.Errorf("GetVersion() = %q, want %q", gotFull, tt.wantFull)
			}

			gotShort := GetShortVersion()
			if gotShort != tt.wantShort {
				t.Errorf("GetShortVersion() = %q, want %q", gotShort, tt.wantShort)
			}
		})
	}
}

func TestGetVersion_Format(t *testing.T) {
	v := "1.2.3"
	c := "brainrot"
	d := "now"

	Version = v
	Commit = c
	BuildDate = d

	expected := fmt.Sprintf("tunnel_pls %s (commit: %s, built: %s)", v, c, d)
	if GetVersion() != expected {
		t.Errorf("GetVersion() formatting mismatch")
	}
}
