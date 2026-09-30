package oracle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/sijms/go-ora/v2/network"

	"github.com/sqlwarden/internal/engine/metadata"
)

var _ metadata.SchemaInspector = (*oracleDriver)(nil)

func (d *oracleDriver) Tree() metadata.Tree {
	return navigatorTree
}

// Querier returns the session pool for every schema: catalog queries filter
// the ALL_* views by owner, so one pool serves all schemas.
func (d *oracleDriver) Querier(context.Context, string) (metadata.Querier, error) {
	if d.db == nil {
		return nil, errors.New("oracle: not connected")
	}
	return d.db, nil
}

func folder(kind, label, child string, order int, list metadata.Loader) metadata.Folder {
	return metadata.Folder{Kind: kind, Label: label, Child: child, Order: order, List: list}
}

func leaf(label, icon string) metadata.Node {
	return metadata.Node{Label: label, Icon: icon, Leaf: true}
}

func definedLeaf(label, icon string) metadata.Node {
	return metadata.Node{Label: label, Icon: icon, Leaf: true, HasDefinition: true}
}

var (
	columnsFolder      = folder("columns", "Columns", "column", 10, ListColumns)
	constraintsFolder  = folder("constraints", "Constraints", "constraint", 20, ListConstraints)
	foreignKeysFolder  = folder("foreign_keys", "Foreign Keys", "foreign_key", 30, ListForeignKeys)
	referencesFolder   = folder("references", "References", "reference", 40, ListReferences)
	triggersFolder     = folder("triggers", "Triggers", "trigger", 50, ListTableTriggers)
	indexesFolder      = folder("indexes", "Indexes", "index", 60, ListTableIndexes)
	dependenciesFolder = folder("dependencies", "Dependencies", "dependency", 70, ListDependencies)
)

var navigatorTree = metadata.Tree{
	SystemObjects: true,
	Root: metadata.Node{Label: "Connection", Icon: "connection", Folders: []metadata.Folder{
		folder("schemas", "Schemas", "schema", 10, ListSchemas),
		folder("types", "Types", "type", 20, ListGlobalTypes),
		folder("users", "Users", "user", 30, ListUsers),
		folder("roles", "Roles", "role", 40, ListRoles),
		folder("profiles", "Profiles", "profile", 50, ListProfiles),
	}},
	Nodes: map[string]metadata.Node{
		"schema": {Label: "Schema", Icon: "schema", Scope: true, ShowAllDatabases: true, SupportsDiagram: true, Folders: []metadata.Folder{
			folder("tables", "Tables", "table", 10, ListTables),
			folder("views", "Views", "view", 20, ListViews),
			folder("materialized_views", "Materialized Views", "materialized_view", 30, ListMaterializedViews),
			folder("indexes", "Indexes", "index", 40, ListSchemaIndexes),
			folder("sequences", "Sequences", "sequence", 50, ListSequences),
			folder("queues", "Queues", "queue", 60, ListQueues),
			folder("types", "Types", "type", 70, ListTypes),
			folder("packages", "Packages", "package", 80, ListPackages),
			folder("procedures", "Procedures", "procedure", 90, ListProcedures),
			folder("functions", "Functions", "function", 100, ListFunctions),
			folder("synonyms", "Synonyms", "synonym", 110, ListSynonyms),
			folder("schema_triggers", "Schema Triggers", "trigger", 120, ListSchemaTriggers),
			folder("table_triggers", "Table Triggers", "trigger", 130, ListSchemaTableTriggers),
			folder("db_links", "Database Links", "db_link", 140, ListDBLinks),
		}},
		"table": {Label: "Table", Icon: "table", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, constraintsFolder, foreignKeysFolder, referencesFolder, triggersFolder, indexesFolder, dependenciesFolder,
		}},
		"view": {Label: "View", Icon: "view", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, constraintsFolder, triggersFolder, dependenciesFolder,
		}},
		"materialized_view": {Label: "Materialized View", Icon: "materialized_view", Relational: true, SupportsDiagram: true, HasDefinition: true, Folders: []metadata.Folder{
			columnsFolder, constraintsFolder, indexesFolder, dependenciesFolder,
		}},
		"column":      {Label: "Column", Icon: "column", Leaf: true, Column: true},
		"constraint":  definedLeaf("Constraint", "constraint"),
		"foreign_key": definedLeaf("Foreign Key", "foreign_key"),
		"reference":   leaf("Reference", "reference"),
		"index":       definedLeaf("Index", "index"),
		"dependency":  leaf("Dependency", "dependency"),
		"sequence":    definedLeaf("Sequence", "sequence"),
		"queue":       definedLeaf("Queue", "queue"),
		"type":        definedLeaf("Type", "type"),
		"package":     definedLeaf("Package", "package"),
		"procedure":   definedLeaf("Procedure", "procedure"),
		"function":    definedLeaf("Function", "function"),
		"synonym":     definedLeaf("Synonym", "synonym"),
		"db_link":     definedLeaf("Database Link", "db_link"),
		"trigger":     definedLeaf("Trigger", "trigger"),
		"user":        leaf("User", "user"),
		"role":        leaf("Role", "role"),
		"profile":     leaf("Profile", "profile"),
	},
}

