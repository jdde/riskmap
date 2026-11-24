// Package input collects the paths of a change: from a patch file or from
// git. It implements section 10.2 of SPEC.md.
//
// Two rules from the specification shape this package. For a rename both the
// old and the new path count, and a deleted file counts. And paths are bytes:
// they are never split at line breaks or spaces.
package input

import (
	"bytes"
	"fmt"
	"strconv"
)

// PathsFromPatch returns the paths a patch touches. The patch must be in the
// format git writes ("git diff", "git format-patch"), with the usual "a/" and
// "b/" prefixes.
//
// The result may contain a path twice. An empty patch gives no paths.
func PathsFromPatch(patch []byte) ([][]byte, error) {
	var paths [][]byte
	lines := bytes.Split(patch, []byte("\n"))

	// Everything between one "diff --git" line and the next describes one
	// file. We collect the paths named there.
	sections := 0
	for i := 0; i < len(lines); {
		if !bytes.HasPrefix(lines[i], []byte("diff --git ")) {
			i++ // text before the first file, e.g. the mail header of format-patch
			continue
		}
		sections++
		header, lineNumber := lines[i], i+1
		i++

		var found [][]byte
		for i < len(lines) && !bytes.HasPrefix(lines[i], []byte("diff --git ")) {
			line := lines[i]
			switch {
			case bytes.HasPrefix(line, []byte("@@ ")):
				// A hunk. Its lines are file content and must be skipped by
				// count: a removed line "-- a/x" looks exactly like a header
				// line "--- a/x".
				i = skipHunk(lines, i)
				continue
			case bytes.HasPrefix(line, []byte("--- ")), bytes.HasPrefix(line, []byte("+++ ")):
				path, err := headerPath(line[4:])
				if err != nil {
					return nil, fmt.Errorf("patch line %d: %w", i+1, err)
				}
				if path != nil {
					found = append(found, path)
				}
			case bytes.HasPrefix(line, []byte("rename from ")):
				found = append(found, mustUnquote(line[len("rename from "):]))
			case bytes.HasPrefix(line, []byte("rename to ")):
				found = append(found, mustUnquote(line[len("rename to "):]))
			case bytes.HasPrefix(line, []byte("copy to ")):
				// The source of a copy is not changed, only the target is.
				found = append(found, mustUnquote(line[len("copy to "):]))
			}
			i++
		}

		// A file without content lines (a binary file, an empty new file, a
		// mode change) is only named in the "diff --git" line itself.
		if len(found) == 0 {
			path, err := gitHeaderPath(header[len("diff --git "):])
			if err != nil {
				return nil, fmt.Errorf("patch line %d: %w", lineNumber, err)
			}
			found = append(found, path)
		}
		paths = append(paths, found...)
	}

	if sections == 0 && len(bytes.TrimSpace(patch)) > 0 {
		return nil, fmt.Errorf("not a git patch: no line starts with \"diff --git\"")
	}
	return paths, nil
}

// skipHunk returns the index of the first line after the hunk that starts at
// lines[start]. The hunk header "@@ -3,7 +3,9 @@" says how many lines of the
// old and of the new file follow.
func skipHunk(lines [][]byte, start int) int {
	oldLeft, newLeft, ok := hunkCounts(lines[start])
	i := start + 1
	if !ok {
		return i
	}
	for i < len(lines) && (oldLeft > 0 || newLeft > 0) {
		line := lines[i]
		switch {
		case len(line) == 0 || line[0] == ' ':
			// A context line. Some tools strip the single space of an empty
			// context line, so an empty line counts as one.
			oldLeft--
			newLeft--
		case line[0] == '-':
			oldLeft--
		case line[0] == '+':
			newLeft--
		case line[0] == '\\':
			// "\ No newline at end of file" belongs to the line before.
		default:
			return i // not a hunk line; let the caller look at it
		}
		i++
	}
	return i
}

// hunkCounts reads the line counts from "@@ -3,7 +3,9 @@". A missing count,
// as in "@@ -3 +3 @@", means 1.
func hunkCounts(header []byte) (oldCount, newCount int, ok bool) {
	fields := bytes.Fields(header)
	if len(fields) < 3 || len(fields[1]) == 0 || fields[1][0] != '-' || len(fields[2]) == 0 || fields[2][0] != '+' {
		return 0, 0, false
	}
	count := func(rangeText []byte) (int, bool) {
		_, after, hasCount := bytes.Cut(rangeText[1:], []byte(","))
		if !hasCount {
			return 1, true
		}
		n, err := strconv.Atoi(string(after))
		return n, err == nil && n >= 0
	}
	oldCount, okOld := count(fields[1])
	newCount, okNew := count(fields[2])
	return oldCount, newCount, okOld && okNew
}

