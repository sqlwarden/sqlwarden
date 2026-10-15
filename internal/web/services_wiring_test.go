package web

import (
	"errors"
	"net/http"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/assert"
)

func TestServiceError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{
			name:        "forbidden",
			err:         errors.Join(errors.New("authorize request"), access.ErrForbidden),
			wantStatus:  http.StatusForbidden,
			wantCode:    apiErrorNotPermitted,
			wantMessage: "You do not have permission to perform this action.",
		},
		{
			name:        "not found",
			err:         errors.Join(errors.New("load resource"), access.ErrNotFound),
			wantStatus:  http.StatusNotFound,
			wantCode:    apiErrorNotFound,
			wantMessage: "The requested resource could not be found.",
		},
		{
			name:        "unknown",
			err:         errors.New("unexpected failure"),
			wantStatus:  http.StatusInternalServerError,
			wantCode:    apiErrorInternalServer,
			wantMessage: "The server encountered a problem and could not process your request.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newTestApplication(t)
			req := newTestRequest(t, http.MethodGet, "/test", nil)

			res := send(t, req, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				app.serviceError(w, r, tt.err)
			}))

			assert.Equal(t, res.StatusCode, tt.wantStatus)
			assertAPIError(t, res, tt.wantCode, tt.wantMessage)
		})
	}
}