func newListing(parents []metadata.ScopePath) map[metadata.ScopePath][]metadata.Child {
	out := make(map[metadata.ScopePath][]metadata.Child, len(parents))
	for _, p := range parents {
		out[p] = []metadata.Child{}
	}
	return out
}

// scanListing runs query and appends each scanned child under the parent the
// scan function resolves; rows for unknown parents are dropped.
func scanListing(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string, args []any,
	scan func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle: list %s: %w", label, err)
	}
	defer rows.Close()
	out := newListing(parents)
	for rows.Next() {
		parent, child, ok, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("oracle: scan %s: %w", label, err)
		}
		if ok {
			out[parent] = append(out[parent], child)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("oracle: list %s rows: %w", label, err)
	}
	return out, nil
}

// listShared runs one query and gives every parent the same children. Root
// folders have the single parent ""; schema folders' batches always share one
// schema, which is bound as the query's first argument.
func listShared(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string, args []any,
	scan func(rows *sql.Rows) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	listed, err := scanListing(ctx, q, parents[:1], label, query, args, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		child, err := scan(rows)
		return parents[0], child, true, err
	})
	if err != nil {
		return nil, err
	}
	out := newListing(parents)
	for _, p := range parents {
		out[p] = listed[parents[0]]
	}
	return out, nil
}

// listInSchema binds parents[0]'s schema as :1, then extra from :2: a batch's
// parents must share one schema.
func listInSchema(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string,
	scan func(rows *sql.Rows) (metadata.Child, error), extra ...any) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	return listShared(ctx, q, parents, label, query, append([]any{parents[0].Name("schema")}, extra...), scan)
}

// isMissingDictionaryView reports ORA-00942 (table or view does not exist) and
// ORA-01031 (insufficient privileges), which the DBA_* views raise for
// accounts without dictionary access.
func isMissingDictionaryView(err error) bool {
	var oraErr *network.OracleError
	return errors.As(err, &oraErr) && (oraErr.ErrCode == 942 || oraErr.ErrCode == 1031)
}

// oracleSystemSchemas supplements ALL_USERS.ORACLE_MAINTAINED on releases
// that predate the flag.
var oracleSystemSchemas = map[string]struct{}{
	"SYS": {}, "SYSTEM": {}, "XDB": {}, "CTXSYS": {}, "MDSYS": {}, "OUTLN": {},
	"DBSNMP": {}, "APPQOSSYS": {}, "GSMADMIN_INTERNAL": {}, "AUDSYS": {},
	"LBACSYS": {}, "DVSYS": {}, "ORDSYS": {}, "ORDDATA": {}, "WMSYS": {},
	"OJVMSYS": {}, "DBSFWUSER": {}, "REMOTE_SCHEDULER_AGENT": {}, "SYS$UMF": {},
	"ANONYMOUS": {}, "APEX_PUBLIC_USER": {}, "FLOWS_FILES": {}, "OLAPSYS": {},
	"SI_INFORMTN_SCHEMA": {}, "DIP": {}, "ORACLE_OCM": {}, "XS$NULL": {},
}

// isOracleSystemSchema classifies a schema as Oracle-supplied, either by the
// ALL_USERS.ORACLE_MAINTAINED flag or by the known-name list for older
// releases.
func isOracleSystemSchema(owner, maintained string) bool {
	_, known := oracleSystemSchemas[owner]
	return maintained == "Y" || known || owner == "PUBLIC"
}

