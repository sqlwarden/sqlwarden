package schema

import (
	"sort"
	"strings"
)

// MetadataSet is one coherent, versioned schema metadata input. Catalog remains
// the lightweight object directory and Objects remains the independently
// inspectable detail tier; grouping them makes their shared generation
// explicit at consumer boundaries.
//
// Relationships is optional because consumers such as SQL completion do not
// need graph topology.
type MetadataSet struct {
	Catalog       *Catalog
	Objects       []Object
	Relationships []*RelationshipGraph
	Version       string
}

// Index is an immutable, prepared lookup over canonical schema metadata. It is
// safe to share between concurrent readers. It neither inspects a live database
// nor persists metadata; callers decide whether MetadataSet came from a
// persistent snapshot or an ephemeral inspection.
type Index struct {
	catalog       *Catalog
	version       string
	refs          map[ObjectRef]struct{}
	foldedRefs    map[string]ObjectRef
	objects       map[ObjectRef]Object
	foldedObjects map[string]ObjectRef
	refsByScope   map[string][]ObjectRef
	relationships map[string][]Relationship
	outgoing      map[ObjectRef][]Relationship
	incoming      map[ObjectRef][]Relationship
}

// NewIndex prepares metadata for repeated object and relationship lookup.
func NewIndex(metadata MetadataSet) *Index {
	index := &Index{
		version:       metadata.Version,
		refs:          make(map[ObjectRef]struct{}),
		foldedRefs:    make(map[string]ObjectRef),
		objects:       make(map[ObjectRef]Object, len(metadata.Objects)),
		foldedObjects: make(map[string]ObjectRef, len(metadata.Objects)),
		refsByScope:   make(map[string][]ObjectRef),
		relationships: make(map[string][]Relationship),
		outgoing:      make(map[ObjectRef][]Relationship),
		incoming:      make(map[ObjectRef][]Relationship),
	}
	if metadata.Catalog != nil {
		catalog := cloneCatalog(*metadata.Catalog)
		index.catalog = &catalog
		for _, namespace := range catalog.Namespaces {
			for _, group := range namespace.Groups {
				for _, ref := range group.Objects {
					index.addRef(ref)
				}
			}
		}
	}
	for _, raw := range metadata.Objects {
		object := cloneObject(raw)
		index.addRef(object.Ref)
		index.objects[object.Ref] = object
		index.foldedObjects[foldedRefKey(object.Ref)] = object.Ref
	}
	for _, graph := range metadata.Relationships {
		if graph == nil {
			continue
		}
		namespace := strings.ToLower(graph.Namespace)
		for _, raw := range graph.Relationships {
			relationship := cloneRelationship(raw)
			index.relationships[namespace] = append(index.relationships[namespace], relationship)
			index.addRef(relationship.Source)
			index.addRef(relationship.References)
			source := index.canonicalRef(relationship.Source)
			target := index.canonicalRef(relationship.References)
			index.outgoing[source] = append(index.outgoing[source], relationship)
			index.incoming[target] = append(index.incoming[target], relationship)
		}
	}
	for scope := range index.refsByScope {
		sort.Slice(index.refsByScope[scope], func(i, j int) bool {
			left, right := index.refsByScope[scope][i], index.refsByScope[scope][j]
			if left.Name != right.Name {
				return left.Name < right.Name
			}
			return left.Kind < right.Kind
		})
	}
	return index
}

// Version identifies the metadata generation used to build the index.
func (i *Index) Version() string {
	if i == nil {
		return ""
	}
	return i.version
}

// Catalog returns an isolated copy of the lightweight catalog.
func (i *Index) Catalog() (*Catalog, bool) {
	if i == nil || i.catalog == nil {
		return nil, false
	}
	catalog := cloneCatalog(*i.catalog)
	return &catalog, true
}

func (i *Index) Database() string {
	if i == nil || i.catalog == nil {
		return ""
	}
	return i.catalog.Database
}

func (i *Index) DefaultNamespace() string {
	if i == nil || i.catalog == nil {
		return ""
	}
	return i.catalog.DefaultNamespace
}

// NamespaceNames returns sorted namespace names from the catalog.
func (i *Index) NamespaceNames() []string {
	if i == nil || i.catalog == nil {
		return nil
	}
	result := make([]string, 0, len(i.catalog.Namespaces))
	for _, namespace := range i.catalog.Namespaces {
		result = append(result, namespace.Name)
	}
	sort.Strings(result)
	return result
}

