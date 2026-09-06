package web

import (
	_ "github.com/sqlwarden/internal/engine/engines/mariadb"
	_ "github.com/sqlwarden/internal/engine/engines/mysql"
	_ "github.com/sqlwarden/internal/engine/engines/neon"
	_ "github.com/sqlwarden/internal/engine/engines/oracle"
	_ "github.com/sqlwarden/internal/engine/engines/postgres"
	_ "github.com/sqlwarden/internal/engine/engines/sqlite"
	_ "github.com/sqlwarden/internal/engine/engines/supabase"
)