// qualifiedIn renders owner.name, or just name when it lives in schema.
func qualifiedIn(schema, owner, name string) string {
	if owner == schema {
		return name
	}
	return owner + "." + name
}

const listSchemasSQL = `
SELECT s.owner, NVL(u.oracle_maintained, 'N'),
       CASE WHEN s.owner = SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') THEN 1 ELSE 0 END
FROM (
  SELECT username AS owner FROM all_users
  UNION SELECT SYS_CONTEXT('USERENV', 'CURRENT_SCHEMA') FROM dual
) s
LEFT JOIN all_users u ON u.username = s.owner
ORDER BY s.owner`

// ListSchemas never flags the current schema as system, so a session logged in
// as an Oracle-maintained account (SYSTEM on development images) keeps its
// working schema visible.
func ListSchemas(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listShared(ctx, q, parents, "schemas", listSchemasSQL, nil, func(rows *sql.Rows) (metadata.Child, error) {
		var owner, maintained string
		var current int
		err := rows.Scan(&owner, &maintained, &current)
		return metadata.Child{Kind: "schema", Name: owner, System: current != 1 && isOracleSystemSchema(owner, maintained), Current: current == 1}, err
	})
}

// listGlobalTypesSQL lists the predefined types, which ALL_TYPES reports
// without an owner. Oracle 23ai reports none.
const listGlobalTypesSQL = `
SELECT t.type_name, t.typecode
FROM all_types t
WHERE t.owner IS NULL
ORDER BY t.type_name`

func ListGlobalTypes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listShared(ctx, q, parents, "global types", listGlobalTypesSQL, nil, scanType)
}

const listUsersSQL = `
SELECT u.username, NVL(u.oracle_maintained, 'N'), CASE WHEN u.username = USER THEN 1 ELSE 0 END
FROM all_users u
ORDER BY u.username`

func ListUsers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listShared(ctx, q, parents, "users", listUsersSQL, nil, func(rows *sql.Rows) (metadata.Child, error) {
		var name, maintained string
		var current int
		err := rows.Scan(&name, &maintained, &current)
		return metadata.Child{Kind: "user", Name: name, System: maintained == "Y", Current: current == 1}, err
	})
}

const listRolesSQL = `
SELECT r.role, NVL(r.oracle_maintained, 'N')
FROM dba_roles r
ORDER BY r.role`

// ListRoles lists roles from DBA_ROLES. Accounts without dictionary access
// get an empty listing rather than an error.
func ListRoles(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	out, err := listShared(ctx, q, parents, "roles", listRolesSQL, nil, func(rows *sql.Rows) (metadata.Child, error) {
		var name, maintained string
		err := rows.Scan(&name, &maintained)
		return metadata.Child{Kind: "role", Name: name, System: maintained == "Y"}, err
	})
	if isMissingDictionaryView(err) {
		return newListing(parents), nil
	}
	return out, err
}

const listProfilesSQL = `
SELECT DISTINCT p.profile
FROM dba_profiles p
ORDER BY p.profile`

// ListProfiles lists profiles from DBA_PROFILES. Accounts without dictionary
// access get an empty listing rather than an error.
func ListProfiles(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	out, err := listShared(ctx, q, parents, "profiles", listProfilesSQL, nil, func(rows *sql.Rows) (metadata.Child, error) {
		child := metadata.Child{Kind: "profile"}
		err := rows.Scan(&child.Name)
		return child, err
	})
	if isMissingDictionaryView(err) {
		return newListing(parents), nil
	}
	return out, err
}

// listTablesSQL skips the storage-only tables ALL_TABLES also reports:
// materialized view containers, IOT overflow segments, nested table storage,
// domain index secondaries, and recycle-bin entries.
const listTablesSQL = `
SELECT t.table_name, t.num_rows, t.partitioned, t.temporary
FROM all_tables t
WHERE t.owner = :1 AND t.nested = 'NO' AND t.secondary = 'N' AND t.dropped = 'NO'
  AND (t.iot_type IS NULL OR t.iot_type <> 'IOT_OVERFLOW')
  AND NOT EXISTS (SELECT 1 FROM all_mviews m WHERE m.owner = t.owner AND m.mview_name = t.table_name)
ORDER BY t.table_name`