// FindRef resolves an exact qualified reference first, then
// case-insensitively. Unlike FindObject, it also finds catalog entries whose
// detailed metadata has not been inspected yet.
func (i *Index) FindRef(namespace, kind, name string) (ObjectRef, bool) {
	if i == nil {
		return ObjectRef{}, false
	}
	if kind != "" {
		ref := ObjectRef{Namespace: namespace, Kind: kind, Name: name}
		if _, ok := i.refs[ref]; ok {
			return ref, true
		}
		canonical, ok := i.foldedRefs[foldedRefKey(ref)]
		return canonical, ok
	}
	for _, ref := range i.refsByScope[foldedScopeKey(namespace, "")] {
		if ref.Name == name {
			return ref, true
		}
	}
	for _, ref := range i.refsByScope[foldedScopeKey(namespace, "")] {
		if strings.EqualFold(ref.Name, name) {
			return ref, true
		}
	}
	return ObjectRef{}, false
}

// ObjectRefs returns lightweight catalog references in a namespace, optionally
// restricted to one kind. Detailed inspection is not required.
func (i *Index) ObjectRefs(namespace, kind string) []ObjectRef {
	if i == nil {
		return nil
	}
	return append([]ObjectRef(nil), i.refsByScope[foldedScopeKey(namespace, kind)]...)
}

// Object resolves an exact qualified object reference.
func (i *Index) Object(ref ObjectRef) (Object, bool) {
	if i == nil {
		return Object{}, false
	}
	object, ok := i.objects[ref]
	if !ok {
		return Object{}, false
	}
	return cloneObject(object), true
}

// FindObject resolves a qualified name exactly first, then case-insensitively.
// An empty kind matches any kind; when names collide the catalog ordering is
// used, making the result deterministic.
func (i *Index) FindObject(namespace, kind, name string) (Object, bool) {
	if i == nil {
		return Object{}, false
	}
	if kind != "" {
		if object, ok := i.Object(ObjectRef{Namespace: namespace, Kind: kind, Name: name}); ok {
			return object, true
		}
		if ref, ok := i.foldedObjects[foldedRefKey(ObjectRef{Namespace: namespace, Kind: kind, Name: name})]; ok {
			return i.Object(ref)
		}
		return Object{}, false
	}
	for _, ref := range i.refsByScope[foldedScopeKey(namespace, "")] {
		if ref.Name == name {
			if object, ok := i.Object(ref); ok {
				return object, true
			}
		}
	}
	for _, ref := range i.refsByScope[foldedScopeKey(namespace, "")] {
		if strings.EqualFold(ref.Name, name) {
			if object, ok := i.Object(ref); ok {
				return object, true
			}
		}
	}
	return Object{}, false
}

// Objects returns objects in a namespace, optionally restricted to one kind.
func (i *Index) Objects(namespace, kind string) []Object {
	if i == nil {
		return nil
	}
	refs := i.refsByScope[foldedScopeKey(namespace, kind)]
	result := make([]Object, 0, len(refs))
	for _, ref := range refs {
		result = append(result, cloneObject(i.objects[ref]))
	}
	return result
}

// Relationships returns the immutable FK topology for a namespace.
func (i *Index) Relationships(namespace string) []Relationship {
	if i == nil {
		return nil
	}
	return cloneRelationships(i.relationships[strings.ToLower(namespace)])
}

func (i *Index) Outgoing(ref ObjectRef) []Relationship {
	if i == nil {
		return nil
	}
	return cloneRelationships(i.outgoing[i.canonicalRef(ref)])
}

func (i *Index) Incoming(ref ObjectRef) []Relationship {
	if i == nil {
		return nil
	}
	return cloneRelationships(i.incoming[i.canonicalRef(ref)])
}

// Neighbors returns unique objects connected to ref by an incoming or outgoing
// relationship. Missing object details are omitted.
func (i *Index) Neighbors(ref ObjectRef) []Object {
	refs := i.NeighborRefs(ref)
	result := make([]Object, 0, len(refs))
	for _, candidate := range refs {
		if object, ok := i.Object(candidate); ok {
			result = append(result, object)
		}
	}
	return result
}

// NeighborRefs returns unique catalog references connected to ref in stable
// order. It works before object details have been inspected, which makes it
// suitable for progressively loading ER diagrams.
func (i *Index) NeighborRefs(ref ObjectRef) []ObjectRef {
	if i == nil {
		return nil
	}
	ref = i.canonicalRef(ref)
	seen := make(map[ObjectRef]struct{})
	for _, relationship := range i.outgoing[ref] {
		seen[i.canonicalRef(relationship.References)] = struct{}{}
	}
	for _, relationship := range i.incoming[ref] {
		seen[i.canonicalRef(relationship.Source)] = struct{}{}
	}
	refs := make([]ObjectRef, 0, len(seen))
	for candidate := range seen {
		if candidate != ref {
			refs = append(refs, candidate)
		}
	}
	sort.Slice(refs, func(a, b int) bool {
		if refs[a].Namespace != refs[b].Namespace {
			return refs[a].Namespace < refs[b].Namespace
		}
		if refs[a].Name != refs[b].Name {
			return refs[a].Name < refs[b].Name
		}
		return refs[a].Kind < refs[b].Kind
	})
	return refs
}

