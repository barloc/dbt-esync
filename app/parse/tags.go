package parse

import "strings"

// Tag is a source-provided label (e.g. Qlik QRS tag).
type Tag struct {
	Name string
}

// Tags splits maturity (low|medium|high) from the remaining tag names.
func Tags(tags []Tag) (maturity string, rest []string) {
	for _, t := range tags {
		name := strings.TrimSpace(t.Name)
		switch strings.ToLower(name) {
		case "low", "medium", "high":
			maturity = strings.ToLower(name)
		default:
			if name != "" {
				rest = append(rest, name)
			}
		}
	}
	return maturity, rest
}