// internalTablePrefixes name the tables Oracle creates to back queues and
// materialized view logs.
var internalTablePrefixes = []string{"AQ$_", "MLOG$_", "RUPD$_"}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func ListTables(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, "tables", listTablesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var partitioned, temporary sql.NullString
		var rowCount sql.NullInt64
		if err := rows.Scan(&name, &rowCount, &partitioned, &temporary); err != nil {
			return metadata.Child{}, err
		}
		attrs := map[string]any{"partitioned": partitioned.String == "YES", "temporary": temporary.String == "Y"}
		if rowCount.Valid {
			attrs["row_count"] = rowCount.Int64
		}
		return metadata.Child{Kind: "table", Name: name, System: hasAnyPrefix(name, internalTablePrefixes), Attributes: attrs}, nil
	})
}

const listViewsSQL = `
SELECT v.view_name
FROM all_views v
WHERE v.owner = :1
ORDER BY v.view_name`

func ListViews(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, "views", listViewsSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		err := rows.Scan(&name)
		return metadata.Child{Kind: "view", Name: name, System: strings.HasPrefix(name, "AQ$")}, err
	})
}

const listMaterializedViewsSQL = `
SELECT m.mview_name, m.refresh_method, m.staleness
FROM all_mviews m
WHERE m.owner = :1
ORDER BY m.mview_name`

func ListMaterializedViews(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, "materialized views", listMaterializedViewsSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var refresh, staleness sql.NullString
		err := rows.Scan(&name, &refresh, &staleness)
		return metadata.Child{Kind: "materialized_view", Name: name, Attributes: map[string]any{
			"refresh_method": refresh.String, "staleness": staleness.String,
		}}, err
	})
}

// listSchemaIndexesSQL lists every index the schema owns; LOB indexes are
// storage-managed and flagged as system.
const listSchemaIndexesSQL = `
SELECT i.index_name, i.table_owner, i.table_name, i.uniqueness, i.index_type
FROM all_indexes i
WHERE i.owner = :1
ORDER BY i.index_name`

func ListSchemaIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	schema := parents[0].Name("schema")
	return listInSchema(ctx, q, parents, "indexes", listSchemaIndexesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, tableOwner, table string
		var uniqueness, indexType sql.NullString
		if err := rows.Scan(&name, &tableOwner, &table, &uniqueness, &indexType); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "index", Name: name, System: indexType.String == "LOB", Attributes: map[string]any{
			"table": qualifiedIn(schema, tableOwner, table), "unique": uniqueness.String == "UNIQUE", "method": strings.ToLower(indexType.String),
		}}, nil
	})
}

const listSequencesSQL = `
SELECT s.sequence_name
FROM all_sequences s
WHERE s.sequence_owner = :1
ORDER BY s.sequence_name`

// ListSequences flags the ISEQ$$_ sequences that back identity columns as
// system.
func ListSequences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, "sequences", listSequencesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		err := rows.Scan(&name)
		return metadata.Child{Kind: "sequence", Name: name, System: strings.HasPrefix(name, "ISEQ$$_")}, err
	})
}

const listQueuesSQL = `
SELECT q.name, q.queue_table, q.queue_type
FROM all_queues q
WHERE q.owner = :1
ORDER BY q.name`

// ListQueues flags the AQ$_ exception queues Oracle creates per queue table
// as system.
func ListQueues(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, "queues", listQueuesSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, table string
		var queueType sql.NullString
		err := rows.Scan(&name, &table, &queueType)
		return metadata.Child{Kind: "queue", Name: name, System: strings.HasPrefix(name, "AQ$_"), Attributes: map[string]any{
			"queue_table": table, "queue_type": strings.ToLower(queueType.String),
		}}, err
	})
}

const listTypesSQL = `
SELECT t.type_name, t.typecode
FROM all_types t
WHERE t.owner = :1
ORDER BY t.type_name`

func scanType(rows *sql.Rows) (metadata.Child, error) {
	var name string
	var typecode sql.NullString
	err := rows.Scan(&name, &typecode)
	return metadata.Child{Kind: "type", Name: name, System: strings.HasPrefix(name, "SYS_PLSQL_"), Attributes: map[string]any{
		"typecode": typecode.String,
	}}, err
}

