// Package build assembles the two-tier metadata models from the flat rows that a
// driver's inspection queries return: a DirectoryBuilder for the cheap listing,
// and a RelationalBuilder for typed object detail with qualified foreign keys.
// Neither is safe for concurrent use; each inspection uses its own builder.
package build
