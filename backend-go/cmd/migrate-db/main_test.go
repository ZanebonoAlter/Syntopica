package main

import (
	"testing"

	"syntopica-backend/internal/platform/database/datamigrate"
)

func TestModeNeedsTargetPreparation(t *testing.T) {
	tests := []struct {
		name string
		mode datamigrate.Mode
		want bool
	}{
		{name: "dry run does not prepare target", mode: datamigrate.ModeDryRun, want: false},
		{name: "verify only does not prepare target", mode: datamigrate.ModeVerifyOnly, want: false},
		{name: "execute prepares target", mode: datamigrate.ModeExecute, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := modeNeedsTargetPreparation(tc.mode)
			if got != tc.want {
				t.Fatalf("modeNeedsTargetPreparation(%q) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

func TestModePreparesTargetBeforeResolvingSpecs(t *testing.T) {
	tests := []struct {
		name string
		mode datamigrate.Mode
		want bool
	}{
		{name: "dry run resolves without preparation", mode: datamigrate.ModeDryRun, want: false},
		{name: "verify only resolves without preparation", mode: datamigrate.ModeVerifyOnly, want: false},
		{name: "execute prepares before resolving specs", mode: datamigrate.ModeExecute, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := modePreparesTargetBeforeResolvingSpecs(tc.mode)
			if got != tc.want {
				t.Fatalf("modePreparesTargetBeforeResolvingSpecs(%q) = %v, want %v", tc.mode, got, tc.want)
			}
		})
	}
}

func TestValidateExecuteSafety(t *testing.T) {
	tests := []struct {
		name    string
		options cliOptions
		wantErr bool
	}{
		{name: "dry run does not need force", options: cliOptions{Mode: datamigrate.ModeDryRun}, wantErr: false},
		{name: "verify only does not need force", options: cliOptions{Mode: datamigrate.ModeVerifyOnly}, wantErr: false},
		{name: "execute requires force", options: cliOptions{Mode: datamigrate.ModeExecute, Force: false}, wantErr: true},
		{name: "execute with force passes", options: cliOptions{Mode: datamigrate.ModeExecute, Force: true}, wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateExecuteSafety(&tc.options)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validateExecuteSafety() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
