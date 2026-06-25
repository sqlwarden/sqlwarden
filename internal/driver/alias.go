package driver

// NormalizeName returns the canonical engine name for a user-facing driver name
// or known alias.
func NormalizeName(name string) string {
	switch name {
	case "postgresql":
		return "postgres"
	case "sqlite3":
		return "sqlite"
	case "mariadb":
		return "mysql"
	default:
		return name
	}
}
