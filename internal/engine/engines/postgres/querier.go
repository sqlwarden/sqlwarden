package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/sqlwarden/internal/engine/metadata"
)

const (
	databasePoolMaxConns    = 2
	databasePoolIdleTimeout = time.Minute
)

// databasePools holds catalog-only pools for databases other than the one the
// session is connected to. Postgres cannot query across databases, so each
// expanded database gets a small clone of the session's connection config.
type databasePools struct {
	mu     sync.Mutex
	pools  map[string]*sql.DB
	closed bool
}

func (p *databasePools) close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	var errs []error
	for name, db := range p.pools {
		errs = append(errs, db.Close())
		delete(p.pools, name)
	}
	return errors.Join(errs...)
}

func (d *Driver) Querier(ctx context.Context, database string) (metadata.Querier, error) {
	return d.databaseFor(ctx, database)
}

// databaseFor returns the primary pool for "" or the connected database, and a
// lazily created clone pool (same TLS and SSH dialer) for any other database.
func (d *Driver) databaseFor(ctx context.Context, database string) (*sql.DB, error) {
	if database == "" || d.config == nil || database == d.config.Database {
		return d.db, nil
	}
	d.pools.mu.Lock()
	if db, ok := d.pools.pools[database]; ok {
		d.pools.mu.Unlock()
		return db, nil
	}
	d.pools.mu.Unlock()

	// Ping outside the lock so one unreachable database cannot stall
	// expansions of every other database on this session.
	config := d.config.Copy()
	config.Database = database
	delete(config.RuntimeParams, "search_path")
	db := stdlib.OpenDB(*config)
	db.SetMaxOpenConns(databasePoolMaxConns)
	db.SetConnMaxIdleTime(databasePoolIdleTimeout)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("postgres: connect to database: %w", err)
	}

	d.pools.mu.Lock()
	defer d.pools.mu.Unlock()
	if d.pools.closed {
		_ = db.Close()
		return nil, errors.New("postgres: session closed")
	}
	if existing, ok := d.pools.pools[database]; ok {
		_ = db.Close()
		return existing, nil
	}
	d.pools.pools[database] = db
	return db, nil
}

// withDatabase prefixes scope with the database segment when it has none.
func withDatabase(scope metadata.ScopePath, database string) metadata.ScopePath {
	if database == "" || scope.Name("database") != "" {
		return scope
	}
	segments, err := scope.Segments()
	if err != nil {
		return scope
	}
	return metadata.NewScopePath(append([]metadata.ScopeSegment{{Kind: "database", Name: database}}, segments...)...)
}

func qualifyObjects(objs []metadata.Object, database string) {
	for i := range objs {
		objs[i].Ref.Scope = withDatabase(objs[i].Ref.Scope, database)
		if objs[i].Relational == nil {
			continue
		}
		for j := range objs[i].Relational.ForeignKeys {
			fk := &objs[i].Relational.ForeignKeys[j]
			fk.References.Scope = withDatabase(fk.References.Scope, database)
		}
	}
}

func groupRefsByDatabase(refs []metadata.ObjectRef) map[string][]metadata.ObjectRef {
	groups := map[string][]metadata.ObjectRef{}
	for _, ref := range refs {
		name := ref.Scope.Name("database")
		groups[name] = append(groups[name], ref)
	}
	return groups
}
