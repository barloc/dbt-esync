package parse

import "strings"

// Options controls table-ref normalization and schema validation.
type Options struct {
	// ExcludedCatalogs: for 3-part refs, drop the catalog segment when it matches
	// (case-insensitive) and keep schema.table.
	ExcludedCatalogs []string
	// SchemaAllowlist: when non-empty, schema must be in this list (case-insensitive).
	// When empty, soft structural heuristics apply (see validTableRef).
	SchemaAllowlist []string
}

// refFilter is the resolved lookup form of Options used while normalizing refs.
type refFilter struct {
	excluded map[string]struct{}
	allow    map[string]struct{}
}

func (o Options) filter() refFilter {
	return refFilter{
		excluded: lowerSet(o.ExcludedCatalogs),
		allow:    lowerSet(o.SchemaAllowlist),
	}
}

func lowerSet(items []string) map[string]struct{} {
	if len(items) == 0 {
		return nil
	}
	m := make(map[string]struct{}, len(items))
	for _, s := range items {
		m[strings.ToLower(s)] = struct{}{}
	}
	return m
}
