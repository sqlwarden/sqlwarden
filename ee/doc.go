// Package ee is the Enterprise Edition composition root. It implements the
// edition.Edition seam with modules gated on a configured license source,
// identity and policy decorators, and the enterprise audit pipeline. Core
// packages never import this tree, and no shipped entrypoint composes it yet.
package ee
