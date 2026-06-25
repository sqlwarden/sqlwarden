package web

import (
	_ "github.com/sqlwarden/internal/dbengine/engines/mysql"
	_ "github.com/sqlwarden/internal/dbengine/engines/postgres"
	_ "github.com/sqlwarden/internal/dbengine/engines/sqlite"
)
