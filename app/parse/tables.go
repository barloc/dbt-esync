package parse

import (
	"regexp"
	"strings"
	"unicode"
)

// Soft schema names accepted when SchemaAllowlist is empty (generic only).
var softSchemas = map[string]struct{}{
	"main":  {},
	"marts": {},
	"core":  {},
}

var (
	// Go RE2 has no lookahead; import/extract/trailing-dot filters are applied in code.
	tableRE   = regexp.MustCompile(`(?i)(?:FROM|JOIN)\s+((?:"[\w-]+"|[\w-]+)(?:\.(?:"[\w-]+"|[\w-]+)){1,2})`)
	inlineRE  = regexp.MustCompile(`(?is)LOAD\s+\*\s+INLINE\s+\[(.*?)\];`)
	fromVarRE = regexp.MustCompile(`(?i)(?:FROM|JOIN)\s+\$\((\w+)\)`)
	cellSplit = regexp.MustCompile(`[,\t]+`)
	importRE  = regexp.MustCompile(`(?i)\bimport\b`)
)

// Tables extracts ordered unique normalized warehouse refs from SQL / filtered Qlik text.
func Tables(text string, opts Options) []string {
	f := opts.filter()

	var names []string
	for _, loc := range tableRE.FindAllStringSubmatchIndex(text, -1) {
		fullStart, refStart, refEnd := loc[0], loc[2], loc[3]
		if refEnd < len(text) && text[refEnd] == '.' {
			continue
		}
		lineStart := strings.LastIndex(text[:fullStart], "\n") + 1
		lineEnd := len(text)
		if i := strings.IndexByte(text[fullStart:], '\n'); i >= 0 {
			lineEnd = fullStart + i
		}
		lineAfter := text[fullStart:lineEnd]
		if importRE.MatchString(lineAfter) {
			continue
		}
		before := text[lineStart:fullStart]
		if extractIdx := strings.LastIndex(strings.ToLower(before), "extract"); extractIdx >= 0 {
			if !strings.Contains(before[extractIdx:], ")") {
				continue
			}
		}
		if ref := normalizeTableRef(text[refStart:refEnd], f); ref != "" {
			names = append(names, ref)
		}
	}

	for _, inline := range inlineRE.FindAllStringSubmatch(text, -1) {
		names = append(names, extractInlineTables(inline[1], f)...)
	}

	seenVars := map[string]struct{}{}
	for _, m := range fromVarRE.FindAllStringSubmatch(text, -1) {
		v := m[1]
		if _, ok := seenVars[v]; ok {
			continue
		}
		seenVars[v] = struct{}{}
		names = append(names, resolveVarTables(text, v, f)...)
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(names))
	for _, t := range names {
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func extractInlineTables(content string, f refFilter) []string {
	var tables []string
	for _, col := range []string{"source_table", "table_name"} {
		tables = append(tables, extractInlineColumnTables(content, col, f)...)
	}
	return tables
}

func extractInlineColumnTables(content, column string, f refFilter) []string {
	lines := splitNonEmpty(content)
	if len(lines) == 0 {
		return nil
	}
	header := splitCells(strings.ToLower(lines[0]))
	colIdx := -1
	for i, h := range header {
		if h == column {
			colIdx = i
			break
		}
	}
	if colIdx < 0 {
		return nil
	}
	var tables []string
	for _, line := range lines[1:] {
		parts := splitCells(line)
		if colIdx >= len(parts) {
			continue
		}
		if ref := normalizeTableRef(parts[colIdx], f); ref != "" {
			tables = append(tables, ref)
		}
	}
	return tables
}

func resolveVarTables(text, varName string, f refFilter) []string {
	letRE := regexp.MustCompile(`(?i)LET\s+` + regexp.QuoteMeta(varName) + `\s*=\s*['"]([\w-]+\.[\w-]+(?:\.[\w-]+)?)['"]`)
	var tables []string
	for _, m := range letRE.FindAllStringSubmatch(text, -1) {
		if ref := normalizeTableRef(m[1], f); ref != "" {
			tables = append(tables, ref)
		}
	}

	peekRE := regexp.MustCompile(`(?i)LET\s+` + regexp.QuoteMeta(varName) + `\s*=\s*Peek\(['"](\w+)['"]`)
	peek := peekRE.FindStringSubmatch(text)
	if peek == nil {
		return tables
	}
	col := strings.ToLower(peek[1])
	for _, inline := range inlineRE.FindAllStringSubmatch(text, -1) {
		tables = append(tables, extractInlineColumnTables(inline[1], col, f)...)
	}
	return tables
}

func normalizeTableRef(raw string, f refFilter) string {
	token := strings.ReplaceAll(strings.TrimSpace(raw), `"`, "")
	if !strings.Contains(token, ".") || looksLikePersonName(token) {
		return ""
	}
	parts := strings.Split(strings.ToLower(token), ".")
	switch len(parts) {
	case 3:
		catalog, schema, table := parts[0], parts[1], parts[2]
		if !validTableRef(schema, table, f.allow) {
			return ""
		}
		if f.excluded != nil {
			if _, ok := f.excluded[catalog]; ok {
				return schema + "." + table
			}
		}
		return catalog + "." + schema + "." + table
	case 2:
		schema, table := parts[0], parts[1]
		if !validTableRef(schema, table, f.allow) {
			return ""
		}
		return schema + "." + table
	default:
		return ""
	}
}

func looksLikePersonName(raw string) bool {
	parts := strings.SplitN(strings.TrimSpace(raw), ".", 2)
	if len(parts) != 2 {
		return false
	}
	left, right := parts[0], parts[1]
	if !isAlpha(left) || !isAlpha(right) {
		return false
	}
	return len(left) <= 2 && unicode.IsUpper(rune(left[0])) && unicode.IsUpper(rune(right[0]))
}

func validTableRef(schema, table string, allow map[string]struct{}) bool {
	if schema == "" || table == "" {
		return false
	}
	if !unicode.IsLetter(rune(schema[0])) || !unicode.IsLetter(rune(table[0])) {
		return false
	}
	if len(schema) <= 2 {
		return false
	}
	if allow != nil {
		_, ok := allow[schema]
		return ok
	}
	if strings.Contains(schema, "_") {
		return true
	}
	if _, ok := softSchemas[schema]; ok {
		return true
	}
	if len(schema) >= 6 {
		return true
	}
	if len(schema) >= 4 && strings.Contains(table, "_") {
		return true
	}
	return false
}

func isAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return s != ""
}

func splitNonEmpty(content string) []string {
	var out []string
	for _, ln := range strings.Split(content, "\n") {
		ln = strings.TrimSpace(ln)
		if ln != "" {
			out = append(out, ln)
		}
	}
	return out
}

func splitCells(line string) []string {
	fields := cellSplit.Split(line, -1)
	var out []string
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f != "" {
			out = append(out, f)
		}
	}
	return out
}
