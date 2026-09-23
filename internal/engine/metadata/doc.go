// Package metadata is the engine metadata domain. It defines the
// SchemaInspector capability an engine implements to report its objects in two
// tiers (a cheap Directory listing and on-demand Object detail), the optional
// definition, relationship, and scope-discovery inspectors, the data model those
// reports use (objects, columns, keys, descriptors), and the static SchemaSpec
// describing which object kinds an engine exposes.
package metadata
