package gitscan

// Test helpers (same package, _test.go pattern without exporting in production).
var (
	PathAllowedForTest    = pathAllowed
	BrowseURLForTest      = browseURL
	SanitizePathIDForTest = sanitizePathID
)
