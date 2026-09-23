// Package classifier defines the SQL classification capability: determining the
// coarse class (DQL/DML/DDL) of a statement so the runtime can authorize it
// against the right connection permission. An engine provides classification by
// implementing Classifier; it is stateless and never touches a live connection.
package classifier
