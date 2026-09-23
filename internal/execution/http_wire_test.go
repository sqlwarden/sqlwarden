package execution

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRPCErrorSerializesOnlySafeSentinelMessages(t *testing.T) {
	const secret = "postgres://user:secret-marker@db/app"
	sentinels := []error{
		ErrCredentialsNotFound,
		ErrCredentialDecryption,
		ErrCredentialsInvalid,
		ErrSQLiteTargetDisabled,
		ErrSQLiteInMemoryTargetDisabled,
		ErrTargetRejected,
		ErrTargetConnection,
	}
	for _, sentinel := range sentinels {
		wrapped := map[string]error{
			"wrapped": fmt.Errorf("%w: %s", sentinel, secret),
			"joined":  errors.Join(sentinel, errors.New(secret)),
			"failure": &Failure{Code: FailureSessionLost, Retryable: true, Err: fmt.Errorf("open %s: %w", secret, sentinel)},
		}
		for name, source := range wrapped {
			t.Run(sentinel.Error()+"/"+name, func(t *testing.T) {
				encoded, err := json.Marshal(newRPCError(source))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), "secret-marker") {
					t.Fatalf("rpc error leaked underlying text: %s", encoded)
				}
				var wire rpcError
				if err := json.Unmarshal(encoded, &wire); err != nil {
					t.Fatal(err)
				}
				if wire.Message != sentinel.Error() {
					t.Fatalf("wire message = %q, want %q", wire.Message, sentinel.Error())
				}
				decoded := wire.err()
				if !errors.Is(decoded, sentinel) {
					t.Fatalf("decoded error = %v, want %v", decoded, sentinel)
				}
				if strings.Contains(decoded.Error(), "secret-marker") {
					t.Fatalf("decoded error leaked underlying text: %q", decoded.Error())
				}
			})
		}
	}
}

func TestRPCErrorDecodeIgnoresUnsafeMessageForRedactedKinds(t *testing.T) {
	wire := &rpcError{Kind: rpcKindCredentialDecryption, Message: "legacy secret-marker"}
	decoded := wire.err()
	if !errors.Is(decoded, ErrCredentialDecryption) {
		t.Fatalf("decoded error = %v, want credential decryption", decoded)
	}
	if strings.Contains(decoded.Error(), "secret-marker") {
		t.Fatalf("decoded error leaked wire message: %q", decoded.Error())
	}
}

func TestDecodeHTTPErrorMapsMissingProtocolEndpointToMismatch(t *testing.T) {
	response := &http.Response{
		StatusCode: http.StatusNotFound,
		Body:       io.NopCloser(strings.NewReader("404 page not found\n")),
	}
	if err := decodeHTTPError(response); !errors.Is(err, ErrProtocolMismatch) {
		t.Fatalf("decodeHTTPError() = %v, want protocol mismatch", err)
	}
}
