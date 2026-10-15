// Package exportjob owns query export use cases: validating and authorizing an
// export request, queueing it as a background job, streaming it synchronously
// to a caller, and running the queued job.
//
// Authorization is the service's responsibility. Request and Download require
// a read or execute permission on the connection before anything else happens,
// and RunJob re-resolves the organization, workspace, connection, and
// permission as the job's owner at run time, so access revoked between queueing
// and running stops the export.
package exportjob
