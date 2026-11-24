// Package glob implements the glob dialect of the Riskmap specification
// (section 3 of SPEC.md).
//
// The dialect is deliberately small:
//
//   - A pattern must match the whole path. Paths are relative and use "/".
//   - "*" matches zero or more characters, but never "/".
//   - "?" matches exactly one character, but never "/".
//   - "**" is only allowed as a whole segment and matches zero or more
//     directories. A trailing "**" matches everything below a directory.
//   - A leading dot is an ordinary character.
//   - Matching is case-sensitive and compares bytes, without Unicode
//     normalisation.
//   - Negation, brackets, braces, backslashes, ".." and empty segments are
//     errors, not patterns that silently never match.
//
// No third-party glob library is used on purpose. Existing libraries disagree
// on exactly these points, and the specification has to be the single source
// of truth.
package glob

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Pattern is a compiled glob. Create one with Compile.
//
// Go note: a struct is a plain data record. Fields that start with a lower-case
// letter are private to this package.
type Pattern struct {
	source   string
	segments []segment
}

// segment is one part of a pattern between two slashes.
type segment struct {
	// doubleStar is true for a "**" segment. Such a segment has no tokens.
	doubleStar bool
	tokens     []token
}

// tokenKind says what a token matches inside a single path segment.
type tokenKind int

const (
	literal  tokenKind = iota // a fixed run of bytes
	anyChar                   // "?": exactly one character
	anyChars                  // "*": zero or more characters
)

type token struct {
	kind tokenKind
	text []byte // only used by literal tokens
}

// Error describes why a pattern is not valid.
//
// Go note: any type with an Error() method can be returned as an error.
type Error struct {
	Pattern string
	Reason  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("invalid glob %q: %s", e.Pattern, e.Reason)
}

// Compile checks a pattern and prepares it for matching.
//
// Go note: functions report failure through a second return value instead of
// throwing an exception. The caller checks "if err != nil".
func Compile(pattern string) (*Pattern, error) {
	fail := func(reason string) (*Pattern, error) {
		return nil, &Error{Pattern: pattern, Reason: reason}
	}

	if pattern == "" {
		return fail("pattern is empty")
	}
	if strings.HasPrefix(pattern, "/") {
		return fail("leading \"/\" is not allowed, patterns are relative")
	}
	// These characters are reserved. Rejecting them everywhere keeps the door
	// open for later versions and avoids patterns that look like they work in
	// other tools but never match here.
	for _, reserved := range []string{"!", "{", "}", "[", "]", "\\"} {
		if strings.Contains(pattern, reserved) {
			return fail(fmt.Sprintf("%q is not allowed", reserved))
		}
	}
	if strings.ContainsRune(pattern, 0) {
		return fail("NUL is not allowed")
	}
	if !utf8.ValidString(pattern) {
		return fail("pattern is not valid UTF-8")
	}

	parts := strings.Split(pattern, "/")
	segments := make([]segment, 0, len(parts)+1)
	for _, part := range parts {
		switch {
		case part == "":
			return fail("empty segment (double or trailing \"/\")")
		case part == "..":
			return fail("\"..\" is not allowed")
		case part == ".":
			return fail("\".\" segment is not allowed, it can never match")
		case part == "**":
			// "a/**/**/b" means the same as "a/**/b".
			if n := len(segments); n > 0 && segments[n-1].doubleStar {
				continue
			}
			segments = append(segments, segment{doubleStar: true})
		case strings.Contains(part, "**"):
			return fail("\"**\" is only allowed as a whole segment")
		default:
			segments = append(segments, segment{tokens: tokenize(part)})
		}
	}

	// A trailing "**" matches everything below a directory, but not the
	// directory itself: "a/**" matches "a/b" and "a/b/c", not "a". That is the
	// same as "a/**/*", so we store it that way.
	if segments[len(segments)-1].doubleStar {
		segments = append(segments, segment{tokens: []token{{kind: anyChars}}})
	}

	return &Pattern{source: pattern, segments: segments}, nil
}

// tokenize splits one pattern segment into literal runs, "?" and "*".
func tokenize(part string) []token {
	var tokens []token
	var run []byte
	flush := func() {
		if len(run) > 0 {
			tokens = append(tokens, token{kind: literal, text: run})
			run = nil
		}
	}
	for i := 0; i < len(part); i++ {
		switch part[i] {
		case '*':
			flush()
			// "a**b" was rejected earlier, so two stars never meet here.
			tokens = append(tokens, token{kind: anyChars})
		case '?':
			flush()
			tokens = append(tokens, token{kind: anyChar})
		default:
			run = append(run, part[i])
		}
	}
	flush()
	return tokens
}

// String returns the pattern as it was written.
func (p *Pattern) String() string {
	return p.source
}

// Match reports whether the pattern matches the whole path.
//
// The path is taken as bytes because the specification compares the UTF-8 byte
// sequence. The caller is responsible for rejecting paths that are not valid
// UTF-8 before matching.
func (p *Pattern) Match(path []byte) bool {
	parts := bytes.Split(path, []byte("/"))

	// Walk pattern segments and path segments side by side. When a "**" is
	// seen we remember where, first try to let it match nothing, and come back
	// to let it swallow one more path segment each time the rest fails.
	pi, si := 0, 0            // position in p.segments and in parts
	starAt, starPath := -1, 0 // last "**" seen and the path position it started at
	for si < len(parts) {
		switch {
		case pi < len(p.segments) && p.segments[pi].doubleStar:
			starAt, starPath = pi, si
			pi++
		case pi < len(p.segments) && matchSegment(p.segments[pi].tokens, parts[si]):
			pi++
			si++
		case starAt >= 0:
			starPath++
			si = starPath
			pi = starAt + 1
		default:
			return false
		}
	}
	// The path is used up. Whatever is left of the pattern must be able to
	// match nothing, and only "**" can do that.
	for pi < len(p.segments) && p.segments[pi].doubleStar {
		pi++
	}
	return pi == len(p.segments)
}

// matchSegment matches the tokens of one pattern segment against one path
// segment. It uses the same remember-and-retry idea as Match, one level down.
func matchSegment(tokens []token, name []byte) bool {
	ti, ni := 0, 0            // position in tokens and in name
	starAt, starName := -1, 0 // last "*" seen and the name position it started at
	for ni < len(name) {
		if ti < len(tokens) {
			switch tok := tokens[ti]; tok.kind {
			case anyChars:
				starAt, starName = ti, ni
				ti++
				continue
			case anyChar:
				ni += charWidth(name[ni:])
				ti++
				continue
			case literal:
				if bytes.HasPrefix(name[ni:], tok.text) {
					ni += len(tok.text)
					ti++
					continue
				}
			}
		}
		if starAt < 0 {
			return false
		}
		// Let the last "*" take one more character and try again. Stepping by
		// whole characters keeps "?" from ever matching half of one.
		starName += charWidth(name[starName:])
		ni = starName
		ti = starAt + 1
	}
	for ti < len(tokens) && tokens[ti].kind == anyChars {
		ti++
	}
	return ti == len(tokens)
}

// charWidth returns the number of bytes of the first character in b. A
// character is one UTF-8 encoded code point. Bytes that are not valid UTF-8
// count as one character each.
func charWidth(b []byte) int {
	_, width := utf8.DecodeRune(b)
	return width
}
