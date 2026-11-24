package input

import (
	"reflect"
	"strings"
	"testing"
)

// paths runs PathsFromPatch and returns the result as text.
func paths(t *testing.T, patch string) []string {
	t.Helper()
	got, err := PathsFromPatch([]byte(patch))
	if err != nil {
		t.Fatalf("PathsFromPatch failed: %v", err)
	}
	var out []string
	for _, p := range got {
		out = append(out, string(p))
	}
	return out
}

func TestPathsFromPatch(t *testing.T) {
	cases := []struct {
		name  string
		patch string
		want  []string
	}{
		{"empty patch", "", nil},

		{"changed file", `diff --git a/src/a.ts b/src/a.ts
index 2e5bfa1..c050fba 100644
--- a/src/a.ts
+++ b/src/a.ts
@@ -1,2 +1,2 @@
 one
-two
+three
`, []string{"src/a.ts", "src/a.ts"}},

		{"new file", `diff --git a/new.txt b/new.txt
new file mode 100644
index 0000000..7d2dcdd
--- /dev/null
+++ b/new.txt
@@ -0,0 +1 @@
+hello
`, []string{"new.txt"}},

		{"deleted file", `diff --git a/old.txt b/old.txt
deleted file mode 100644
index 13d1bda..0000000
--- a/old.txt
+++ /dev/null
@@ -1 +0,0 @@
-bye
`, []string{"old.txt"}},

		{"rename without content change", `diff --git a/src/lib/auth.ts b/docs/auth.md
similarity index 100%
rename from src/lib/auth.ts
rename to docs/auth.md
`, []string{"src/lib/auth.ts", "docs/auth.md"}},

		{"copy counts only the target", `diff --git a/a.txt b/b.txt
similarity index 100%
copy from a.txt
copy to b.txt
`, []string{"b.txt"}},

		{"binary file", `diff --git a/assets/logo.png b/assets/logo.png
index 613754c..145a07d 100644
GIT binary patch
literal 12
Tc${NkU|;|M00aO5

literal 10
Rc${NkU|;|M00aO5

`, []string{"assets/logo.png"}},

		{"mode change only", `diff --git a/run.sh b/run.sh
old mode 100644
new mode 100755
`, []string{"run.sh"}},

		{"empty new file with a space in its name", `diff --git a/docs/a b.md b/docs/a b.md
new file mode 100644
index 0000000..e69de29
`, []string{"docs/a b.md"}},

		{"name that contains ' b/'", `diff --git a/x b/y b/x b/y
new file mode 100644
index 0000000..e69de29
`, []string{"x b/y"}},

		{"space in the name, with content", "diff --git a/docs/a b.md b/docs/a b.md\n" +
			"index 2e5bfa1..c050fba 100644\n--- a/docs/a b.md\t\n+++ b/docs/a b.md\t\n@@ -1 +1 @@\n-a\n+b\n",
			[]string{"docs/a b.md", "docs/a b.md"}},

		{"quoted name", `diff --git "a/docs/\303\234ber uns.md" "b/docs/\303\234ber uns.md"
new file mode 100644
index 0000000..7d2dcdd
--- /dev/null
+++ "b/docs/\303\234ber uns.md"` + "\t" + `
@@ -0,0 +1 @@
+hello
`, []string{"docs/Über uns.md"}},

		{"quoted name without content", `diff --git "a/a\tb\"c\\d\ne.txt" "b/a\tb\"c\\d\ne.txt"
new file mode 100644
index 0000000..e69de29
`, []string{"a\tb\"c\\d\ne.txt"}},

		{"quoted rename", `diff --git "a/\303\234.md" b/u.md
similarity index 100%
rename from "\303\234.md"
rename to u.md
`, []string{"Ü.md", "u.md"}},

		// File content that looks like patch headers must not be read as
		// headers. The removed line is "-- a/fake", the added "++ b/fake".
		{"content that looks like a header", `diff --git a/real.txt b/real.txt
index 2e5bfa1..c050fba 100644
--- a/real.txt
+++ b/real.txt
@@ -1,3 +1,3 @@
 context
--- a/fake
-diff --git a/fake2 b/fake2
+++ b/fake
+rename to fake3
`, []string{"real.txt", "real.txt"}},

		{"two files and a mail header", `From 1234 Mon Sep 17 00:00:00 2001
Subject: [PATCH] change

--- a/not-a-file
---
 a.txt | 2 +-
diff --git a/a.txt b/a.txt
index 2e5bfa1..c050fba 100644
--- a/a.txt
+++ b/a.txt
@@ -1 +1 @@
-a
+b
diff --git a/b.txt b/b.txt
index 2e5bfa1..c050fba 100644
--- a/b.txt
+++ b/b.txt
@@ -1 +1 @@
-a
+b
\ No newline at end of file
`, []string{"a.txt", "a.txt", "b.txt", "b.txt"}},
	}

	for _, c := range cases {
		if got := paths(t, c.patch); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestPathsFromPatchErrors(t *testing.T) {
	cases := []struct {
		name  string
		patch string
		want  string
	}{
		{"not a patch", "hello\nworld\n", "not a git patch"},
		{"plain unified diff", "--- old.txt\n+++ new.txt\n@@ -1 +1 @@\n-a\n+b\n", "not a git patch"},
		{"no prefix", "diff --git x.txt x.txt\n--- x.txt\n+++ x.txt\n@@ -1 +1 @@\n-a\n+b\n", "does not start with a/ or b/"},
		{"different names without rename lines", "diff --git a/x.txt b/y.txt\n", "cannot read the path"},
		{"broken quoting", "diff --git \"a/x.txt b/x.txt\n", "no closing quote"},
		{"unknown escape", "diff --git \"a/x\\q.txt\" \"b/x\\q.txt\"\n", "unknown escape"},
		{"broken quoting in a file header", "diff --git a/x b/x\n--- \"a/x\n", "no closing quote"},
		{"backslash at the end", "diff --git \"a/x\\\n", "ends in a backslash"},
		{"incomplete octal escape", "diff --git \"a/\\30\n", "incomplete octal escape"},
		{"invalid octal escape", "diff --git \"a/\\389\" \"b/\\389\"\n", "invalid octal escape"},
	}
	for _, c := range cases {
		_, err := PathsFromPatch([]byte(c.patch))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error mentioning %q", c.name, err, c.want)
		}
	}
}

func TestSplitNUL(t *testing.T) {
	cases := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"a\x00", []string{"a"}},
		{"a", []string{"a"}},
		{"a\x00b c\x00d\ne\x00", []string{"a", "b c", "d\ne"}},
		// An empty entry is kept. It is classified as an unreadable path.
		{"a\x00\x00b\x00", []string{"a", "", "b"}},
	}
	for _, c := range cases {
		var got []string
		for _, p := range SplitNUL([]byte(c.input)) {
			got = append(got, string(p))
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("SplitNUL(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

// TestPathsFromPatchOddHunks checks that unusual hunks do not hide the files
// that follow them.
func TestPathsFromPatchOddHunks(t *testing.T) {
	cases := []struct {
		name  string
		patch string
		want  []string
	}{
		{"hunk header that cannot be read",
			"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ broken @@\n-a\n+b\n",
			[]string{"x", "x"}},
		{"no newline at end of file",
			"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-a\n\\ No newline at end of file\n+b\n" +
				"diff --git a/y b/y\n--- a/y\n+++ b/y\n@@ -1 +1 @@\n-a\n+b\n",
			[]string{"x", "x", "y", "y"}},
		{"hunk shorter than its header says",
			"diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,5 +1,5 @@\n a\n" +
				"diff --git a/y b/y\n--- a/y\n+++ b/y\n@@ -1 +1 @@\n-a\n+b\n",
			[]string{"x", "x", "y", "y"}},
	}
	for _, c := range cases {
		if got := paths(t, c.patch); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
