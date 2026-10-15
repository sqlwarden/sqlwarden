package identity

import (
	"strings"

	"github.com/sqlwarden/internal/orgs"
)

const (
	MethodPassword = "password"
	MethodLocal    = "local"

	LocalAccountName      = "Local User"
	LocalAccountEmail     = "local@sqlwarden.local"
	LocalOrganizationName = "Local"
	LocalOrganizationSlug = "local"

	minPasswordLength = 8
)

// SetupInput is the decoded first-run setup request.
type SetupInput struct {
	Name             string
	Email            string
	Password         string
	OrganizationName string
	OrganizationSlug string
}

// SetupPlan is what first-run setup will create. Password is plaintext and
// must be hashed by the caller. A nil Password creates an account with no
// password credential.
type SetupPlan struct {
	AccountName      string
	AccountEmail     string
	Password         *string
	OrganizationName string
	OrganizationSlug string
	// SlugFallback lets the caller append a random suffix when the slug is
	// taken. When false, a taken slug is a field error.
	SlugFallback bool
	Method       string
}

// SetupStrategy turns setup input into a plan or field errors.
type SetupStrategy interface {
	RequiresInput() bool
	Plan(SetupInput) (SetupPlan, map[string]string)
}

var (
	FormSetup  SetupStrategy = formSetup{}
	LocalSetup SetupStrategy = localSetup{}
)

type formSetup struct{}

func (formSetup) RequiresInput() bool { return true }

func (formSetup) Plan(in SetupInput) (SetupPlan, map[string]string) {
	errs := map[string]string{}
	name := strings.TrimSpace(in.Name)
	email := strings.TrimSpace(in.Email)
	orgName := strings.TrimSpace(in.OrganizationName)
	slug := strings.TrimSpace(in.OrganizationSlug)

	if name == "" {
		errs["name"] = "Name is required."
	}
	if email == "" {
		errs["email"] = "Email is required."
	}
	if len(in.Password) < minPasswordLength {
		errs["password"] = "Password must be at least 8 characters."
	}
	if orgName == "" {
		errs["organization_name"] = "Organization name is required."
	}
	if slug == "" {
		slug = orgs.Slugify(orgName)
	}
	switch {
	case slug == "":
		errs["organization_slug"] = "Organization slug is required."
	case !orgs.ValidSlug(slug):
		errs["organization_slug"] = "Organization slug may only contain lowercase letters, numbers, and hyphens."
	case len(slug) > orgs.MaxSlugLength:
		errs["organization_slug"] = "Organization slug must be 64 characters or fewer."
	}
	if len(errs) > 0 {
		return SetupPlan{}, errs
	}
	password := in.Password
	return SetupPlan{
		AccountName: name, AccountEmail: email, Password: &password,
		OrganizationName: orgName, OrganizationSlug: slug, Method: MethodPassword,
	}, nil
}

type localSetup struct{}

func (localSetup) RequiresInput() bool { return false }

func (localSetup) Plan(SetupInput) (SetupPlan, map[string]string) {
	return SetupPlan{
		AccountName: LocalAccountName, AccountEmail: LocalAccountEmail,
		OrganizationName: LocalOrganizationName, OrganizationSlug: LocalOrganizationSlug,
		SlugFallback: true, Method: MethodLocal,
	}, nil
}
