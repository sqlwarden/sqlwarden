package schema

import "github.com/sqlwarden/internal/engine/metadata"

func (s *Service) CachedScopeDirectory(connID string, scope metadata.ScopePath) (*metadata.Directory, bool) {
	data, ok := s.cache.Get(directoryKey(connID) + sep + string(scope))
	if !ok {
		return nil, false
	}
	var directory metadata.Directory
	if gunzipJSON(data, &directory) != nil {
		return nil, false
	}
	return &directory, true
}

// WithCachedScopes overlays scopes expanded since a snapshot was built. The
// snapshot and cached directory remain immutable. Its timestamp changes when
// an overlay changes, so prepared completion indexes cannot reuse stale names.
func (s *Service) WithCachedScopes(connID string, directory *metadata.Directory) *metadata.Directory {
	if directory == nil {
		return nil
	}
	result := *directory
	var merge func([]metadata.ScopeNode) []metadata.ScopeNode
	merge = func(nodes []metadata.ScopeNode) []metadata.ScopeNode {
		out := append([]metadata.ScopeNode(nil), nodes...)
		for i, node := range out {
			if node.Lazy {
				if cached, ok := s.CachedScopeDirectory(connID, node.Path); ok {
					for _, loaded := range cached.ScopeNodes() {
						if loaded.Path == node.Path {
							loaded.System = node.System
							out[i] = loaded
							if cached.GeneratedAt.After(result.GeneratedAt) {
								result.GeneratedAt = cached.GeneratedAt
							}
							break
						}
					}
				}
			}
			out[i].Children = merge(out[i].Children)
		}
		return out
	}
	result.Roots = merge(directory.Roots)
	return &result
}

// CachedObjects augments persistent metadata with details fetched on demand.
// It never opens a target connection during a completion request.
func (s *Service) CachedObjects(connID string, refs []metadata.ObjectRef) []metadata.Object {
	var result []metadata.Object
	for _, ref := range refs {
		if data, ok := s.cache.Get(objectKey(connID, ref)); ok {
			var object metadata.Object
			if gunzipJSON(data, &object) == nil {
				result = append(result, object)
			}
		}
	}
	return result
}