func ListTypes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, "types", listTypesSQL, scanType)
}

// listStoredCodeSQL lists one stored PL/SQL object type from ALL_OBJECTS;
// ALL_PROCEDURES would also report package subprograms.
const listStoredCodeSQL = `
SELECT o.object_name, o.status
FROM all_objects o
WHERE o.owner = :1 AND o.object_type = :2
ORDER BY o.object_name`

func listStoredCode(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, objectType, kind string) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, kind+"s", listStoredCodeSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name, status string
		err := rows.Scan(&name, &status)
		return metadata.Child{Kind: kind, Name: name, Attributes: map[string]any{"status": strings.ToLower(status)}}, err
	}, objectType)
}

func ListPackages(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listStoredCode(ctx, q, parents, "PACKAGE", "package")
}

func ListProcedures(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listStoredCode(ctx, q, parents, "PROCEDURE", "procedure")
}

func ListFunctions(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listStoredCode(ctx, q, parents, "FUNCTION", "function")
}

const listSynonymsSQL = `
SELECT s.synonym_name, s.table_owner, s.table_name, s.db_link
FROM all_synonyms s
WHERE s.owner = :1
ORDER BY s.synonym_name`

func ListSynonyms(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, "synonyms", listSynonymsSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var owner, table, link sql.NullString
		if err := rows.Scan(&name, &owner, &table, &link); err != nil {
			return metadata.Child{}, err
		}
		target := table.String
		if owner.String != "" {
			target = owner.String + "." + target
		}
		if link.String != "" {
			target += "@" + link.String
		}
		return metadata.Child{Kind: "synonym", Name: name, Attributes: map[string]any{"target": target}}, nil
	})
}

const listDBLinksSQL = `
SELECT l.db_link, l.username, l.host
FROM all_db_links l
WHERE l.owner = :1
ORDER BY l.db_link`

func ListDBLinks(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, "database links", listDBLinksSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var username, host sql.NullString
		if err := rows.Scan(&name, &username, &host); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "db_link", Name: name, Attributes: map[string]any{"username": username.String, "host": host.String}}, nil
	})
}

// BASE_OBJECT_TYPE is blank-padded on some releases, hence the TRIM.
const listSchemaTriggersSQL = `
SELECT t.trigger_name, t.trigger_type, t.triggering_event, t.status
FROM all_triggers t
WHERE t.owner = :1 AND TRIM(t.base_object_type) IN ('SCHEMA', 'DATABASE')
ORDER BY t.trigger_name`

func ListSchemaTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listInSchema(ctx, q, parents, "schema triggers", listSchemaTriggersSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var timing, event, status sql.NullString
		err := rows.Scan(&name, &timing, &event, &status)
		return metadata.Child{Kind: "trigger", Name: name, Attributes: map[string]any{
			"timing": strings.ToLower(timing.String), "event": strings.ToLower(event.String), "status": strings.ToLower(status.String),
		}}, err
	})
}

const listSchemaTableTriggersSQL = `
SELECT t.trigger_name, t.table_owner, t.table_name, t.trigger_type, t.triggering_event, t.status
FROM all_triggers t
WHERE t.owner = :1 AND TRIM(t.base_object_type) NOT IN ('SCHEMA', 'DATABASE')
ORDER BY t.trigger_name`

func ListSchemaTableTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	if len(parents) == 0 {
		return map[metadata.ScopePath][]metadata.Child{}, nil
	}
	schema := parents[0].Name("schema")
	return listInSchema(ctx, q, parents, "table triggers", listSchemaTableTriggersSQL, func(rows *sql.Rows) (metadata.Child, error) {
		var name string
		var tableOwner, table, timing, event, status sql.NullString
		err := rows.Scan(&name, &tableOwner, &table, &timing, &event, &status)
		return metadata.Child{Kind: "trigger", Name: name, Attributes: map[string]any{
			"table": qualifiedIn(schema, tableOwner.String, table.String), "timing": strings.ToLower(timing.String),
			"event": strings.ToLower(event.String), "status": strings.ToLower(status.String),
		}}, err
	})
}

type relationKey struct{ schema, name string }

