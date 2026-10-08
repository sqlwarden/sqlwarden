package execution

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sqlwarden/internal/credentials"
	"github.com/sqlwarden/internal/engine"
	"github.com/sqlwarden/internal/engine/metadata"
	"github.com/sqlwarden/pkg/result"

	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
)

type fakeProvider struct {
	calls atomic.Int32
	creds credentials.Credentials
	err   error
}

func (p *fakeProvider) Resolve(context.Context, credentials.ConnectionRef) (credentials.Credentials, error) {
	p.calls.Add(1)
	if p.err != nil {
		return credentials.Credentials{}, p.err
	}
	return p.creds, nil
}

type policyFunc func(ctx context.Context, driver, dsn string) error

func (f policyFunc) Check(ctx context.Context, driver, dsn string) error { return f(ctx, driver, dsn) }

func allowAll() TargetPolicy {
	return policyFunc(func(context.Context, string, string) error { return nil })
}

func newTestRuntime(t *testing.T, policy TargetPolicy) (*LocalRuntime, *fakeProvider) {
	t.Helper()
	p := &fakeProvider{creds: credentials.Credentials{Driver: "sqlite", DSN: ":memory:"}}
	r := NewLocal(LocalConfig{Credentials: p, Policy: policy, IdleTimeout: time.Hour})
	t.Cleanup(r.Shutdown)
	return r, p
}

func scopeOf(org, ws, acct, conn string) Scope {
	return Scope{OrgID: org, WorkspaceID: ws, AccountID: acct, ConnectionID: conn}
}

var baseScope = scopeOf("o1", "w1", "a1", "c1")

func mustOpen(t *testing.T, r *LocalRuntime, s Scope) SessionInfo {
	t.Helper()
	info, err := r.Open(context.Background(), OpenRequest{Scope: s})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	return info
}

func requireNotFound(t *testing.T, err error) {
	t.Helper()
	var f *Failure
	if !errors.As(err, &f) || f.Code != FailureSessionNotFound {
		t.Fatalf("want session_not_found failure, got %v", err)
	}
	if !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("want ErrSessionNotFound, got %v", err)
	}
}

func TestLocalSessionsOpenReuses(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	first := mustOpen(t, r, baseScope)
	if first.Reused || first.ID == "" || first.Scope != baseScope || first.Driver != "sqlite" {
		t.Fatalf("unexpected first info: %+v", first)
	}
	second := mustOpen(t, r, baseScope)
	if !second.Reused || second.ID != first.ID {
		t.Fatalf("expected reuse of %s, got %+v", first.ID, second)
	}
}

func TestLocalSessionsGetScopeMismatch(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := mustOpen(t, r, baseScope)

	got, err := r.Get(context.Background(), baseScope, info.ID)
	if err != nil || got.ID != info.ID {
		t.Fatalf("Get same scope: %+v, %v", got, err)
	}

	for name, s := range map[string]Scope{
		"account":    scopeOf("o1", "w1", "a2", "c1"),
		"connection": scopeOf("o1", "w1", "a1", "c2"),
		"org":        scopeOf("o2", "w1", "a1", "c1"),
		"workspace":  scopeOf("o1", "w2", "a1", "c1"),
	} {
		_, err := r.Get(context.Background(), s, info.ID)
		if err == nil {
			t.Fatalf("%s mismatch must fail", name)
		}
		requireNotFound(t, err)
	}
	_, err = r.Get(context.Background(), baseScope, "unknown")
	requireNotFound(t, err)
}

func TestLocalSessionsCloseThenGet(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := mustOpen(t, r, baseScope)

	if err := r.Close(context.Background(), baseScope, info.ID); err != nil {
		t.Fatal(err)
	}
	_, err := r.Get(context.Background(), baseScope, info.ID)
	requireNotFound(t, err)

	if err := r.Close(context.Background(), baseScope, info.ID); err != nil {
		t.Fatalf("closing a closed session must be a no-op: %v", err)
	}
}

