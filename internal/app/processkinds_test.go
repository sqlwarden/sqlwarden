package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/sqlwarden/internal/config"
)

func TestSelectKinds(t *testing.T) {
	tests := []struct {
		selected []string
		wantAPI  bool
		wantJobs bool
	}{
		{[]string{config.ProcessKindAll}, true, true},
		{[]string{config.ProcessKindAPI}, true, false},
		{[]string{config.ProcessKindJobs}, false, true},
		{[]string{config.ProcessKindAPI, config.ProcessKindJobs}, true, true},
	}
	for _, tt := range tests {
		cfg := config.Default()
		cfg.ProcessKinds = tt.selected
		api, jobs := selectKinds(cfg)
		if api != tt.wantAPI || jobs != tt.wantJobs {
			t.Errorf("selectKinds(%v) = api %v jobs %v", tt.selected, api, jobs)
		}
	}
}

func TestJobsOnlyHandlerServesHealthOnly(t *testing.T) {
	health := NewHealth()
	handler := httpHandler(nil, health)
	for path, want := range map[string]int{
		"/livez":            http.StatusOK,
		"/api/setup/status": http.StatusNotFound,
	} {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != want {
			t.Errorf("GET %s = %d, want %d", path, rec.Code, want)
		}
	}
}

func TestRuntimeKindPassesJobsSelection(t *testing.T) {
	for _, runJobs := range []bool{true, false} {
		rt := &fakeRuntime{}
		kind := newRuntimeKind(rt, runJobs)
		if err := kind.Start(t.Context()); err != nil {
			t.Fatal(err)
		}
		if err := kind.Close(t.Context()); err != nil {
			t.Fatal(err)
		}
		want := []string{fmt.Sprintf("start jobs=%v", runJobs), "stop"}
		if !slices.Equal(rt.calls, want) {
			t.Fatalf("calls = %v, want %v", rt.calls, want)
		}
	}
}

type fakeRuntime struct{ calls []string }

func (f *fakeRuntime) StartRuntime(runJobs bool) {
	f.calls = append(f.calls, fmt.Sprintf("start jobs=%v", runJobs))
}
func (f *fakeRuntime) StopRuntime() { f.calls = append(f.calls, "stop") }
