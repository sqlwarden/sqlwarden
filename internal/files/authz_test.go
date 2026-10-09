package files

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/access/accesstest"
	"github.com/sqlwarden/internal/audit"
	"github.com/sqlwarden/internal/database"
)

func accountPrincipal(id int64) access.Principal {
	return access.Principal{Subject: access.SubjectRef{Kind: access.SubjectAccount, ID: id}}
}

// dbDirectory adapts the metadata database to access.Directory for tests.
type dbDirectory struct{ db *database.DB }

func (d dbDirectory) GetOrg(ctx context.Context, id int64) (access.Org, bool, error) {
	org, found, err := d.db.GetOrg(ctx, id)
	if err != nil || !found {
		return access.Org{}, found, err
	}
	return access.Org{ID: org.ID, Slug: org.Slug, Name: org.Name}, true, nil
}

func (d dbDirectory) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	return d.db.IsOrgMember(ctx, orgID, accountID)
}

func (d dbDirectory) IsEffectiveWorkspaceMember(ctx context.Context, orgID, workspaceID, accountID int64) (bool, error) {
	return d.db.IsEffectiveWorkspaceMember(ctx, orgID, workspaceID, accountID)
}

func (d dbDirectory) GetWorkspace(ctx context.Context, id int64) (access.Workspace, bool, error) {
	ws, found, err := d.db.GetWorkspace(ctx, id)
	if err != nil || !found {
		return access.Workspace{}, found, err
	}
	return access.Workspace{ID: ws.ID, OrgID: ws.OrgID, OwnerType: ws.OwnerType, OwnerID: ws.OwnerID, Name: ws.Name}, true, nil
}

func (d dbDirectory) GetConnection(ctx context.Context, id int64) (access.Connection, bool, error) {
	conn, found, err := d.db.GetConnection(ctx, id)
	if err != nil || !found {
		return access.Connection{}, found, err
	}
	return access.Connection{ID: conn.ID, WorkspaceID: conn.WorkspaceID, EnvironmentID: conn.EnvironmentID, Name: conn.Name, Driver: conn.Driver}, true, nil
}

type policyEvaluator struct{ enforcer *fakeEnforcer }

func (p policyEvaluator) Can(ctx context.Context, accountID, orgID int64, ownerType, resourceType string, resourceID int64, permission string) bool {
	return p.enforcer.Can(ctx, accountID, orgID, ownerType, resourceType, resourceID, permission)
}

func (p policyEvaluator) EffectivePermissions(context.Context, int64, int64, string, string, int64) ([]string, error) {
	return nil, nil
}

// memoryDirectory has one account that belongs to org 1 only, with workspace
// 10 in org 1 and workspace 20 in org 2.
type memoryDirectory struct{}

func (memoryDirectory) GetOrg(_ context.Context, id int64) (access.Org, bool, error) {
	return access.Org{ID: id, Slug: "org", Name: "Org"}, true, nil
}

func (memoryDirectory) IsOrgMember(_ context.Context, orgID, accountID int64) (bool, error) {
	return orgID == 1 && accountID == 5, nil
}

func (memoryDirectory) IsEffectiveWorkspaceMember(context.Context, int64, int64, int64) (bool, error) {
	return true, nil
}

func (memoryDirectory) GetWorkspace(_ context.Context, id int64) (access.Workspace, bool, error) {
	orgs := map[int64]int64{10: 1, 20: 2}
	orgID, ok := orgs[id]
	if !ok {
		return access.Workspace{}, false, nil
	}
	return access.Workspace{ID: id, OrgID: &orgID, OwnerType: "org", OwnerID: orgID, Name: "ws"}, true, nil
}

func (memoryDirectory) GetConnection(context.Context, int64) (access.Connection, bool, error) {
	return access.Connection{}, false, nil
}

// serviceCalls invokes every principal-scoped Service method with valid input
// so a failure can only come from authorization.
var serviceCalls = map[string]func(context.Context, *Service, access.Principal, Scope) error{
	"List": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		_, err := s.List(ctx, p, sc, nil)
		return err
	},
	"Browser": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		_, err := s.Browser(ctx, p, sc, nil)
		return err
	},
	"Recent": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		_, err := s.Recent(ctx, p, sc, 5)
		return err
	},
	"Create": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		_, err := s.Create(ctx, p, sc, CreateInput{Name: "a.sql"})
		return err
	},
	"Get": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		_, err := s.Get(ctx, p, sc, 1)
		return err
	},
	"Update": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		name := "b.sql"
		_, err := s.Update(ctx, p, sc, 1, UpdateInput{Name: &name})
		return err
	},
	"Duplicate": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		_, err := s.Duplicate(ctx, p, sc, 1, DuplicateInput{Name: "c.sql"})
		return err
	},
	"Delete": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		return s.Delete(ctx, p, sc, 1)
	},
	"ReadContent": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		_, err := s.ReadContent(ctx, p, sc, 1)
		return err
	},
	"WriteContent": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		_, err := s.WriteContent(ctx, p, sc, 1, "", strings.NewReader("x"))
		return err
	},
	"Search": func(ctx context.Context, s *Service, p access.Principal, sc Scope) error {
		_, err := s.Search(ctx, p, sc, SearchInput{Query: "select"})
		return err
	},
}

