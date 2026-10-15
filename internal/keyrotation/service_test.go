package keyrotation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/credentials"
)

type fakeStages struct {
	calls []string
	conn  credentials.RotationReport
	files [2]int
	smtp  [2]bool
	err   map[string]error
}

func (f *fakeStages) Run(context.Context) (credentials.RotationReport, error) {
	f.calls = append(f.calls, "connections")
	return f.conn, f.err["connections"]
}

func (f *fakeStages) RotateContents(context.Context) (int, int, error) {
	f.calls = append(f.calls, "files")
	return f.files[0], f.files[1], f.err["files"]
}

func (f *fakeStages) RotateSMTPPassword(context.Context) (bool, bool, error) {
	f.calls = append(f.calls, "smtp")
	return f.smtp[0], f.smtp[1], f.err["smtp"]
}

func newService(f *fakeStages) *Service {
	return New(Deps{Connections: f, SMTP: f, Files: f})
}

func TestRotateDeniesNonAdminBeforeAnyStage(t *testing.T) {
	f := &fakeStages{}
	principals := map[string]access.Principal{
		"member":             {Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: 7}, Credential: access.CredentialInfo{Kind: access.CredentialSession}},
		"zero":               {},
		"job":                {Credential: access.CredentialInfo{Kind: access.CredentialJob}},
		"local-actor":        {Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: 7}, Credential: access.CredentialInfo{Kind: access.CredentialLocal}},
		"admin-flagged-job":  {InstanceAdmin: true, Credential: access.CredentialInfo{Kind: access.CredentialJob}},
		"admin-flagged-zero": {InstanceAdmin: true},
	}
	for name, p := range principals {
		t.Run(name, func(t *testing.T) {
			_, err := newService(f).Rotate(context.Background(), p, Options{})
			accesstest.AssertForbidden(t, err, "Rotate")
		})
	}
	if len(f.calls) != 0 {
		t.Fatalf("stages ran for denied callers: %v", f.calls)
	}
}

func TestRotateRunsEachStageOnceForInstanceAdmin(t *testing.T) {
	f := &fakeStages{
		conn:  credentials.RotationReport{Split: 1, Reencrypted: 2, Skipped: 3},
		files: [2]int{4, 5},
		smtp:  [2]bool{true, true},
	}
	admin := access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: 1}, InstanceAdmin: true}
	report, err := newService(f).Rotate(context.Background(), admin, Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := Report{
		ConnectionsSplit: 1, ConnectionSecretsRotated: 2, ConnectionSecretsSkipped: 3,
		FileContentsScanned: 4, FileContentsRotated: 5,
		SMTPPasswordsScanned: 1, SMTPPasswordsRotated: 1,
	}
	if report != want {
		t.Fatalf("report = %+v, want %+v", report, want)
	}
	if got := strings.Join(f.calls, ","); got != "connections,files,smtp" {
		t.Fatalf("stage calls = %s", got)
	}
}

func TestRotateAcceptsOperatorPrincipal(t *testing.T) {
	f := &fakeStages{}
	if _, err := newService(f).Rotate(context.Background(), OperatorPrincipal(), Options{}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("stage calls = %v", f.calls)
	}
}

func TestRotateStopsAtFailingStage(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		stage string
		ran   string
		want  string
	}{
		{"connections", "connections", "rotate connection credentials: boom"},
		{"files", "connections,files", "boom"},
		{"smtp", "connections,files,smtp", "boom"},
	}
	for _, tc := range cases {
		t.Run(tc.stage, func(t *testing.T) {
			f := &fakeStages{err: map[string]error{tc.stage: boom}, conn: credentials.RotationReport{Split: 2}}
			report, err := newService(f).Rotate(context.Background(), OperatorPrincipal(), Options{})
			if !errors.Is(err, boom) || err.Error() != tc.want {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
			if got := strings.Join(f.calls, ","); got != tc.ran {
				t.Fatalf("stage calls = %s, want %s", got, tc.ran)
			}
			if report.ConnectionsSplit != 2 {
				t.Fatalf("partial report lost counts: %+v", report)
			}
		})
	}
}
