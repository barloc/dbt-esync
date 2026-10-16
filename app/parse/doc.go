// Package parse extracts table refs and tags from SQL / Qlik scripts.
//
// FilterQlikScript slices a load script by LIB CONNECT TO + SUB/CALL expansion.
// Tables extracts ordered unique schema.table (or catalog.schema.table) refs.
// Tags separates maturity from other labels.
package parse
