package observability

import (
	"context"
	"strings"
	"testing"
)

func TestNormalizeRequestID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "accepts safe token", input: "client-request_1.a:b/c=", want: "client-request_1.a:b/c="},
		{name: "trims surrounding whitespace", input: "  req-1\t", want: "req-1"},
		{name: "rejects empty", input: "   ", want: ""},
		{name: "rejects control characters", input: "bad request id\n", want: ""},
		{name: "rejects log injection", input: "req\" level=ERROR", want: ""},
		{name: "accepts maximum length", input: strings.Repeat("a", MaxRequestIDLength), want: strings.Repeat("a", MaxRequestIDLength)},
		{name: "rejects over maximum length", input: strings.Repeat("a", MaxRequestIDLength+1), want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeRequestID(tt.input); got != tt.want {
				t.Fatalf("NormalizeRequestID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRequestIDContextRoundTrip(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-1")
	if got := RequestID(ctx); got != "req-1" {
		t.Fatalf("RequestID() = %q, want req-1", got)
	}
	if got := RequestID(WithRequestID(context.Background(), "")); got != "" {
		t.Fatalf("RequestID() with empty ID = %q, want empty", got)
	}
}