// headerPath reads the path from the rest of a "--- " or "+++ " line. It
// returns nil for /dev/null, which stands for "no file" on that side.
func headerPath(rest []byte) ([]byte, error) {
	var name []byte
	if len(rest) > 0 && rest[0] == '"' {
		unquoted, _, err := unquote(rest)
		if err != nil {
			return nil, err
		}
		name = unquoted
	} else {
		// Git puts a tab after a name that contains a space. An unquoted name
		// never contains a tab itself: that would make git quote it.
		name, _, _ = bytes.Cut(rest, []byte("\t"))
	}
	if bytes.Equal(name, []byte("/dev/null")) {
		return nil, nil
	}
	return stripPrefix(name)
}

// stripPrefix removes the "a/" or "b/" git puts in front of a path.
func stripPrefix(name []byte) ([]byte, error) {
	if len(name) > 2 && (name[0] == 'a' || name[0] == 'b') && name[1] == '/' {
		return name[2:], nil
	}
	return nil, fmt.Errorf("path %q does not start with a/ or b/; patches made with --no-prefix are not supported", name)
}

// gitHeaderPath reads the path from the rest of a "diff --git" line, which is
// "a/PATH b/PATH". It is only used when both sides name the same path.
//
// Git does not quote spaces, so "a/x y b/x y" cannot be split at a space. But
// since both halves are equal, the line can be cut in the middle.
func gitHeaderPath(rest []byte) ([]byte, error) {
	if len(rest) > 0 && rest[0] == '"' {
		// The second name is not needed: it is the same path.
		first, _, err := unquote(rest)
		if err != nil {
			return nil, err
		}
		return stripPrefix(first)
	}
	// len(rest) = len("a/PATH") + 1 space + len("b/PATH")
	if half := len(rest) / 2; len(rest)%2 == 1 && rest[half] == ' ' {
		left, right := rest[:half], rest[half+1:]
		if len(left) > 2 && bytes.Equal(left[2:], right[2:]) {
			return stripPrefix(left)
		}
	}
	return nil, fmt.Errorf("cannot read the path from %q", rest)
}

// mustUnquote returns the path of a "rename from" style line, which git
// quotes only if needed. Text that is not a valid quoted string is taken as
// it is.
func mustUnquote(text []byte) []byte {
	if len(text) > 0 && text[0] == '"' {
		if unquoted, _, err := unquote(text); err == nil {
			return unquoted
		}
	}
	return text
}

// unquote decodes a path that git wrote in quotes, such as
// "docs/\303\234ber uns.md". Git quotes a path if it contains a control
// character, a quote, a backslash or (by default) a byte outside ASCII, and
// writes those as C-style escapes.
//
// It returns the decoded bytes and whatever follows the closing quote.
func unquote(text []byte) (path, rest []byte, err error) {
	escapes := map[byte]byte{
		'a': '\a', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v',
		'\\': '\\', '"': '"',
	}
	for i := 1; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '"':
			return path, text[i+1:], nil
		case c != '\\':
			path = append(path, c)
		case i+1 >= len(text):
			return nil, nil, fmt.Errorf("quoted path %q ends in a backslash", text)
		case text[i+1] >= '0' && text[i+1] <= '3':
			// One byte as three octal digits, e.g. \303.
			if i+3 >= len(text) {
				return nil, nil, fmt.Errorf("quoted path %q has an incomplete octal escape", text)
			}
			value, parseErr := strconv.ParseUint(string(text[i+1:i+4]), 8, 8)
			if parseErr != nil {
				return nil, nil, fmt.Errorf("quoted path %q has an invalid octal escape", text)
			}
			path = append(path, byte(value))
			i += 3
		default:
			decoded, known := escapes[text[i+1]]
			if !known {
				return nil, nil, fmt.Errorf("quoted path %q has an unknown escape \\%c", text, text[i+1])
			}
			path = append(path, decoded)
			i++
		}
	}
	return nil, nil, fmt.Errorf("quoted path %q has no closing quote", text)
}
