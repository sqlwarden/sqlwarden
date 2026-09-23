package observability

import "strings"

// RequestIDHeader is the HTTP header that carries the request correlation ID
// between clients, the public API, and internal execution endpoints.
const RequestIDHeader = "X-Request-ID"

// MaxRequestIDLength bounds accepted request correlation IDs.
const MaxRequestIDLength = 128

// NormalizeRequestID trims requestID and returns it when it is a safe
// correlation token, or "" when it is empty, too long, or contains characters
// outside [A-Za-z0-9-_.:/=]. Callers must treat "" as "no request ID supplied".
func NormalizeRequestID(requestID string) string {
	requestID = strings.TrimSpace(requestID)
	if requestID == "" || len(requestID) > MaxRequestIDLength {
		return ""
	}
	for _, r := range requestID {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == ':', r == '/', r == '=':
		default:
			return ""
		}
	}
	return requestID
}
