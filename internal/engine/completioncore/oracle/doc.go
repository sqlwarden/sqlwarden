// Package oracle adapts Bytebase/Omni's Oracle parser-native completion to
// SQLWarden's dialect-neutral completion boundary. Omni exposes no Oracle
// catalog or high-level completion, so grammar candidates come from
// parser.CollectCompletion and every name is resolved from SQLWarden's own
// metadata index via completioncore.MetadataResolver.
package oracle