func TestLocalSessionsList(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	a1c1 := mustOpen(t, r, scopeOf("o1", "w1", "a1", "c1"))
	a1c2 := mustOpen(t, r, scopeOf("o1", "w1", "a1", "c2"))
	a2c1 := mustOpen(t, r, scopeOf("o1", "w1", "a2", "c1"))
	a1w2 := mustOpen(t, r, scopeOf("o1", "w2", "a1", "c3"))

	ids := func(infos []SessionInfo) map[SessionID]bool {
		m := map[SessionID]bool{}
		for _, i := range infos {
			m[i.ID] = true
		}
		return m
	}

	byAccount, err := r.List(context.Background(), SessionFilter{AccountID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	got := ids(byAccount)
	if len(got) != 3 || !got[a1c1.ID] || !got[a1c2.ID] || !got[a1w2.ID] {
		t.Fatalf("account filter: %+v", byAccount)
	}

	byBoth, err := r.List(context.Background(), SessionFilter{AccountID: "a1", WorkspaceID: "w1"})
	if err != nil {
		t.Fatal(err)
	}
	got = ids(byBoth)
	if len(got) != 2 || !got[a1c1.ID] || !got[a1c2.ID] {
		t.Fatalf("account+workspace filter: %+v", byBoth)
	}
	for _, i := range byBoth {
		if i.Scope.WorkspaceID != "w1" || i.Scope.AccountID != "a1" || i.Driver != "sqlite" {
			t.Fatalf("unexpected info: %+v", i)
		}
	}

	byWorkspace, err := r.List(context.Background(), SessionFilter{WorkspaceID: "w1"})
	if err != nil {
		t.Fatal(err)
	}
	got = ids(byWorkspace)
	if len(got) != 3 || !got[a1c1.ID] || !got[a1c2.ID] || !got[a2c1.ID] {
		t.Fatalf("workspace filter: %+v", byWorkspace)
	}

	eph, err := r.Open(context.Background(), OpenRequest{Scope: scopeOf("o1", "w1", "a1", "c1"), Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	byAccount, err = r.List(context.Background(), SessionFilter{AccountID: "a1"})
	if err != nil {
		t.Fatal(err)
	}
	if got = ids(byAccount); len(got) != 3 || got[eph.ID] {
		t.Fatalf("ephemeral session must not be listed: %+v", byAccount)
	}
	if n, _ := r.CountForConnection(context.Background(), "c1"); n != 2 {
		t.Fatalf("ephemeral session must not be counted, got %d", n)
	}
	if err := r.Close(context.Background(), scopeOf("o1", "w1", "a1", "c1"), eph.ID); err != nil {
		t.Fatalf("ephemeral session must stay closable: %v", err)
	}

	if _, err := r.List(context.Background(), SessionFilter{}); err == nil {
		t.Fatal("empty filter must fail")
	}
}

func TestLocalSessionsRevoke(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	s1 := scopeOf("o1", "w1", "a1", "c1")
	s2 := scopeOf("o1", "w1", "a2", "c1")
	s3 := scopeOf("o1", "w2", "a1", "c2")
	s4 := scopeOf("o2", "w3", "a1", "c4")
	i1, i2, i3, i4 := mustOpen(t, r, s1), mustOpen(t, r, s2), mustOpen(t, r, s3), mustOpen(t, r, s4)

	n, err := r.CountForConnection(ctx, "c1")
	if err != nil || n != 2 {
		t.Fatalf("CountForConnection = %d, %v", n, err)
	}

	n, err = r.RevokeWorkspaceAccount(ctx, "w1", "a1")
	if err != nil || n != 1 {
		t.Fatalf("RevokeWorkspaceAccount = %d, %v", n, err)
	}
	_, err = r.Get(ctx, s1, i1.ID)
	requireNotFound(t, err)
	if _, err := r.Get(ctx, s2, i2.ID); err != nil {
		t.Fatalf("other account session removed: %v", err)
	}

	n, err = r.RevokeOrgAccount(ctx, "o2", "a1")
	if err != nil || n != 1 {
		t.Fatalf("RevokeOrgAccount = %d, %v", n, err)
	}
	_, err = r.Get(ctx, s4, i4.ID)
	requireNotFound(t, err)
	if _, err := r.Get(ctx, s3, i3.ID); err != nil {
		t.Fatalf("other org session removed: %v", err)
	}

	n, err = r.RevokeConnection(ctx, "c1")
	if err != nil || n != 1 {
		t.Fatalf("RevokeConnection = %d, %v", n, err)
	}
	_, err = r.Get(ctx, s2, i2.ID)
	requireNotFound(t, err)
	if n, _ := r.CountForConnection(ctx, "c1"); n != 0 {
		t.Fatalf("count after revoke = %d", n)
	}
}

func TestLocalRevokerHonoursCancelledContext(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := mustOpen(t, r, baseScope)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := r.CountForConnection(ctx, "c1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("CountForConnection err = %v", err)
	}
	if _, err := r.RevokeConnection(ctx, "c1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RevokeConnection err = %v", err)
	}
	if _, err := r.RevokeWorkspaceAccount(ctx, "w1", "a1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RevokeWorkspaceAccount err = %v", err)
	}
	if _, err := r.RevokeOrgAccount(ctx, "o1", "a1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("RevokeOrgAccount err = %v", err)
	}
	if _, err := r.Get(context.Background(), baseScope, info.ID); err != nil {
		t.Fatalf("session must survive a cancelled revoke: %v", err)
	}
}

func TestLocalOnConnectionEmpty(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	emptied := make(chan string, 4)
	r.OnConnectionEmpty(func(id string) { emptied <- id })

	a := mustOpen(t, r, scopeOf("o1", "w1", "a1", "c1"))
	b := mustOpen(t, r, scopeOf("o1", "w1", "a2", "c1"))

	if err := r.Close(context.Background(), scopeOf("o1", "w1", "a1", "c1"), a.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-emptied:
		t.Fatalf("hook fired early for %s", id)
	default:
	}
	if err := r.Close(context.Background(), scopeOf("o1", "w1", "a2", "c1"), b.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-emptied:
		if id != "c1" {
			t.Fatalf("hook id = %s", id)
		}
	default:
		t.Fatal("hook did not fire after last session closed")
	}
}

func TestLocalOpenInvalidScopeSkipsProvider(t *testing.T) {
	r, p := newTestRuntime(t, allowAll())
	_, err := r.Open(context.Background(), OpenRequest{Scope: Scope{AccountID: "a1", ConnectionID: "c1"}})
	if err == nil {
		t.Fatal("expected error")
	}
	if p.calls.Load() != 0 {
		t.Fatal("provider must not be called for an invalid scope")
	}
}

func TestLocalOpenPolicyDenied(t *testing.T) {
	denied := errors.New("target denied")
	r, _ := newTestRuntime(t, policyFunc(func(context.Context, string, string) error { return denied }))
	_, err := r.Open(context.Background(), OpenRequest{Scope: baseScope})
	if !errors.Is(err, denied) {
		t.Fatalf("err = %v", err)
	}
	if n, _ := r.CountForConnection(context.Background(), "c1"); n != 0 {
		t.Fatalf("session created despite denial: %d", n)
	}
}

func TestLocalOpenProviderNotFound(t *testing.T) {
	r, p := newTestRuntime(t, allowAll())
	p.err = credentials.ErrNotFound
	_, err := r.Open(context.Background(), OpenRequest{Scope: baseScope})
	if err == nil || !errors.Is(err, credentials.ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestLocalOpenErrorsOutsideTheTargetAreNotConnectErrors(t *testing.T) {
	storeErr := errors.New("store unavailable")
	cases := map[string]func(*fakeProvider) TargetPolicy{
		"credentials": func(p *fakeProvider) TargetPolicy { p.err = storeErr; return allowAll() },
		"policy": func(*fakeProvider) TargetPolicy {
			return policyFunc(func(context.Context, string, string) error { return storeErr })
		},
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			p := &fakeProvider{creds: credentials.Credentials{Driver: "sqlite", DSN: ":memory:"}}
			r := NewLocal(LocalConfig{Credentials: p, Policy: setup(p), IdleTimeout: time.Hour})
			t.Cleanup(r.Shutdown)
			_, err := r.Open(context.Background(), OpenRequest{Scope: baseScope})
			var connectErr *ConnectError
			if !errors.Is(err, storeErr) || errors.As(err, &connectErr) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestLocalOpenUnknownDriverIsConnectError(t *testing.T) {
	r, p := newTestRuntime(t, allowAll())
	p.creds = credentials.Credentials{Driver: "no-such-driver", DSN: "x"}
	_, err := r.Open(context.Background(), OpenRequest{Scope: baseScope})
	var connectErr *ConnectError
	if !errors.As(err, &connectErr) {
		t.Fatalf("err = %v", err)
	}
}

func TestLocalProbe(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	err := r.Probe(context.Background(), baseScope, credentials.Credentials{Driver: "sqlite", DSN: ":memory:"}, Limits{}, func(in metadata.SchemaInspector) error {
		if in == nil {
			t.Fatal("nil inspector")
		}
		_ = in.Tree()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := r.CountForConnection(context.Background(), "c1"); n != 0 {
		t.Fatalf("probe must not register a session: %d", n)
	}

	want := errors.New("boom")
	err = r.Probe(context.Background(), baseScope, credentials.Credentials{Driver: "sqlite", DSN: ":memory:"}, Limits{}, func(metadata.SchemaInspector) error { return want })
	if !errors.Is(err, want) {
		t.Fatalf("fn error not propagated: %v", err)
	}
}

func TestLocalProbePolicyDenied(t *testing.T) {
	denied := errors.New("target denied")
	r, _ := newTestRuntime(t, policyFunc(func(context.Context, string, string) error { return denied }))
	called := false
	err := r.Probe(context.Background(), baseScope, credentials.Credentials{Driver: "sqlite", DSN: ":memory:"}, Limits{}, func(metadata.SchemaInspector) error { called = true; return nil })
	if !errors.Is(err, denied) || called {
		t.Fatalf("err = %v called = %v", err, called)
	}
}

func TestLocalEphemeralSessions(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	pooled := mustOpen(t, r, baseScope)

	e1, err := r.Open(ctx, OpenRequest{Scope: baseScope, Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	e2, err := r.Open(ctx, OpenRequest{Scope: baseScope, Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	if e1.Reused || e2.Reused || e1.ID == e2.ID || e1.ID == pooled.ID {
		t.Fatalf("ephemeral sessions must be private: %+v %+v %+v", pooled, e1, e2)
	}

	if err := r.Close(ctx, baseScope, e1.ID); err != nil {
		t.Fatal(err)
	}
	_, err = r.Get(ctx, baseScope, e1.ID)
	requireNotFound(t, err)

	if _, err := r.Get(ctx, baseScope, pooled.ID); err != nil {
		t.Fatalf("closing an ephemeral session removed the pooled one: %v", err)
	}
	again := mustOpen(t, r, baseScope)
	if !again.Reused || again.ID != pooled.ID {
		t.Fatalf("pool must still serve the pooled session: %+v", again)
	}
	if _, err := r.Get(ctx, baseScope, e2.ID); err != nil {
		t.Fatalf("other ephemeral session lost: %v", err)
	}
}

type capturingDriver struct {
	engine.Driver
	cfg *atomic.Pointer[engine.ConnectionConfig]
}

func (d *capturingDriver) Connect(ctx context.Context, cfg engine.ConnectionConfig) error {
	d.cfg.Store(&cfg)
	return nil
}
func (d *capturingDriver) Close() error               { return nil }
func (d *capturingDriver) Ping(context.Context) error { return nil }

var capturedConfig atomic.Pointer[engine.ConnectionConfig]

func init() {
	engine.Register(engine.Registration{
		ID:          "capturecfg",
		DisplayName: "Capture",
		Dialect:     engine.DialectSQLite,
		New:         func() engine.Driver { return &capturingDriver{cfg: &capturedConfig} },
	})
}

func TestLocalOpenPassesTLSAndLimitsToDriver(t *testing.T) {
	tlsCfg := &engine.TLSConfig{Mode: engine.TLSMode("verify-full"), ServerName: "db.internal"}
	r, p := newTestRuntime(t, allowAll())
	p.creds = credentials.Credentials{Driver: "capturecfg", DSN: "x", DefaultScope: "public", TLS: tlsCfg}
	capturedConfig.Store(nil)

	_, err := r.Open(context.Background(), OpenRequest{Scope: baseScope, Limits: Limits{MaxRows: 123, MaxBytes: 4567}})
	if err != nil {
		t.Fatal(err)
	}
	got := capturedConfig.Load()
	if got == nil {
		t.Fatal("driver was not connected")
	}
	if got.TLS != tlsCfg || got.MaxResultRows != 123 || got.MaxResultBytes != 4567 ||
		got.DSN != "x" || got.Driver != "capturecfg" || got.DefaultScope != "public" {
		t.Fatalf("unexpected connection config: tls=%v rows=%d bytes=%d", got.TLS, got.MaxResultRows, got.MaxResultBytes)
	}
}

func TestLocalProbePassesTLSAndLimitsToDriver(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	tlsCfg := &engine.TLSConfig{Mode: engine.TLSMode("require")}
	capturedConfig.Store(nil)
	err := r.Probe(context.Background(), baseScope, credentials.Credentials{Driver: "capturecfg", DSN: "x", TLS: tlsCfg}, Limits{MaxRows: 7, MaxBytes: 9}, func(metadata.SchemaInspector) error { return nil })
	if !errors.Is(err, ErrSchemaUnsupported) {
		t.Fatalf("err = %v", err)
	}
	got := capturedConfig.Load()
	if got == nil || got.TLS != tlsCfg || got.MaxResultRows != 7 || got.MaxResultBytes != 9 {
		t.Fatalf("unexpected probe config: %+v", got)
	}
}

type scriptedDriver struct {
	engine.Driver
	connectErr error
	started    chan struct{}
	release    chan struct{}
}

func (d *scriptedDriver) Connect(context.Context, engine.ConnectionConfig) error { return d.connectErr }
func (d *scriptedDriver) Close() error                                           { return nil }
func (d *scriptedDriver) Query(context.Context, string, ...any) (*result.ResultSet, error) {
	close(d.started)
	<-d.release
	return &result.ResultSet{}, nil
}

var scriptedCurrent atomic.Pointer[scriptedDriver]

func init() {
	engine.Register(engine.Registration{
		ID:          "scripted",
		DisplayName: "Scripted",
		Dialect:     engine.DialectSQLite,
		New:         func() engine.Driver { return scriptedCurrent.Load() },
	})
}

func TestLocalOpenFailureLeavesNoSession(t *testing.T) {
	connectFailed := errors.New("connect failed")
	scriptedCurrent.Store(&scriptedDriver{connectErr: connectFailed})
	r, p := newTestRuntime(t, allowAll())
	p.creds = credentials.Credentials{Driver: "scripted", DSN: "x"}
	for _, eph := range []bool{false, true} {
		_, err := r.Open(context.Background(), OpenRequest{Scope: baseScope, Ephemeral: eph})
		var connectErr *ConnectError
		if !errors.As(err, &connectErr) || !errors.Is(err, connectFailed) {
			t.Fatalf("want ConnectError wrapping the driver error, got %v", err)
		}
	}
	if n, _ := r.CountForConnection(context.Background(), "c1"); n != 0 {
		t.Fatalf("failed open left %d sessions", n)
	}
	infos, err := r.List(context.Background(), SessionFilter{AccountID: "a1"})
	if err != nil || len(infos) != 0 {
		t.Fatalf("List = %+v, %v", infos, err)
	}
}

func TestLocalGetDoesNotWaitForRunningStatement(t *testing.T) {
	d := &scriptedDriver{started: make(chan struct{}), release: make(chan struct{})}
	scriptedCurrent.Store(d)
	r, p := newTestRuntime(t, allowAll())
	p.creds = credentials.Credentials{Driver: "scripted", DSN: "x"}
	info := mustOpen(t, r, baseScope)
	sess, err := r.session(baseScope, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	queryDone := make(chan struct{})
	go func() {
		defer close(queryDone)
		_, _ = sess.Query(context.Background(), "select 1")
	}()
	<-d.started
	defer func() {
		close(d.release)
		<-queryDone
	}()

	done := make(chan error, 3)
	go func() { _, err := r.Get(context.Background(), baseScope, info.ID); done <- err }()
	go func() { _, err := r.List(context.Background(), SessionFilter{AccountID: "a1"}); done <- err }()
	go func() { _, err := r.Open(context.Background(), OpenRequest{Scope: baseScope}); done <- err }()
	for i := 0; i < 3; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("session operations blocked behind a running statement")
		}
	}
}

func TestLocalTunnelHealthReachesSessionInfo(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := mustOpen(t, r, baseScope)
	if info.TunnelHealthy != nil {
		t.Fatal("session without a tunnel must report nil health")
	}
	sess, err := r.session(baseScope, info.ID)
	if err != nil {
		t.Fatal(err)
	}
	healthy := true
	sess.SetTunnelHealth(func() *bool { return &healthy })

	got, err := r.Get(context.Background(), baseScope, info.ID)
	if err != nil || got.TunnelHealthy == nil || !*got.TunnelHealthy {
		t.Fatalf("Get health = %v, %v", got.TunnelHealthy, err)
	}
	healthy = false
	listed, err := r.List(context.Background(), SessionFilter{AccountID: "a1"})
	if err != nil || len(listed) != 1 || listed[0].TunnelHealthy == nil || *listed[0].TunnelHealthy {
		t.Fatalf("List health = %+v, %v", listed, err)
	}
}

func TestLocalConcurrentOpenGetList(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	info := mustOpen(t, r, baseScope)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				if _, err := r.Open(context.Background(), OpenRequest{Scope: baseScope}); err != nil {
					t.Error(err)
					return
				}
				if _, err := r.Get(context.Background(), baseScope, info.ID); err != nil {
					t.Error(err)
					return
				}
				if _, err := r.List(context.Background(), SessionFilter{AccountID: "a1", WorkspaceID: "w1"}); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestLocalOnConnectionEmptyWithPrivateSessions(t *testing.T) {
	ctx := context.Background()
	r, _ := newTestRuntime(t, allowAll())
	emptied := make(chan string, 4)
	r.OnConnectionEmpty(func(id string) { emptied <- id })

	pooled := mustOpen(t, r, baseScope)
	priv, err := r.Open(ctx, OpenRequest{Scope: baseScope, Ephemeral: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(ctx, baseScope, pooled.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case id := <-emptied:
		t.Fatalf("hook fired while a private session remained: %s", id)
	default:
	}
	if err := r.Close(ctx, baseScope, priv.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-emptied:
	default:
		t.Fatal("hook did not fire after the last private session closed")
	}
}

var errPingFailed = errors.New("ping failed")

type pingFailDriver struct{ capturingDriver }

func (d *pingFailDriver) Ping(context.Context) error { return errPingFailed }

func init() {
	engine.Register(engine.Registration{
		ID:          "pingfail",
		DisplayName: "Ping Fail",
		Dialect:     engine.DialectSQLite,
		New: func() engine.Driver {
			return &pingFailDriver{capturingDriver{cfg: &atomic.Pointer[engine.ConnectionConfig]{}}}
		},
	})
}

func TestLocalProbePingFailureSkipsFn(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	called := false
	err := r.Probe(context.Background(), baseScope, credentials.Credentials{Driver: "pingfail", DSN: "x"}, Limits{}, func(metadata.SchemaInspector) error { called = true; return nil })
	var connectErr *ConnectError
	if !errors.Is(err, errPingFailed) || !errors.As(err, &connectErr) || called {
		t.Fatalf("err = %v called = %v", err, called)
	}
}

func TestLocalProbeWithoutConnectionID(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	called := false
	err := r.Probe(context.Background(), scopeOf("o1", "w1", "a1", ""), credentials.Credentials{Driver: "sqlite", DSN: ":memory:"}, Limits{}, func(metadata.SchemaInspector) error { called = true; return nil })
	if err != nil || !called {
		t.Fatalf("err = %v called = %v", err, called)
	}
}

func TestTunnelErrorUnwraps(t *testing.T) {
	inner := errors.New("handshake failed")
	var err error = &TunnelError{Err: inner}
	var te *TunnelError
	if !errors.Is(err, inner) || !errors.As(err, &te) || err.Error() != "ssh tunnel: handshake failed" {
		t.Fatalf("unexpected tunnel error: %v", err)
	}
}

func TestLocalProbeInvalidScope(t *testing.T) {
	r, _ := newTestRuntime(t, allowAll())
	called := false
	err := r.Probe(context.Background(), Scope{}, credentials.Credentials{Driver: "sqlite", DSN: ":memory:"}, Limits{}, func(metadata.SchemaInspector) error { called = true; return nil })
	if err == nil || called {
		t.Fatalf("err = %v called = %v", err, called)
	}
}
