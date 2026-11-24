package glob

import (
	"errors"
	"strings"
	"testing"
)

// The cases follow section 12 ("Conformance tests", area "Glob") of the
// specification. Each row is one pattern, one path and the expected result.
func TestMatch(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		// A pattern must match the whole path.
		{"README.md", "README.md", true},
		{"README.md", "README.md.bak", false},
		{"README.md", "docs/README.md", false},
		{"docs/a.md", "docs/a.md", true},
		{"docs/a.md", "docs/a.mdx", false},
		{"docs", "docs/a.md", false},

		// "*" does not cross "/".
		{"*.md", "a.md", true},
		{"*.md", "docs/a.md", false},
		{"docs/*", "docs/a.md", true},
		{"docs/*", "docs/sub/a.md", false},
		{"docs/*.md", "docs/a.md", true},
		{"apps/*/src/main.ts", "apps/api/src/main.ts", true},
		{"apps/*/src/main.ts", "apps/api/v2/src/main.ts", false},

		// "*" matches zero or more characters.
		{"a*", "a", true},
		{"a*b", "ab", true},
		{"a*b", "axxb", true},
		{"a*b", "axxbc", false},
		{"*", "anything", true},
		{"*a*b*", "xaxbx", true},
		{"*a*b*", "xbxax", false},
		{"lib/features/*/auth_*.dart", "lib/features/login/auth_service.dart", true},
		{"lib/features/*/auth_*.dart", "lib/features/login/oauth_service.dart", false},
		{"*_create_users_table.php", "0001_01_01_000000_create_users_table.php", true},

		// "?" matches exactly one character and does not cross "/".
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"a?c", "abbc", false},
		{"a?c", "a/c", false},
		{"docs/?.md", "docs/a.md", true},

		// "**" matches zero or more directories.
		{"**/x", "x", true},
		{"**/x", "a/x", true},
		{"**/x", "a/b/c/x", true},
		{"**/x", "a/xx", false},
		{"**/*.md", "README.md", true},
		{"**/*.md", "docs/deep/a.md", true},
		{"a/**/b", "a/b", true},
		{"a/**/b", "a/x/b", true},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "ab", false},
		{"a/**/b", "a/x/b/c", false},
		{"src/app/**/page.tsx", "src/app/page.tsx", true},
		{"src/app/**/page.tsx", "src/app/(shop)/checkout/page.tsx", true},
		{"a/**/b/**/c", "a/b/c", true},
		{"a/**/b/**/c", "a/1/b/2/3/c", true},
		{"a/**/b/**/c", "a/1/c", false},
		{"a/**/**/b", "a/b", true},

		// A trailing "**" matches everything below a directory, not the
		// directory itself.
		{"a/**", "a/b", true},
		{"a/**", "a/b/c/d", true},
		{"a/**", "a", false},
		{"a/**", "ab/c", false},
		{"**", "a", true},
		{"**", "a/b/c", true},

		// A pattern without "/" matches only in the root.
		{"package.json", "package.json", true},
		{"package.json", "apps/web/package.json", false},

		// Dotfiles are ordinary names.
		{"*", ".env", true},
		{"**", ".github/workflows/ci.yml", true},
		{".github/**", ".github/workflows/ci.yml", true},
		{"**/*.yml", ".github/workflows/ci.yml", true},
		{"docs/*", "docs/.hidden", true},

		// Matching is case-sensitive.
		{"README.md", "readme.md", false},
		{"docs/**", "Docs/a.md", false},

		// Near misses must not match.
		{"app/**", "appx/main.ts", false},
		{"docs/**", "docs2/readme.md", false},
		{"tests/**", "tests2/a_test.php", false},

		// Characters that are special elsewhere are literal here.
		{"src/app/(shop)/**", "src/app/(shop)/checkout/page.tsx", true},
		{"a.b", "a.b", true},
		{"a.b", "axb", false},
		{"a+b", "a+b", true},
		{"a+b", "aab", false},
		{"docs/Über uns.md", "docs/Über uns.md", true},
		// The path may contain characters that are not allowed in a pattern.
		{"src/app/api/auth/**", "src/app/api/auth/[...nextauth]/route.ts", true},
		{"docs/*", "docs/wow!.md", true},
		{"docs/**", "docs/a\nb.md", true},
		{"docs/**", "docs/a b.md", true},

		// "?" and "*" work on characters, not on bytes.
		{"?ber", "Über", true},
		{"??ber", "Über", false},
		{"*?", "€", true},
		{"*??", "€", false},
		{"*??", "€€", true},
		{"a?b", "a€b", true},

		// Bytes are compared without Unicode normalisation: "Ü" as one code
		// point is not the same as "U" followed by a combining diaeresis.
		{"docs/Über.md", "docs/Über.md", false},
	}

	for _, c := range cases {
		p, err := Compile(c.pattern)
		if err != nil {
			t.Errorf("Compile(%q) failed: %v", c.pattern, err)
			continue
		}
		if got := p.Match([]byte(c.path)); got != c.want {
			t.Errorf("%q against %q: got %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

// Patterns that other tools accept but that are errors in this dialect.
func TestCompileRejects(t *testing.T) {
	cases := []struct {
		pattern string
		reason  string // a fragment of the expected reason
	}{
		{"", "empty"},
		{"/docs/**", "leading"},
		{"!docs/**", `"!"`},
		{"docs/!a.md", `"!"`},
		{"src/{a,b}/**", `"{"`},
		{"src/[ab].ts", `"["`},
		{"src/a].ts", `"]"`},
		{`src\a.ts`, `"\\"`},
		{"../secrets", `".."`},
		{"a/../b", `".."`},
		{"./docs/**", `"."`},
		{"a/./b", `"."`},
		{"a//b", "empty segment"},
		{"docs/", "empty segment"},
		{"a**", "whole segment"},
		{"**a", "whole segment"},
		{"a/b**/c", "whole segment"},
		{"a/***/c", "whole segment"},
		{"a\x00b", "NUL"},
		{"a\xffb", "UTF-8"},
	}

	for _, c := range cases {
		_, err := Compile(c.pattern)
		if err == nil {
			t.Errorf("Compile(%q) succeeded, want an error", c.pattern)
			continue
		}
		// errors.As checks that the error is our own *Error type.
		var globErr *Error
		if !errors.As(err, &globErr) {
			t.Errorf("Compile(%q): error has type %T, want *glob.Error", c.pattern, err)
			continue
		}
		if !strings.Contains(globErr.Reason, c.reason) {
			t.Errorf("Compile(%q): reason %q does not mention %q", c.pattern, globErr.Reason, c.reason)
		}
	}
}

func TestString(t *testing.T) {
	p, err := Compile("a/**")
	if err != nil {
		t.Fatal(err)
	}
	if p.String() != "a/**" {
		t.Errorf("String() = %q, want the pattern as written", p.String())
	}
}

// A pattern with many "*" must not take exponential time on a path that
// almost matches.
func TestMatchManyStars(t *testing.T) {
	p, err := Compile(strings.Repeat("a*", 30) + "b")
	if err != nil {
		t.Fatal(err)
	}
	if p.Match([]byte(strings.Repeat("a", 200))) {
		t.Error("pattern ending in b matched a path without b")
	}

	deep, err := Compile(strings.Repeat("**/a/", 20) + "b")
	if err != nil {
		t.Fatal(err)
	}
	if deep.Match([]byte(strings.Repeat("a/", 200) + "c")) {
		t.Error("pattern ending in b matched a path ending in c")
	}
}
