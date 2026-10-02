package schema

import (
	"context"
	"log/slog"
	"sync"
	"time"

	metadata "github.com/sqlwarden/internal/engine/metadata"
)

const (
	completionLoadCap     = 8
	completionLoadTimeout = 15 * time.Second
)

// EnsureReport counts what EnsureForCompletion did with its demands. A load
// that was started or joined but outlived ctx counts as neither Completed nor
// Failed.
type EnsureReport struct {
	Started   int
	Joined    int
	Cached    int
	Completed int
	Failed    int
	Capped    int
}

// EnsureForCompletion lists the demanded folders through the shared listing
// path. It is the only completion path that queries a target and requires a
// live session. It returns when every load finished or ctx ended; loads
// already started keep running detached and still populate the cache. Load
// failures are logged and counted, never returned.
func (n *Navigator) EnsureForCompletion(ctx context.Context, conn Connection, tree metadata.Tree, live metadata.SchemaInspector, demands []metadata.Demand) EnsureReport {
	var report EnsureReport
	if live == nil {
		return report
	}
	unique := make([]metadata.Demand, 0, len(demands))
	seen := map[metadata.Demand]bool{}
	for _, demand := range demands {
		if !seen[demand] {
			seen[demand] = true
			unique = append(unique, demand)
		}
	}
	if len(unique) > completionLoadCap {
		report.Capped = len(unique) - completionLoadCap
		unique = unique[:completionLoadCap]
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, demand := range unique {
		if ctx.Err() != nil {
			break
		}
		parentKind := tree.NodeKindOf(demand.Parent)
		folder, ok := tree.Folder(parentKind, demand.Folder)
		if !ok {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, origin, err := n.cachedOrLoad(ctx, conn, tree, live, demand.Parent, folder, completionLoadTimeout)
			mu.Lock()
			defer mu.Unlock()
			switch origin {
			case originCache:
				report.Cached++
			case originStarted:
				report.Started++
			case originJoined:
				report.Joined++
			}
			switch {
			case err == nil:
				if origin != originCache {
					report.Completed++
				}
			case ctx.Err() != nil:
				// A started load keeps running detached; this request just stops observing it.
			default:
				report.Failed++
				n.logger.Debug("completion metadata load failed",
					slog.Int64("connection_id", conn.ID),
					slog.String("folder", demand.Folder),
					slog.String("parent_kind", parentKind))
			}
		}()
	}
	wg.Wait()
	return report
}
