// Package web is SQLWarden's HTTP transport: routes, middleware, handlers,
// request/response mapping, and embedded frontend serving. It also adapts the
// already-built application services into the process kinds ("all", "api",
// and "connector") that internal/app starts and stops.
//
// The package does not load configuration or construct the service graph;
// internal/config and internal/app own those. Target database work goes
// through the internal/execution runtime rather than live connection or
// credential packages.
package web
