package main

import (
	"slices"
	"testing"

	"github.com/sqlwarden/internal/app"
)

func TestParseCommand(t *testing.T) {
	tests := []struct {
		args     []string
		wantCmd  app.Command
		wantRest []string
	}{
		{nil, app.CommandServe, nil},
		{[]string{"--http-port", "7000"}, app.CommandServe, []string{"--http-port", "7000"}},
		{[]string{"serve", "--http-port", "7000"}, app.CommandServe, []string{"--http-port", "7000"}},
		{[]string{"migrate"}, app.CommandMigrate, []string{}},
		{[]string{"rotate-keys", "--config", "x.yaml"}, app.CommandRotateKeys, []string{"--config", "x.yaml"}},
	}
	for _, tt := range tests {
		cmd, rest := parseCommand(tt.args)
		if cmd != tt.wantCmd || !slices.Equal(rest, tt.wantRest) {
			t.Errorf("parseCommand(%v) = %v %v", tt.args, cmd, rest)
		}
	}
}
