package oracle

import (
	"context"
	"fmt"

	"github.com/sqlwarden/internal/engine/metadata"
)

// oracleSystemSchemas supplements ALL_USERS.ORACLE_MAINTAINED when classifying
// scope listings. The browser hides system schemas by default; discovery omits
// them unless they are the current schema.
var oracleSystemSchemas = map[string]struct{}{
	"SYS": {}, "SYSTEM": {}, "XDB": {}, "CTXSYS": {}, "MDSYS": {}, "OUTLN": {},
	"DBSNMP": {}, "APPQOSSYS": {}, "GSMADMIN_INTERNAL": {}, "AUDSYS": {},
	"LBACSYS": {}, "DVSYS": {}, "ORDSYS": {}, "ORDDATA": {}, "WMSYS": {},
	"OJVMSYS": {}, "DBSFWUSER": {}, "REMOTE_SCHEDULER_AGENT": {}, "SYS$UMF": {},
	"ANONYMOUS": {}, "APEX_PUBLIC_USER": {}, "FLOWS_FILES": {}, "OLAPSYS": {},
	"SI_INFORMTN_SCHEMA": {}, "DIP": {}, "ORACLE_OCM": {}, "XS$NULL": {},
}

// DiscoverScopes lists visible non-system schemas for connection setup,
// always including the current schema. Oracle schemas are top-level scopes,
// so a request with a parent has no children.
func (d *oracleDriver) DiscoverScopes(ctx context.Context, request metadata.ScopeDiscoveryRequest) (*metadata.ScopeDiscovery, error) {
	current, err := d.currentSchema(ctx)
	if err != nil {
		return nil, err
	}
	result := &metadata.ScopeDiscovery{Scopes: []metadata.ScopePath{}}
	if current != "" {
		result.Current = metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: current})
	}
	if request.Parent != "" {
		return result, nil
	}
	nodes, err := d.visibleSchemas(ctx)
	if err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if !node.System || node.Path == result.Current {
			result.Scopes = append(result.Scopes, node.Path)
		}
	}
	return result, nil
}

// visibleSchemas lists users visible through ALL_USERS, including schemas with
// no accessible objects. Object owners supplement that list (for example PUBLIC),
// and current/login schemas are retained even when empty. Listing a schema does
// not imply object privileges; expansion still uses the permission-aware ALL_*
// views to inspect only accessible objects.
func (d *oracleDriver) visibleSchemas(ctx context.Context) ([]metadata.ScopeNode, error) {
	rows, err := d.db.QueryContext(ctx, `
SELECT owners.owner, NVL(users.oracle_maintained, 'N')
FROM (
  SELECT username AS owner FROM all_users
  UNION SELECT owner FROM all_objects
  UNION SELECT SYS_CONTEXT('USERENV','CURRENT_SCHEMA') FROM dual
  UNION SELECT USER FROM dual
) owners
LEFT JOIN all_users users ON users.username = owners.owner
ORDER BY owners.owner`)
	if err != nil {
		return nil, fmt.Errorf("oracle: discover schemas: %w", err)
	}
	defer rows.Close()
	nodes := []metadata.ScopeNode{}
	for rows.Next() {
		var owner, maintained string
		if err := rows.Scan(&owner, &maintained); err != nil {
			return nil, fmt.Errorf("oracle: discover schema: %w", err)
		}
		_, knownSystem := oracleSystemSchemas[owner]
		nodes = append(nodes, metadata.ScopeNode{
			Path:   metadata.NewScopePath(metadata.ScopeSegment{Kind: "schema", Name: owner}),
			Groups: []metadata.ObjectGroup{}, Lazy: true,
			System: maintained == "Y" || knownSystem || owner == "PUBLIC",
		})
	}
	return nodes, rows.Err()
}
