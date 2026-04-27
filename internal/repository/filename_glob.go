package repository

import "strings"

// LIKEEscapeChar is the escape used with ILIKE ... ESCAPE in queries (see [FilenameGlobToILike]).
const LIKEEscapeChar = '#'

// FilenameGlobToILike converts a user pattern for use with
//
//	... filename ILIKE $1 ESCAPE '#'
//
// The only user wildcard is `*` (becomes %). The characters %, _ and # are
// always literal. When the input has no `*`, we apply a contains search
// (VSCode-like) by wrapping the escaped value with %...%.
func FilenameGlobToILike(s string) string {
	if s == "" {
		return "%"
	}
	if !strings.ContainsRune(s, '*') {
		return "%" + likeEscapeHash(s) + "%"
	}
	b := new(strings.Builder)
	b.Grow(len(s) * 2)
	for _, r := range s {
		if r == '*' {
			b.WriteRune('%')
			continue
		}
		escapeRuneForHash(b, r)
	}
	return b.String()
}

func likeEscapeHash(s string) string {
	b := new(strings.Builder)
	b.Grow(len(s) + 8)
	for _, r := range s {
		escapeRuneForHash(b, r)
	}
	return b.String()
}

func escapeRuneForHash(b *strings.Builder, r rune) {
	if r == '%' || r == '_' || r == rune(LIKEEscapeChar) {
		b.WriteRune(rune(LIKEEscapeChar))
	}
	b.WriteRune(r)
}
