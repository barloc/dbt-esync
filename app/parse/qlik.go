package parse

import (
	"regexp"
	"strings"
)

var (
	blockCommentRE = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineCommentRE  = regexp.MustCompile(`(?i)^\s*(//|REM\s)`)
	subHeaderRE    = regexp.MustCompile(`(?i)^\s*sub\s+([A-Za-z_]\w*)`)
	subEndRE       = regexp.MustCompile(`(?i)^\s*end\s*sub\b`)
	callRE         = regexp.MustCompile(`(?i)^\s*call\s+([A-Za-z_]\w*)`)
	libConnectRE   = regexp.MustCompile(`(?i)\bLIB\s+CONNECT\s+TO\b`)
)

const (
	maxCallDepth     = 50
	maxExpandedLines = 100_000
)

// FilterQlikScript returns the warehouse slice of a Qlik load script for datasource.
// Pipeline: strip comments → expand SUB/CALL → keep lines under matching LIB CONNECT TO.
// Datasource match is a case-insensitive substring of the connect line.
func FilterQlikScript(body, datasource string) string {
	ds := strings.ToLower(datasource)
	lines := inlineSubs(strings.Split(stripComments(body), "\n"))

	var out []string
	include := false
	for _, ln := range lines {
		if libConnectRE.MatchString(ln) {
			include = strings.Contains(strings.ToLower(ln), ds)
			continue
		}
		if !include {
			continue
		}
		line := strings.TrimSpace(ln)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") {
			line = line[1:]
			switch {
			case strings.HasSuffix(line, "];"):
				line = line[:len(line)-2]
			case strings.HasSuffix(line, "]"):
				line = line[:len(line)-1]
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func stripComments(script string) string {
	withoutBlocks := blockCommentRE.ReplaceAllString(script, " ")
	var kept []string
	for _, line := range strings.Split(withoutBlocks, "\n") {
		if lineCommentRE.MatchString(line) {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func inlineSubs(lines []string) []string {
	subs := map[string][]string{}
	var main []string
	var current string
	inSub := false

	for _, line := range lines {
		if m := subHeaderRE.FindStringSubmatch(line); m != nil {
			current = strings.ToLower(m[1])
			subs[current] = nil
			inSub = true
			continue
		}
		if inSub && subEndRE.MatchString(line) {
			inSub = false
			current = ""
			continue
		}
		if inSub {
			subs[current] = append(subs[current], line)
		} else {
			main = append(main, line)
		}
	}

	var expanded []string
	var expand func(block []string, depth int)
	expand = func(block []string, depth int) {
		if depth > maxCallDepth {
			return
		}
		for _, line := range block {
			if len(expanded) > maxExpandedLines {
				return
			}
			if m := callRE.FindStringSubmatch(line); m != nil {
				name := strings.ToLower(m[1])
				if body, ok := subs[name]; ok {
					expand(body, depth+1)
					continue
				}
			}
			expanded = append(expanded, line)
		}
	}
	expand(main, 0)
	return expanded
}
