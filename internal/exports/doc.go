// Package exports streams query results into downloadable artifacts.
//
// Exports are intentionally separate from interactive query responses: they
// stream rows through engine cursor capabilities and write formatted bytes
// without retaining the full result set in memory.
package exports
