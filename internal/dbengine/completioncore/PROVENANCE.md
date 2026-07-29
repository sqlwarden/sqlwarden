# Completion provenance

The PostgreSQL and MySQL completion behavior in this directory is adapted from
Bytebase's MIT-licensed parser completion implementation:

- Source repository: `https://github.com/bytebase/bytebase`
- Source commit: `83bd74741ba7842d1a7d393d7ac44893462b9cb1`
- Original paths:
  - `backend/plugin/parser/base/complete.go`
  - `backend/plugin/parser/pg/completion.go`
  - `backend/plugin/parser/pg/builtin_functions.go`
  - `backend/plugin/parser/mysql/completion.go`

The implementation is adapted to use SQLWarden's immutable schema snapshots
and completion API. Bytebase protobuf, store, query-span, and transport
dependencies are deliberately not included.