// relationFilter renders relation-level parents as a row-constructor list for
// `(owner, name) IN (%s)`, bound from :start, and indexes them back to parent
// paths.
func relationFilter(parents []metadata.ScopePath, start int) (string, []any, map[relationKey]metadata.ScopePath) {
	var sb strings.Builder
	args := make([]any, 0, len(parents)*2)
	index := make(map[relationKey]metadata.ScopePath, len(parents))
	for _, p := range parents {
		last, ok := p.Last()
		if !ok {
			continue
		}
		key := relationKey{schema: p.Name("schema"), name: last.Name}
		if len(args) > 0 {
			sb.WriteString(",")
		}
		n := start + len(args)
		sb.WriteString("(:" + strconv.Itoa(n) + ",:" + strconv.Itoa(n+1) + ")")
		args = append(args, key.schema, key.name)
		index[key] = p
	}
	return sb.String(), args, index
}

// listRelationChildren formats the parent filter into every %[1]s of query
// (each occurrence gets its own binds) and runs it; the first two columns of
// every row are the parent's owner and relation name, scan reads the rest.
func listRelationChildren(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath, label, query string, occurrences int,
	scan func(rows *sql.Rows, key *relationKey) (metadata.Child, error)) (map[metadata.ScopePath][]metadata.Child, error) {
	var filters []any
	var args []any
	var index map[relationKey]metadata.ScopePath
	for range occurrences {
		var filter string
		var occurrenceArgs []any
		filter, occurrenceArgs, index = relationFilter(parents, len(args)+1)
		filters = append(filters, filter)
		args = append(args, occurrenceArgs...)
	}
	if len(args) == 0 {
		return newListing(parents), nil
	}
	return scanListing(ctx, q, parents, label, fmt.Sprintf(query, filters...), args, func(rows *sql.Rows) (metadata.ScopePath, metadata.Child, bool, error) {
		var key relationKey
		child, err := scan(rows, &key)
		if err != nil {
			return "", metadata.Child{}, false, err
		}
		parent, ok := index[key]
		return parent, child, ok, nil
	})
}

// listColumnsSQL aggregates key membership once for the requested relations
// instead of probing the constraint views per column.
const listColumnsSQL = `
WITH keys AS (
  SELECT cc.owner, cc.table_name, cc.column_name,
         MAX(CASE WHEN k.constraint_type = 'P' THEN 1 ELSE 0 END) AS pk,
         MAX(CASE WHEN k.constraint_type = 'R' THEN 1 ELSE 0 END) AS fk
  FROM all_cons_columns cc
  JOIN all_constraints k ON k.owner = cc.owner AND k.constraint_name = cc.constraint_name
  WHERE (cc.owner, cc.table_name) IN (%[1]s) AND k.constraint_type IN ('P', 'R')
  GROUP BY cc.owner, cc.table_name, cc.column_name
)
SELECT c.owner, c.table_name, c.column_name, c.data_type, c.char_length, c.data_precision, c.data_scale,
       c.nullable, c.column_id, NVL(k.pk, 0), NVL(k.fk, 0)
FROM all_tab_columns c
LEFT JOIN keys k ON k.owner = c.owner AND k.table_name = c.table_name AND k.column_name = c.column_name
WHERE (c.owner, c.table_name) IN (%[2]s)
ORDER BY c.owner, c.table_name, c.column_id`

func ListColumns(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "columns", listColumnsSQL, 2, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, nullable string
		var dataType sql.NullString
		var length, precision, scale sql.NullInt64
		var ordinal int64
		var primaryKey, foreignKey int
		if err := rows.Scan(&key.schema, &key.name, &name, &dataType, &length, &precision, &scale, &nullable, &ordinal, &primaryKey, &foreignKey); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "column", Name: name, Attributes: map[string]any{
			"data_type": oracleColumnType(dataType.String, length, precision, scale), "nullable": nullable == "Y", "ordinal": ordinal,
			"primary_key": primaryKey == 1, "foreign_key": foreignKey == 1,
		}}, nil
	})
}

var constraintTypes = map[string]string{
	"P": "primary_key", "U": "unique", "C": "check", "V": "check_option", "O": "read_only",
}

const listConstraintsSQL = `
SELECT k.owner, k.table_name, k.constraint_name, k.constraint_type, k.generated, k.search_condition_vc
FROM all_constraints k
WHERE (k.owner, k.table_name) IN (%s) AND k.constraint_type IN ('P', 'U', 'C', 'V', 'O')
ORDER BY k.owner, k.table_name, k.constraint_name`

