package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/sqlwarden/internal/access"
	"github.com/sqlwarden/internal/response"
	"github.com/uptrace/bun"
)

type Organization struct {
	bun.BaseModel `bun:"table:organizations"`

	ID        int64     `bun:",pk,autoincrement" json:"id"`
	Slug      string    `bun:",notnull,unique"   json:"slug"`
	Name      string    `bun:",notnull"          json:"name"`
	CreatedAt time.Time `bun:",notnull"          json:"created_at"`
	UpdatedAt time.Time `bun:",notnull"          json:"updated_at"`
}

type OrganizationListItem struct {
	ID          int64     `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	MemberCount int       `json:"member_count"`
	TeamCount   int       `json:"team_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type AccountOrganizationListItem struct {
	ID          int64     `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Role        string    `json:"role"`
	MemberCount int       `json:"member_count"`
	TeamCount   int       `json:"team_count"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type OrgIDPConfig struct {
	bun.BaseModel `bun:"table:org_idp_configs"`

	ID          string    `bun:",pk"             json:"id"`
	OrgID       int64     `bun:",notnull,unique" json:"org_id"`
	Provider    string    `bun:",notnull"        json:"provider"`
	DisplayName string    `bun:",notnull"        json:"display_name"`
	Config      string    `bun:",notnull"        json:"-"`
	SSORequired bool      `bun:",notnull"        json:"sso_required"`
	IsActive    bool      `bun:",notnull,default:true" json:"is_active"`
	CreatedAt   time.Time `bun:",notnull"        json:"created_at"`
	UpdatedAt   time.Time `bun:",notnull"        json:"updated_at"`
}

type OrgMember struct {
	bun.BaseModel `bun:"table:org_members"`

	OrgID     int64     `bun:",pk" json:"org_id"`
	AccountID int64     `bun:",pk" json:"account_id"`
	JoinedAt  time.Time `bun:",notnull" json:"joined_at"`
}

type OrgMemberListItem struct {
	OrgID     int64     `json:"org_id"`
	AccountID int64     `json:"account_id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	JoinedAt  time.Time `json:"joined_at"`
}

type ListOrgMembersParams struct {
	OrgID    int64
	Search   string
	Role     string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

type ListAccountOrgsParams struct {
	AccountID int64
	Search    string
	Sort      string
	Order     string
	Page      int
	PageSize  int
}

type ListOrganizationsParams struct {
	Search   string
	Slug     string
	Sort     string
	Order    string
	Page     int
	PageSize int
}

func (db *DB) InsertOrg(ctx context.Context, slug, name string) (Organization, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.InsertOrgWithExecutor(ctx, db.DB, slug, name)
}

// InsertOrgWithExecutor inserts an organization using exec so callers can compose
// organization bootstrap in a larger transaction.
func (db *DB) InsertOrgWithExecutor(ctx context.Context, exec bun.IDB, slug, name string) (Organization, error) {
	org := Organization{
		Slug:      slug,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	_, err := exec.NewInsert().Model(&org).Returning("id").Exec(ctx)
	if err != nil {
		return Organization{}, err
	}
	return org, nil
}

func (db *DB) GetOrg(ctx context.Context, id int64) (Organization, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var org Organization
	err := db.NewSelect().Model(&org).Where("id = ?", id).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Organization{}, false, nil
	}
	if err != nil {
		return Organization{}, false, err
	}
	return org, true, nil
}

func (db *DB) GetOrgBySlug(ctx context.Context, slug string) (Organization, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var org Organization
	err := db.NewSelect().Model(&org).Where("slug = ?", slug).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return Organization{}, false, nil
	}
	if err != nil {
		return Organization{}, false, err
	}
	return org, true, nil
}

func (db *DB) UpdateOrg(ctx context.Context, id int64, name string) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	_, err := db.NewUpdate().
		Model((*Organization)(nil)).
		Set("name = ?", name).
		Set("updated_at = ?", time.Now()).
		Where("id = ?", id).
		Exec(ctx)
	return err
}

func (db *DB) DeleteOrg(ctx context.Context, id int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().TableExpr("resource_hierarchy").
			Where("owner_type = ? AND owner_id = ?", "org", id).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*RoleBinding)(nil)).
			Where("org_id = ?", id).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*Connection)(nil)).
			Where("workspace_id IN (SELECT id FROM workspaces WHERE owner_type = ? AND owner_id = ?)", "org", id).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*Environment)(nil)).
			Where("workspace_id IN (SELECT id FROM workspaces WHERE owner_type = ? AND owner_id = ?)", "org", id).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*Workspace)(nil)).
			Where("owner_type = ? AND owner_id = ?", "org", id).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*TeamMember)(nil)).
			Where("team_id IN (SELECT id FROM teams WHERE org_id = ?)", id).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*Team)(nil)).
			Where("org_id = ?", id).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*OrgMember)(nil)).
			Where("org_id = ?", id).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*OrgIDPConfig)(nil)).
			Where("org_id = ?", id).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*Role)(nil)).
			Where("org_id = ?", id).
			Exec(ctx); err != nil {
			return err
		}

		_, err := tx.NewDelete().Model((*Organization)(nil)).
			Where("id = ?", id).
			Exec(ctx)
		return err
	})
}

func (db *DB) AddOrgMember(ctx context.Context, orgID, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.AddOrgMemberWithExecutor(ctx, db.DB, orgID, accountID)
}

// AddOrgMemberWithExecutor adds an org member using exec for transaction composition.
func (db *DB) AddOrgMemberWithExecutor(ctx context.Context, exec bun.IDB, orgID, accountID int64) error {
	member := OrgMember{OrgID: orgID, AccountID: accountID, JoinedAt: time.Now()}
	_, err := exec.NewInsert().Model(&member).On("CONFLICT DO NOTHING").Exec(ctx)
	return err
}

func (db *DB) RemoveOrgMember(ctx context.Context, orgID, accountID int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.RemoveOrgMemberAccess(ctx, orgID, accountID, nil, "")
}

// RemoveOrgMemberAccess removes an account from an organization and atomically
// clears derived memberships, direct account policy bindings, and org sessions.
func (db *DB) RemoveOrgMemberAccess(ctx context.Context, orgID, accountID int64, revokedBy *int64, reason string) error {
	return db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewDelete().Model((*WorkspaceMember)(nil)).
			Where("account_id = ?", accountID).
			Where("workspace_id IN (SELECT id FROM workspaces WHERE owner_type = 'org' AND owner_id = ?)", orgID).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*TeamMember)(nil)).
			Where("account_id = ?", accountID).
			Where("team_id IN (SELECT id FROM teams WHERE org_id = ?)", orgID).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*RoleBinding)(nil)).
			Where("org_id = ? AND subject_type = ? AND subject_id = ?", orgID, "account", accountID).
			Exec(ctx); err != nil {
			return err
		}

		if _, err := tx.NewDelete().Model((*OrgMember)(nil)).
			Where("org_id = ? AND account_id = ?", orgID, accountID).Exec(ctx); err != nil {
			return err
		}

		_, err := tx.NewUpdate().
			Model((*OrgAccessSession)(nil)).
			Set("revoked_at = ?", time.Now()).
			Set("revoked_by_account_id = ?", revokedBy).
			Set("revocation_reason = ?", reason).
			Where("org_id = ? AND account_id = ? AND revoked_at IS NULL", orgID, accountID).
			Exec(ctx)
		return err
	})
}

func (db *DB) GetOrgMembers(ctx context.Context, orgID int64) ([]OrgMember, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var members []OrgMember
	err := db.NewSelect().Model(&members).Where("org_id = ?", orgID).Scan(ctx)
	return members, err
}

func (db *DB) listOrgMembers(ctx context.Context, params ListOrgMembersParams) ([]OrgMemberListItem, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeOrgMemberParams(params)

	query := `
SELECT
	om.org_id,
	om.account_id,
	a.email,
	a.name,
	COALESCE(MAX(CASE WHEN ro.name IN (?, ?) THEN ro.name END), '') AS role,
	om.joined_at
FROM org_members AS om
JOIN accounts AS a ON a.id = om.account_id
LEFT JOIN role_bindings AS rb
	ON rb.org_id = om.org_id
	AND rb.subject_type = 'account'
	AND rb.subject_id = om.account_id
	AND rb.resource_type = 'org'
	AND rb.resource_id = om.org_id
LEFT JOIN roles AS ro ON ro.id = rb.role_id
WHERE om.org_id = ?`

	args := []any{access.BuiltinOrgOwnerRole, access.BuiltinOrgAdminRole, params.OrgID}
	if params.Search != "" {
		query += " AND (LOWER(a.name) LIKE ? OR LOWER(a.email) LIKE ?)"
		search := "%" + strings.ToLower(params.Search) + "%"
		args = append(args, search, search)
	}
	if params.Role != "" {
		query += " AND EXISTS (SELECT 1 FROM roles r JOIN role_bindings rb_role ON rb_role.role_id = r.id WHERE rb_role.org_id = om.org_id AND rb_role.subject_type = 'account' AND rb_role.subject_id = om.account_id AND rb_role.resource_type = 'org' AND rb_role.resource_id = om.org_id AND r.name = ?)"
		args = append(args, params.Role)
	}

	query += fmt.Sprintf(`
GROUP BY om.org_id, om.account_id, a.email, a.name, om.joined_at
ORDER BY %s %s, om.account_id %s`, orgMemberSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))

	var items []OrgMemberListItem
	err := db.NewRaw(query, args...).Scan(ctx, &items)
	return items, err
}

func (db *DB) ListOrgMembersPage(ctx context.Context, params ListOrgMembersParams) (response.Paginated[OrgMemberListItem], error) {
	items, err := db.listOrgMembers(ctx, params)
	if err != nil {
		return response.Paginated[OrgMemberListItem]{}, err
	}
	return response.PaginateItems(items, params.Page, params.PageSize), nil
}

func (db *DB) GetOrgMember(ctx context.Context, orgID, accountID int64) (OrgMemberListItem, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	query := `
SELECT
	om.org_id,
	om.account_id,
	a.email,
	a.name,
	COALESCE(MAX(CASE WHEN ro.name IN (?, ?) THEN ro.name END), '') AS role,
	om.joined_at
FROM org_members AS om
JOIN accounts AS a ON a.id = om.account_id
LEFT JOIN role_bindings AS rb
	ON rb.org_id = om.org_id
	AND rb.subject_type = 'account'
	AND rb.subject_id = om.account_id
	AND rb.resource_type = 'org'
	AND rb.resource_id = om.org_id
LEFT JOIN roles AS ro ON ro.id = rb.role_id
WHERE om.org_id = ? AND om.account_id = ?
GROUP BY om.org_id, om.account_id, a.email, a.name, om.joined_at`

	var item OrgMemberListItem
	err := db.NewRaw(query, access.BuiltinOrgOwnerRole, access.BuiltinOrgAdminRole, orgID, accountID).Scan(ctx, &item)
	if errors.Is(err, sql.ErrNoRows) {
		return OrgMemberListItem{}, false, nil
	}
	if err != nil {
		return OrgMemberListItem{}, false, err
	}
	return item, true, nil
}

func (db *DB) IsOrgMember(ctx context.Context, orgID, accountID int64) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	return db.NewSelect().Model((*OrgMember)(nil)).
		Where("org_id = ? AND account_id = ?", orgID, accountID).Exists(ctx)
}

func (db *DB) GetAccountOrgs(ctx context.Context, accountID int64) ([]Organization, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var orgs []Organization
	err := db.NewSelect().Model(&orgs).
		Join("JOIN org_members AS om ON om.org_id = organization.id").
		Where("om.account_id = ?", accountID).
		Scan(ctx)
	return orgs, err
}

func (db *DB) ListOrganizationsPage(ctx context.Context, params ListOrganizationsParams) (response.Paginated[OrganizationListItem], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeOrganizationParams(params)

	query := `
SELECT
	o.id,
	o.slug,
	o.name,
	(SELECT COUNT(*) FROM org_members AS om WHERE om.org_id = o.id) AS member_count,
	(SELECT COUNT(*) FROM teams AS t WHERE t.org_id = o.id) AS team_count,
	o.created_at,
	o.updated_at
FROM organizations AS o
WHERE 1 = 1`
	args := []any{}
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query += " AND (LOWER(o.name) LIKE ? OR LOWER(o.slug) LIKE ?)"
		args = append(args, search, search)
	}
	if params.Slug != "" {
		query += " AND o.slug = ?"
		args = append(args, params.Slug)
	}

	query += fmt.Sprintf(`
ORDER BY %s %s, o.id %s`, organizationSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))

	var orgs []OrganizationListItem
	err := db.NewRaw(query, args...).Scan(ctx, &orgs)
	if err != nil {
		return response.Paginated[OrganizationListItem]{}, err
	}
	return response.PaginateItems(orgs, params.Page, params.PageSize), nil
}

func (db *DB) ListAccountOrgsPage(ctx context.Context, params ListAccountOrgsParams) (response.Paginated[AccountOrganizationListItem], error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	params = normalizeAccountOrgParams(params)

	query := `
SELECT
	o.id,
	o.slug,
	o.name,
	COALESCE(MAX(CASE WHEN ro.name IN (?, ?) THEN ro.name END), ?) AS role,
	(SELECT COUNT(*) FROM org_members AS omc WHERE omc.org_id = o.id) AS member_count,
	(SELECT COUNT(*) FROM teams AS tc WHERE tc.org_id = o.id) AS team_count,
	o.created_at,
	o.updated_at
FROM organizations AS o
JOIN org_members AS om ON om.org_id = o.id
LEFT JOIN role_bindings AS rb
	ON rb.org_id = o.id
	AND rb.subject_type = 'account'
	AND rb.subject_id = om.account_id
	AND rb.resource_type = 'org'
	AND rb.resource_id = o.id
LEFT JOIN roles AS ro ON ro.id = rb.role_id
WHERE om.account_id = ?`

	args := []any{access.BuiltinOrgOwnerRole, access.BuiltinOrgAdminRole, access.BuiltinOrgMemberRole, params.AccountID}
	if params.Search != "" {
		search := "%" + strings.ToLower(params.Search) + "%"
		query += " AND (LOWER(o.name) LIKE ? OR LOWER(o.slug) LIKE ?)"
		args = append(args, search, search)
	}

	query += fmt.Sprintf(`
GROUP BY o.id, o.slug, o.name, o.created_at, o.updated_at
ORDER BY %s %s, o.id %s`, accountOrgSortColumn(params.Sort), strings.ToUpper(params.Order), strings.ToUpper(params.Order))

	var orgs []AccountOrganizationListItem
	err := db.NewRaw(query, args...).Scan(ctx, &orgs)
	if err != nil {
		return response.Paginated[AccountOrganizationListItem]{}, err
	}
	return response.PaginateItems(orgs, params.Page, params.PageSize), nil
}

func normalizeOrgMemberParams(params ListOrgMembersParams) ListOrgMembersParams {
	if params.Sort == "" {
		params.Sort = "name"
	}
	switch params.Sort {
	case "name", "email", "joined_at":
	default:
		params.Sort = "name"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "asc"
	}
	params.Search = strings.TrimSpace(params.Search)
	params.Role = strings.TrimSpace(params.Role)
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func normalizeAccountOrgParams(params ListAccountOrgsParams) ListAccountOrgsParams {
	if params.Sort == "" {
		params.Sort = "name"
	}
	switch params.Sort {
	case "name", "slug", "created_at":
	default:
		params.Sort = "name"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "asc"
	}
	params.Search = strings.TrimSpace(params.Search)
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func normalizeOrganizationParams(params ListOrganizationsParams) ListOrganizationsParams {
	if params.Sort == "" {
		params.Sort = "name"
	}
	switch params.Sort {
	case "name", "slug", "created_at":
	default:
		params.Sort = "name"
	}
	switch params.Order {
	case "asc", "desc":
	default:
		params.Order = "asc"
	}
	params.Search = strings.TrimSpace(params.Search)
	params.Slug = strings.TrimSpace(params.Slug)
	if params.Page < 1 {
		params.Page = 1
	}
	if params.PageSize < 1 {
		params.PageSize = 25
	}
	return params
}

func orgMemberSortColumn(sort string) string {
	switch sort {
	case "email":
		return "a.email"
	case "joined_at":
		return "om.joined_at"
	default:
		return "a.name"
	}
}

func accountOrgSortColumn(sort string) string {
	switch sort {
	case "slug":
		return "o.slug"
	case "created_at":
		return "o.created_at"
	default:
		return "o.name"
	}
}

func organizationSortColumn(sort string) string {
	switch sort {
	case "slug":
		return "o.slug"
	case "created_at":
		return "o.created_at"
	default:
		return "o.name"
	}
}

// DeleteAccountRoleBindings removes account-scoped role bindings for a specific resource.
func (db *DB) DeleteAccountRoleBindings(ctx context.Context, orgID, accountID int64, resourceType string, resourceID int64, roleIDs []int64) error {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	if len(roleIDs) == 0 {
		return nil
	}

	_, err := db.NewDelete().
		TableExpr("role_bindings").
		Where("org_id = ? AND subject_type = 'account' AND subject_id = ? AND resource_type = ? AND resource_id = ?",
			orgID, accountID, resourceType, resourceID).
		Where("role_id IN (?)", bun.List(roleIDs)).
		Exec(ctx)
	return err
}

func (db *DB) UpsertOrgIDPConfig(ctx context.Context, orgID int64, provider, displayName, encryptedConfig string, ssoRequired bool) (OrgIDPConfig, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	now := time.Now()
	config := OrgIDPConfig{
		ID:          newID(),
		OrgID:       orgID,
		Provider:    provider,
		DisplayName: displayName,
		Config:      encryptedConfig,
		SSORequired: ssoRequired,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := db.NewInsert().Model(&config).
		On("CONFLICT (org_id) DO UPDATE").
		Set("provider = EXCLUDED.provider").
		Set("display_name = EXCLUDED.display_name").
		Set("config = EXCLUDED.config").
		Set("sso_required = EXCLUDED.sso_required").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	if err != nil {
		return OrgIDPConfig{}, err
	}
	return config, nil
}

func (db *DB) GetOrgIDPConfig(ctx context.Context, orgID int64) (OrgIDPConfig, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, defaultTimeout)
	defer cancel()

	var config OrgIDPConfig
	err := db.NewSelect().Model(&config).Where("org_id = ?", orgID).Scan(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return OrgIDPConfig{}, false, nil
	}
	if err != nil {
		return OrgIDPConfig{}, false, err
	}
	return config, true, nil
}
