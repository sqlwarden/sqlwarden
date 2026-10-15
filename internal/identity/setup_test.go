package identity

import (
	"strings"
	"testing"
)

func TestFormSetupValidInput(t *testing.T) {
	plan, errs := FormSetup.Plan(SetupInput{
		Name: " Ada ", Email: " ada@example.com ", Password: "longpassword",
		OrganizationName: " Acme Corp ",
	})
	if len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	if plan.AccountName != "Ada" || plan.AccountEmail != "ada@example.com" {
		t.Fatalf("account = %q %q", plan.AccountName, plan.AccountEmail)
	}
	if plan.Password == nil || *plan.Password != "longpassword" {
		t.Fatal("password not carried")
	}
	if plan.OrganizationName != "Acme Corp" || plan.OrganizationSlug != "acme-corp" {
		t.Fatalf("org = %q %q", plan.OrganizationName, plan.OrganizationSlug)
	}
	if plan.SlugFallback || plan.Method != MethodPassword {
		t.Fatalf("fallback=%v method=%q", plan.SlugFallback, plan.Method)
	}
	if !FormSetup.RequiresInput() {
		t.Fatal("FormSetup must require input")
	}
}

func TestFormSetupFieldErrors(t *testing.T) {
	_, errs := FormSetup.Plan(SetupInput{Password: "short", OrganizationSlug: "Bad_Slug"})
	for _, field := range []string{"name", "email", "password", "organization_name", "organization_slug"} {
		if errs[field] == "" {
			t.Errorf("missing field error for %s", field)
		}
	}
}

func TestFormSetupSlugTooLong(t *testing.T) {
	_, errs := FormSetup.Plan(SetupInput{
		Name: "a", Email: "a@b.c", Password: "longpassword",
		OrganizationName: "x", OrganizationSlug: strings.Repeat("a", 65),
	})
	if errs["organization_slug"] == "" {
		t.Fatal("expected slug length error")
	}
}

func TestLocalSetupIgnoresInput(t *testing.T) {
	plan, errs := LocalSetup.Plan(SetupInput{Name: "x", Password: "p"})
	if len(errs) != 0 {
		t.Fatalf("errs = %v", errs)
	}
	if LocalSetup.RequiresInput() {
		t.Fatal("LocalSetup must not require input")
	}
	want := SetupPlan{
		AccountName: LocalAccountName, AccountEmail: LocalAccountEmail,
		OrganizationName: LocalOrganizationName, OrganizationSlug: LocalOrganizationSlug,
		SlugFallback: true, Method: MethodLocal,
	}
	if plan != want {
		t.Fatalf("plan = %+v, want %+v", plan, want)
	}
}