// ListConstraints flags the system-named NOT NULL checks Oracle creates for
// column nullability as system.
func ListConstraints(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "constraints", listConstraintsSQL, 1, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, constraintType string
		var generated, condition sql.NullString
		if err := rows.Scan(&key.schema, &key.name, &name, &constraintType, &generated, &condition); err != nil {
			return metadata.Child{}, err
		}
		notNull := constraintType == "C" && generated.String == "GENERATED NAME" && isOracleNotNullCondition(condition.String)
		return metadata.Child{Kind: "constraint", Name: name, System: notNull, Attributes: map[string]any{
			"constraint_type": constraintTypes[constraintType],
		}}, nil
	})
}

// listForeignKeysSQL outer-joins the referenced key so a foreign key into a
// table the account cannot see still lists, with the key's owner only.
const listForeignKeysSQL = `
SELECT k.owner, k.table_name, k.constraint_name, k.r_owner, r.table_name, k.delete_rule
FROM all_constraints k
LEFT JOIN all_constraints r ON r.owner = k.r_owner AND r.constraint_name = k.r_constraint_name
WHERE (k.owner, k.table_name) IN (%s) AND k.constraint_type = 'R'
ORDER BY k.owner, k.table_name, k.constraint_name`

func ListForeignKeys(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "foreign keys", listForeignKeysSQL, 1, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name string
		var refOwner, refTable, deleteRule sql.NullString
		if err := rows.Scan(&key.schema, &key.name, &name, &refOwner, &refTable, &deleteRule); err != nil {
			return metadata.Child{}, err
		}
		target := refOwner.String
		if refTable.Valid {
			target += "." + refTable.String
		}
		return metadata.Child{Kind: "foreign_key", Name: name, Attributes: map[string]any{
			"referenced_table": target, "delete_rule": strings.ToLower(deleteRule.String),
		}}, nil
	})
}

// listReferencesSQL names a referencing key by owner when it lives outside the
// referenced table's schema, since constraint names are unique per owner only.
const listReferencesSQL = `
SELECT r.owner, r.table_name, k.owner, k.constraint_name, k.owner || '.' || k.table_name
FROM all_constraints r
JOIN all_constraints k ON k.r_owner = r.owner AND k.r_constraint_name = r.constraint_name AND k.constraint_type = 'R'
WHERE (r.owner, r.table_name) IN (%s) AND r.constraint_type IN ('P', 'U')
ORDER BY r.owner, r.table_name, k.owner, k.constraint_name`

func ListReferences(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "references", listReferencesSQL, 1, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var owner, name, source string
		if err := rows.Scan(&key.schema, &key.name, &owner, &name, &source); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "reference", Name: qualifiedIn(key.schema, owner, name), Attributes: map[string]any{"source_table": source}}, nil
	})
}

const listTableTriggersSQL = `
SELECT t.table_owner, t.table_name, t.trigger_name, t.owner, t.trigger_type, t.triggering_event, t.status
FROM all_triggers t
WHERE (t.table_owner, t.table_name) IN (%s) AND TRIM(t.base_object_type) NOT IN ('SCHEMA', 'DATABASE')
ORDER BY t.table_owner, t.table_name, t.owner, t.trigger_name`

func ListTableTriggers(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "table triggers", listTableTriggersSQL, 1, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, owner string
		var timing, event, status sql.NullString
		if err := rows.Scan(&key.schema, &key.name, &name, &owner, &timing, &event, &status); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "trigger", Name: qualifiedIn(key.schema, owner, name), Attributes: map[string]any{
			"owner": owner, "timing": strings.ToLower(timing.String), "event": strings.ToLower(event.String), "status": strings.ToLower(status.String),
		}}, nil
	})
}

const listTableIndexesSQL = `
SELECT i.table_owner, i.table_name, i.index_name, i.owner, i.uniqueness, i.index_type,
       CASE WHEN EXISTS (
         SELECT 1 FROM all_constraints k
         WHERE k.owner = i.table_owner AND k.table_name = i.table_name AND k.constraint_type = 'P'
           AND k.index_owner = i.owner AND k.index_name = i.index_name
       ) THEN 1 ELSE 0 END
FROM all_indexes i
WHERE (i.table_owner, i.table_name) IN (%s)
ORDER BY i.table_owner, i.table_name, i.owner, i.index_name`

