// Package completioncore contains SQLWarden's dialect-neutral completion
// boundary. The dialect implementations are adapted from Bytebase's MIT
// licensed completion implementation, while metadata remains owned by
// SQLWarden.
//
// Keeping parser candidates and catalog resolution behind this package is
// intentional: as Omni grows complete semantic resolvers, a dialect can
// delegate more work to Omni without changing the engine or HTTP contracts.
package completioncore
