package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/app"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		args         []string
		wantCmd      app.Command
		wantRest     []string
		wantExplicit bool
	}{
		{nil, app.CommandServe, nil, false},
		{[]string{"--http-port", "7000"}, app.CommandServe, []string{"--http-port", "7000"}, false},
		{[]string{"serve", "--http-port", "7000"}, app.CommandServe, []string{"--http-port", "7000"}, true},
		{[]string{"migrate"}, app.CommandMigrate, []string{}, true},
		{[]string{"rotate-keys", "--config", "x.yaml"}, app.CommandRotateKeys, []string{"--config", "x.yaml"}, true},
	}
	for _, tt := range tests {
		cmd, rest, explicit, err := parseCommand(tt.args)
		if err != nil || cmd != tt.wantCmd || !slices.Equal(rest, tt.wantRest) || explicit != tt.wantExplicit {
			t.Errorf("parseCommand(%v) = %v %v %v %v", tt.args, cmd, rest, explicit, err)
		}
	}
}

func TestParseCommandRejectsUnknownSubcommand(t *testing.T) {
	_, _, _, err := parseCommand([]string{"migrat"})
	if err == nil {
		t.Fatal("parseCommand accepted an unknown subcommand")
	}
	for _, want := range []string{`"migrat"`, "serve", "migrate", "rotate-keys"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %s", err, want)
		}
	}
}

func TestResolveCommand(t *testing.T) {
	tests := []struct {
		name       string
		command    app.Command
		explicit   bool
		positional []string
		want       app.Command
		wantErr    string
	}{
		{"no positional keeps default", app.CommandServe, false, nil, app.CommandServe, ""},
		{"trailing migrate", app.CommandServe, false, []string{"migrate"}, app.CommandMigrate, ""},
		{"trailing rotate-keys", app.CommandServe, false, []string{"rotate-keys"}, app.CommandRotateKeys, ""},
		{"trailing unknown", app.CommandServe, false, []string{"migrat"}, app.CommandServe, `unknown subcommand "migrat"`},
		{"trailing extra", app.CommandServe, false, []string{"migrate", "now"}, app.CommandServe, `unexpected argument "now"`},
		{"leading plus trailing", app.CommandMigrate, true, []string{"serve"}, app.CommandMigrate, `unexpected argument "serve"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveCommand(tt.command, tt.explicit, tt.positional)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("resolveCommand = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
}