func ListTableIndexes(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	return listRelationChildren(ctx, q, parents, "table indexes", listTableIndexesSQL, 1, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, owner string
		var uniqueness, indexType sql.NullString
		var primary int
		if err := rows.Scan(&key.schema, &key.name, &name, &owner, &uniqueness, &indexType, &primary); err != nil {
			return metadata.Child{}, err
		}
		return metadata.Child{Kind: "index", Name: qualifiedIn(key.schema, owner, name), System: indexType.String == "LOB", Attributes: map[string]any{
			"owner": owner, "unique": uniqueness.String == "UNIQUE", "primary": primary == 1, "method": strings.ToLower(indexType.String),
		}}, nil
	})
}

// listDependenciesSQL lists both directions of ALL_DEPENDENCIES for a
// relation: the objects that depend on it and the objects it depends on. The
// type filters keep same-named objects in other namespaces (triggers,
// package bodies) out of the parent match, and self rows are dropped because a
// materialized view depends on its own container table. Bodies fold into their
// spec so a package or type lists once; remote objects have no owner and carry
// their database link instead.
const listDependenciesSQL = `
SELECT d.referenced_owner, d.referenced_name, d.owner || '.' || d.name,
       DECODE(d.type, 'PACKAGE BODY', 'PACKAGE', 'TYPE BODY', 'TYPE', d.type), 'dependent'
FROM all_dependencies d
WHERE (d.referenced_owner, d.referenced_name) IN (%[1]s)
  AND d.referenced_type IN ('TABLE', 'VIEW', 'MATERIALIZED VIEW')
  AND NOT (d.owner = d.referenced_owner AND d.name = d.referenced_name)
UNION
SELECT d.owner, d.name,
       NVL2(d.referenced_owner, d.referenced_owner || '.', '') || d.referenced_name
         || NVL2(d.referenced_link_name, '@' || d.referenced_link_name, ''),
       DECODE(d.referenced_type, 'PACKAGE BODY', 'PACKAGE', 'TYPE BODY', 'TYPE', d.referenced_type), 'dependency'
FROM all_dependencies d
WHERE (d.owner, d.name) IN (%[2]s)
  AND d.type IN ('TABLE', 'VIEW', 'MATERIALIZED VIEW')
  AND d.referenced_type <> 'NON-EXISTENT'
  AND NOT (d.owner = d.referenced_owner AND d.name = d.referenced_name)
ORDER BY 1, 2, 3, 5`

// ListDependencies flags objects owned by SYS or PUBLIC (DUAL, STANDARD, and
// their public synonyms) as system.
func ListDependencies(ctx context.Context, q metadata.Querier, parents []metadata.ScopePath) (map[metadata.ScopePath][]metadata.Child, error) {
	out, err := listRelationChildren(ctx, q, parents, "dependencies", listDependenciesSQL, 2, func(rows *sql.Rows, key *relationKey) (metadata.Child, error) {
		var name, objectType, direction string
		if err := rows.Scan(&key.schema, &key.name, &name, &objectType, &direction); err != nil {
			return metadata.Child{}, err
		}
		system := strings.HasPrefix(name, "SYS.") || strings.HasPrefix(name, "PUBLIC.")
		return metadata.Child{Kind: "dependency", Name: name, System: system, Attributes: map[string]any{
			"object_kind": strings.ReplaceAll(strings.ToLower(objectType), " ", "_"), "direction": direction,
		}}, nil
	})
	for parent, children := range out {
		out[parent] = mergeDependencies(children)
	}
	return out, err
}

// mergeDependencies collapses rows that name the same object, which happens
// when it both depends on the relation and is depended on by it (a view
// calling a function that selects from the view), or when same-named objects
// live in different namespaces.
func mergeDependencies(children []metadata.Child) []metadata.Child {
	merged := children[:0]
	seen := make(map[string]int, len(children))
	for _, c := range children {
		i, ok := seen[c.Name]
		if !ok {
			seen[c.Name] = len(merged)
			merged = append(merged, c)
			continue
		}
		if merged[i].Attributes["direction"] != c.Attributes["direction"] {
			merged[i].Attributes["direction"] = "both"
		}
	}
	return merged
}
