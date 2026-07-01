/** dataTransfer MIME for dragging a schema object ref onto the diagram canvas.
 *  The schema tree sets it (JSON-encoded ObjectRef); the canvas reads it on drop
 *  to add that table to the working set. Distinct from the SQL-identifier drag. */
export const OBJECT_REF_DND_MIME = 'application/x-sqlwarden-object-ref'