// systemMethods run without a caller and therefore take no principal.
var systemMethods = map[string]bool{
	"ReapContentDeletionsOnce": true,
	"RunContentDeletionReaper": true,
}

func TestErrorsAliasAccessErrors(t *testing.T) {
	if !errors.Is(ErrForbidden, access.ErrForbidden) || !errors.Is(ErrNotFound, access.ErrNotFound) {
		t.Fatal("files errors must alias the access errors")
	}
}

func TestExportedServiceMethodsTakePrincipalSecond(t *testing.T) {
	principalType := reflect.TypeOf(access.Principal{})
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	serviceType := reflect.TypeOf(&Service{})

	var covered []string
	for i := 0; i < serviceType.NumMethod(); i++ {
		method := serviceType.Method(i)
		if systemMethods[method.Name] {
			continue
		}
		covered = append(covered, method.Name)
		if method.Type.NumIn() < 3 || method.Type.In(1) != contextType || method.Type.In(2) != principalType {
			t.Errorf("%s must take (context.Context, access.Principal, ...)", method.Name)
		}
		if _, ok := serviceCalls[method.Name]; !ok {
			t.Errorf("%s has no entry in serviceCalls", method.Name)
		}
	}
	sort.Strings(covered)
	if len(covered) != len(serviceCalls) {
		t.Errorf("serviceCalls lists %d methods, Service exports %d principal-scoped methods (%v)", len(serviceCalls), len(covered), covered)
	}
}

func TestServiceAuthorizesBeforeTouchingStore(t *testing.T) {
	requirer := access.NewRequirer(memoryDirectory{}, nil)
	valid := Scope{OrgID: 1, WorkspaceID: 10, Visibility: database.FileVisibilityPrivate}

	tests := []struct {
		name      string
		requirer  *access.Requirer
		principal access.Principal
		scope     Scope
		want      error
	}{
		{"wrong org id", requirer, accountPrincipal(5), Scope{OrgID: 2, WorkspaceID: 10, Visibility: valid.Visibility}, ErrForbidden},
		{"workspace in another org", requirer, accountPrincipal(5), Scope{OrgID: 1, WorkspaceID: 20, Visibility: valid.Visibility}, ErrNotFound},
		{"unknown workspace", requirer, accountPrincipal(5), Scope{OrgID: 1, WorkspaceID: 99, Visibility: valid.Visibility}, ErrNotFound},
		{"non-member account", requirer, accountPrincipal(6), valid, ErrForbidden},
		{"non-account subject", requirer, access.Principal{Subject: access.SubjectRef{Kind: access.SubjectServiceAccount, ID: 5}}, valid, ErrForbidden},
		{"nil requirer", nil, accountPrincipal(5), valid, ErrForbidden},
	}

	for _, storageMode := range []string{StorageModeObject, StorageModeFile} {
		for _, tt := range tests {
			for name, call := range serviceCalls {
				t.Run(storageMode+"/"+tt.name+"/"+name, func(t *testing.T) {
					// A nil database panics on first use, which proves the store
					// was never reached.
					service := NewService(Deps{Requirer: tt.requirer}, Config{StorageMode: storageMode}, nil)
					defer func() {
						if recovered := recover(); recovered != nil {
							t.Fatalf("store touched before authorization: %v", recovered)
						}
					}()
					err := call(context.Background(), service, tt.principal, tt.scope)
					accesstest.AssertDenies(t, err, tt.want, "service call")
				})
			}
		}
	}
}

func TestDeleteEmitsAuditAndFailsOpen(t *testing.T) {
	for _, auditErr := range []error{nil, errors.New("audit down")} {
		f := newServiceFixture(t, Config{StorageMode: StorageModeObject, RevisionPolicy: RevisionPolicyDisabled})
		var events []audit.Event
		f.service.audit = audit.WriterFunc(func(_ context.Context, event audit.Event) error {
			events = append(events, event)
			return auditErr
		})
		principal, scope := f.as(f.member), f.privateTree()
		file, err := f.service.Create(f.ctx, principal, scope, CreateInput{Name: "gone.sql"})
		if err != nil {
			t.Fatal(err)
		}
		if err := f.service.Delete(f.ctx, principal, scope, file.ID); err != nil {
			t.Fatalf("delete with audit error %v: %v", auditErr, err)
		}
		if len(events) != 1 || events[0].Action != "file.deleted" || events[0].Resource != "workspace_file" || events[0].Actor.SubjectID == nil || *events[0].Actor.SubjectID != f.member.ID {
			t.Fatalf("audit events = %+v", events)
		}
	}
}