func (i *Index) canonicalRef(ref ObjectRef) ObjectRef {
	if _, ok := i.refs[ref]; ok {
		return ref
	}
	if canonical, ok := i.foldedRefs[foldedRefKey(ref)]; ok {
		return canonical
	}
	return ref
}

func (i *Index) addRef(ref ObjectRef) {
	if _, exists := i.refs[ref]; exists {
		return
	}
	i.refs[ref] = struct{}{}
	i.foldedRefs[foldedRefKey(ref)] = ref
	scope := foldedScopeKey(ref.Namespace, ref.Kind)
	i.refsByScope[scope] = append(i.refsByScope[scope], ref)
	i.refsByScope[foldedScopeKey(ref.Namespace, "")] = append(
		i.refsByScope[foldedScopeKey(ref.Namespace, "")],
		ref,
	)
}

func foldedScopeKey(namespace, kind string) string {
	return strings.ToLower(namespace) + "\x00" + strings.ToLower(kind)
}

func foldedRefKey(ref ObjectRef) string {
	return foldedScopeKey(ref.Namespace, ref.Kind) + "\x00" + strings.ToLower(ref.Name)
}

func cloneCatalog(catalog Catalog) Catalog {
	result := catalog
	result.Namespaces = make([]NamespaceCatalog, len(catalog.Namespaces))
	for i, namespace := range catalog.Namespaces {
		result.Namespaces[i] = namespace
		result.Namespaces[i].Groups = make([]ObjectGroupCatalog, len(namespace.Groups))
		for j, group := range namespace.Groups {
			result.Namespaces[i].Groups[j] = group
			result.Namespaces[i].Groups[j].Objects = append([]ObjectRef(nil), group.Objects...)
		}
	}
	return result
}

func cloneObject(object Object) Object {
	result := object
	if object.Relational != nil {
		relational := *object.Relational
		relational.Columns = append([]Column(nil), object.Relational.Columns...)
		for i := range relational.Columns {
			relational.Columns[i].Attributes = cloneAttributes(relational.Columns[i].Attributes)
		}
		relational.PrimaryKey = append([]string(nil), object.Relational.PrimaryKey...)
		relational.ForeignKeys = append([]ForeignKey(nil), object.Relational.ForeignKeys...)
		for i := range relational.ForeignKeys {
			relational.ForeignKeys[i].Columns = append([]string(nil), relational.ForeignKeys[i].Columns...)
			relational.ForeignKeys[i].ReferencedColumns = append([]string(nil), relational.ForeignKeys[i].ReferencedColumns...)
			relational.ForeignKeys[i].Attributes = cloneAttributes(relational.ForeignKeys[i].Attributes)
		}
		relational.Indexes = append([]SecondaryIndex(nil), object.Relational.Indexes...)
		for i := range relational.Indexes {
			relational.Indexes[i].Columns = append([]string(nil), relational.Indexes[i].Columns...)
			relational.Indexes[i].Attributes = cloneAttributes(relational.Indexes[i].Attributes)
		}
		result.Relational = &relational
	}
	result.Descriptors = append([]Descriptor(nil), object.Descriptors...)
	for i := range result.Descriptors {
		result.Descriptors[i].Fields = append([]Field(nil), result.Descriptors[i].Fields...)
		if result.Descriptors[i].Rows != nil {
			rows := *result.Descriptors[i].Rows
			rows.Columns = append([]string(nil), rows.Columns...)
			rows.Rows = make([][]string, len(rows.Rows))
			for j := range rows.Rows {
				rows.Rows[j] = append([]string(nil), rows.Rows[j]...)
			}
			result.Descriptors[i].Rows = &rows
		}
		if result.Descriptors[i].Source != nil {
			source := *result.Descriptors[i].Source
			result.Descriptors[i].Source = &source
		}
	}
	result.Attributes = cloneAttributes(object.Attributes)
	return result
}

func cloneAttributes(attributes map[string]any) map[string]any {
	if attributes == nil {
		return nil
	}
	result := make(map[string]any, len(attributes))
	for key, value := range attributes {
		result[key] = cloneAttributeValue(value)
	}
	return result
}

func cloneAttributeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAttributes(typed)
	case []any:
		result := make([]any, len(typed))
		for i, item := range typed {
			result[i] = cloneAttributeValue(item)
		}
		return result
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func cloneRelationship(relationship Relationship) Relationship {
	result := relationship
	result.Columns = append([]string(nil), relationship.Columns...)
	result.ReferencedColumns = append([]string(nil), relationship.ReferencedColumns...)
	return result
}

func cloneRelationships(relationships []Relationship) []Relationship {
	result := make([]Relationship, len(relationships))
	for i, relationship := range relationships {
		result[i] = cloneRelationship(relationship)
	}
	return result
}
