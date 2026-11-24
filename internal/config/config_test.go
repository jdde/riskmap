package config

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A small valid file that the invalid cases below are derived from.
const valid = `version: 1
dimensions:
  domain:    { type: set }
  protected: { type: flag }
  data:      { type: ordered, values: [none, internal, personal] }
default: { risk: high }
rules:
  - id: docs
    match: ["docs/**", "README.md"]
    except: ["docs/internal/**"]
    labels: { risk: low, domain: [docs, public, docs] }
    reason: "Documentation"
  - match: [".github/**"]
    labels: { risk: high, protected: true, data: internal }
    reason: "CI"
tests:
  - path: "docs/a.md"
    expect: { risk: low, domain: [docs] }
x-owner: "platform team"
`

func TestParseValid(t *testing.T) {
	cfg, err := Parse(".riskmap.yml", []byte(valid))
	if err != nil {
		t.Fatalf("Parse failed:\n%v", err)
	}

	var names []string
	for _, d := range cfg.Dimensions {
		names = append(names, d.Name+":"+d.Kind.String())
	}
	if got, want := strings.Join(names, " "), "data:ordered domain:set protected:flag risk:ordered"; got != want {
		t.Errorf("dimensions = %q, want %q", got, want)
	}
	if d, ok := cfg.Dimension("data"); !ok || d.Rank("personal") != 2 || d.Rank("nope") != -1 {
		t.Errorf("dimension data = %+v, %v", d, ok)
	}

	if got := cfg.Default["risk"].Level; got != "high" {
		t.Errorf("default risk = %q, want high", got)
	}

	if len(cfg.Rules) != 2 {
		t.Fatalf("got %d rules, want 2", len(cfg.Rules))
	}
	docs := cfg.Rules[0]
	if docs.ID != "docs" || docs.Reason != "Documentation" || docs.Line != 8 {
		t.Errorf("first rule = %+v", docs)
	}
	if len(docs.Match) != 2 || docs.Match[0].String() != "docs/**" || len(docs.Except) != 1 {
		t.Errorf("globs of first rule: match %v, except %v", docs.Match, docs.Except)
	}
	// Set values are sorted and duplicates are dropped.
	if got, want := docs.Labels["domain"].Set, []string{"docs", "public"}; !reflect.DeepEqual(got, want) {
		t.Errorf("domain = %v, want %v", got, want)
	}

	// A rule without id is named after its position in the list.
	ci := cfg.Rules[1]
	if ci.ID != "1" || !ci.Labels["protected"].Flag || ci.Labels["data"].Level != "internal" {
		t.Errorf("second rule = %+v", ci)
	}

	if len(cfg.Tests) != 1 || cfg.Tests[0].Path != "docs/a.md" || cfg.Tests[0].Expect["risk"].Level != "low" {
		t.Errorf("tests = %+v", cfg.Tests)
	}
}

// The rule files of the example projects must stay valid.
func TestParseExamples(t *testing.T) {
	files, err := filepath.Glob("../../tests/examples/*/.riskmap.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 {
		t.Fatalf("found %d example rule files, want at least 3", len(files))
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := Parse(file, data)
		if err != nil {
			t.Errorf("%v", err)
			continue
		}
		if len(cfg.Rules) == 0 || len(cfg.Tests) == 0 {
			t.Errorf("%s: %d rules and %d tests, want some of each", file, len(cfg.Rules), len(cfg.Tests))
		}
	}
}

// Every case is a rule file with exactly one thing wrong. "want" is a fragment
// of the expected message, "line" the line it must point to (0 = not checked).
func TestParseInvalid(t *testing.T) {
	// with returns the valid file with one piece of text replaced.
	with := func(old, new string) string {
		if !strings.Contains(valid, old) {
			t.Fatalf("test setup: %q is not part of the valid file", old)
		}
		return strings.Replace(valid, old, new, 1)
	}

	cases := []struct {
		name string
		text string
		want string
		line int
	}{
		// YAML level
		{"empty file", "", "file is empty", 0},
		{"only a comment", "# nothing\n", "file is empty", 0},
		{"not a mapping", "- a\n- b\n", "must be a mapping", 1},
		{"syntax error", "version: 1\nrules: [\n", "YAML syntax error", 0},
		{"two documents", valid + "---\nversion: 1\n", "more than one YAML document", 0},
		{"invalid UTF-8", "version: 1\n# \xff\n", "not valid UTF-8", 0},
		{"anchor", with("default: {", "default: &d {"), "anchors", 6},
		{"alias", with(`reason: "Documentation"`, `reason: &r "Documentation"`) + "x-copy: *r\n", "aliases", 20},
		{"tag", with("version: 1", "version: !!int 1"), "tags", 1},
		{"duplicate key", with("default: { risk: high }", "default: { risk: high }\nversion: 1"), `duplicate key "version"`, 7},
		{"duplicate key in a rule", with(`reason: "CI"`, "reason: \"CI\"\n    reason: \"again\""), `duplicate key "reason"`, 0},
		{"key is a list", with("version: 1", "version: 1\n[a, b]: c"), "key must be plain text", 2},

		// top level
		{"unknown key", with("x-owner:", "owner:"), `unknown key "owner"`, 19},
		{"missing version", with("version: 1\n", ""), `missing key "version"`, 0},
		{"version as text", with("version: 1", `version: "1"`), "version must be a whole number", 1},
		{"version 2", with("version: 1", "version: 2"), "unsupported version 2", 1},
		{"version float", with("version: 1", "version: 1.0"), "version must be a whole number", 1},
		{"missing default", with("default: { risk: high }\n", ""), `missing key "default"`, 0},
		{"default without risk", with("default: { risk: high }", "default: { protected: true }"), `default must contain "risk"`, 6},
		{"missing rules", "version: 1\ndefault: { risk: high }\n", `missing key "rules"`, 0},
		{"rules not a list", "version: 1\ndefault: { risk: high }\nrules: {}\n", "rules must be a list", 3},

		// dimensions
		{"redefine risk", with("domain:    { type: set }", "risk: { type: set }"), `"risk" is predefined`, 3},
		{"declare review", with("domain:    { type: set }", "review: { type: set }"), `"review" is reserved`, 3},
		{"dimension named x-", with("protected: { type: flag }", "x-flag: { type: flag }"), `must not start with "x-"`, 4},
		{"unknown type", with("{ type: flag }", "{ type: boolean }"), `unknown type "boolean"`, 4},
		{"missing type", with("{ type: flag }", "{ }"), `missing key "type"`, 4},
		{"ordered without values", with("{ type: ordered, values: [none, internal, personal] }", "{ type: ordered }"), `needs "values"`, 5},
		{"ordered with empty values", with("values: [none, internal, personal]", "values: []"), "values must not be empty", 5},
		{"ordered with duplicate value", with("values: [none, internal, personal]", "values: [a, b, a]"), `duplicate value "a"`, 5},
		{"values on a set", with("{ type: set }", "{ type: set, values: [a] }"), "only allowed for type ordered", 3},
		{"unknown key in dimension", with("{ type: set }", "{ type: set, merge: union }"), `unknown key "merge"`, 3},

		// labels
		{"undeclared dimension", with("risk: low, domain: [docs, public, docs]", "risk: low, team: [a]"), `label "team": no such dimension`, 11},
		{"value outside values", with("risk: low", "risk: critical"), `"critical" is not one of low, medium, high`, 11},
		{"ordered value as list", with("risk: low", "risk: [low]"), `label "risk" must be text`, 11},
		{"set value not a list", with("domain: [docs, public, docs]", "domain: docs"), "must be a list", 11},
		{"set value is a number", with("domain: [docs, public, docs]", "domain: [1]"), "put it in quotes", 11},
		{"flag as text", with("protected: true", `protected: "true"`), "must be true or false", 14},
		{"flag as yes", with("protected: true", "protected: yes"), "must be true or false", 14},
		{"no labels", with("labels: { risk: high, protected: true, data: internal }", "labels: {}"), "at least one label", 14},

		// rules
		{"unknown key in rule", with("    except:", "    exclude:"), `unknown key "exclude"`, 10},
		{"duplicate id", with("  - match: [\".github/**\"]", "  - id: docs\n    match: [\".github/**\"]"), `duplicate rule id "docs", first used in line 8`, 13},
		{"id collides with position", with("id: docs", `id: "1"`), `duplicate rule id "1"`, 13},
		{"id as number", with("id: docs", "id: 7"), "put it in quotes", 8},
		{"missing match", with("  - match: [\".github/**\"]\n    labels", "  - labels"), `missing key "match"`, 13},
		{"empty match", with(`match: [".github/**"]`, "match: []"), "match must not be empty", 13},
		{"match not a list", with(`match: [".github/**"]`, `match: ".github/**"`), "match must be a list", 13},
		{"invalid glob", with(`"docs/internal/**"`, `"!docs/internal/**"`), `invalid glob "!docs/internal/**"`, 10},
		{"glob with braces", with(`"README.md"`, `"*.{md,txt}"`), `"{" is not allowed`, 9},
		{"glob twice in match", with(`"README.md"`, `"docs/**"`), "listed twice", 9},
		{"glob in match and except", with(`"docs/internal/**"`, `"README.md"`), "listed twice", 10},
		{"missing labels", with("    labels: { risk: low, domain: [docs, public, docs] }\n", ""), `missing key "labels"`, 8},
		{"missing reason", with("    reason: \"Documentation\"\n", ""), `rule "docs": missing key "reason"`, 8},
		{"blank reason", with(`reason: "Documentation"`, `reason: "  "`), "reason must not be empty", 12},
		{"reason is empty", with(`reason: "Documentation"`, "reason:"), "reason must be text, but is empty", 12},

		// tests
		{"test without path", with("  - path: \"docs/a.md\"\n    expect", "  - expect"), `missing key "path"`, 0},
		{"test without expect", with("    expect: { risk: low, domain: [docs] }\n", ""), `missing key "expect"`, 17},
		{"test with bad value", with("expect: { risk: low, domain: [docs] }", "expect: { risk: none }"), `"none" is not one of`, 18},
		{"unknown key in test", with("    expect:", "    expects:"), `unknown key "expects"`, 18},
	}

	for _, c := range cases {
		// Go note: t.Run makes every case a test of its own with its own name
		// in the output.
		t.Run(c.name, func(t *testing.T) {
			cfg, err := Parse(".riskmap.yml", []byte(c.text))
			if err == nil {
				t.Fatalf("Parse succeeded, want an error mentioning %q", c.want)
			}
			if cfg != nil {
				t.Error("Parse returned a config together with an error")
			}
			var invalid *InvalidError
			if !errors.As(err, &invalid) {
				t.Fatalf("error has type %T, want *InvalidError", err)
			}
			for _, problem := range invalid.Problems {
				if strings.Contains(problem.Message, c.want) {
					if c.line != 0 && problem.Line != c.line {
						t.Errorf("problem is reported in line %d, want %d: %s", problem.Line, c.line, problem)
					}
					return
				}
			}
			t.Errorf("no problem mentions %q, got:\n%v", c.want, err)
		})
	}
}

// All problems of a file are reported at once, in the order of the file.
func TestParseReportsAllProblems(t *testing.T) {
	text := `version: 3
default: { risk: extreme }
rules:
  - match: ["a/**"]
    labels: { risk: low }
`
	_, err := Parse("sub/.riskmap.yml", []byte(text))
	if err == nil {
		t.Fatal("Parse succeeded")
	}
	got := err.Error()
	want := `sub/.riskmap.yml:1:10: unsupported version 3; this tool knows version 1
sub/.riskmap.yml:2:18: label "risk": "extreme" is not one of low, medium, high
sub/.riskmap.yml:4:5: rule 0: missing key "reason"`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Values that YAML 1.1 parsers and YAML 1.2 parsers read differently.
func TestScalarTypes(t *testing.T) {
	// Each value is used as a set value. Text is accepted, anything else is
	// rejected with a hint to quote it.
	cases := []struct {
		value  string
		isText bool
	}{
		{"docs", true},
		{`"123"`, true},
		{"'true'", true},
		{"yes", true},   // a boolean only in YAML 1.1
		{"on", true},    // a boolean only in YAML 1.1
		{"1_000", true}, // a number only in YAML 1.1
		{"0b11", true},  // a number only in YAML 1.1
		{"1.2.3", true},
		{"v1", true},
		{"123", false},
		{"012", false},
		{"0x1F", false},
		{"0o17", false},
		{"1.5", false},
		{"1e3", false},
		{".inf", false},
		{"true", false},
		{"False", false},
		{"null", false},
		{"~", false},
	}
	for _, c := range cases {
		text := "version: 1\ndimensions: { d: { type: set } }\ndefault: { risk: high, d: [" + c.value + "] }\nrules: []\n"
		_, err := Parse(".riskmap.yml", []byte(text))
		if c.isText && err != nil {
			t.Errorf("%s was rejected, want it to be read as text: %v", c.value, err)
		}
		if !c.isText && err == nil {
			t.Errorf("%s was accepted as text, want an error", c.value)
		}
	}
}

// 012 is the number 12 in YAML 1.2, not octal 10.
func TestVersionIsReadAsYAML12(t *testing.T) {
	for value, want := range map[string]string{
		"01":   "",
		"0x1":  "",
		"+1":   "",
		"012":  "unsupported version 12",
		"0o12": "unsupported version 10",
		"1_0":  "must be a whole number",
	} {
		_, err := Parse(".riskmap.yml", []byte("version: "+value+"\ndefault: { risk: high }\nrules: []\n"))
		switch {
		case want == "" && err != nil:
			t.Errorf("version %s was rejected: %v", value, err)
		case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
			t.Errorf("version %s: got %v, want an error mentioning %q", value, err, want)
		}
	}
}

// TestParseWrongShapes feeds Parse values of the wrong kind: a number where a
// mapping belongs, a list where text belongs, and so on.
func TestParseWrongShapes(t *testing.T) {
	const rules = "rules:\n  - match: [\"a\"]\n    labels: { risk: low }\n    reason: \"r\"\n"
	file := func(dimensions, rest string) string {
		text := "version: 1\ndefault: { risk: high }\n"
		if dimensions != "" {
			text += "dimensions: " + dimensions + "\n"
		}
		if rest == "" {
			rest = rules
		}
		return text + rest
	}
	cases := []struct {
		name string
		text string
		want string
	}{
		{"dimensions is a number", file("5", ""), "dimensions"},
		{"dimension name is a number", file("{ 1: { type: set } }", ""), "a dimension name must be text"},
		{"dimension is a number", file("{ d: 5 }", ""), `dimension "d"`},
		{"type is a list", file("{ d: { type: [set] } }", ""), "type"},
		{"values is a number", file("{ d: { type: ordered, values: 5 } }", ""), "values"},
		{"a value is a number", file("{ d: { type: ordered, values: [a, 5] } }", ""), "a value"},
		{"labels is a number", file("", "rules:\n  - match: [\"a\"]\n    labels: 5\n    reason: \"r\"\n"), "labels"},
		{"a rule is a number", file("", "rules: [5]\n"), "rule"},
		{"a glob is a number", file("", "rules:\n  - match: [5]\n    labels: { risk: low }\n    reason: \"r\"\n"), "glob"},
		{"tests is a number", file("", rules+"tests: 5\n"), "tests"},
		{"a test is a number", file("", rules+"tests: [5]\n"), "test"},
		{"version out of range", "version: 99999999999999999999999\ndefault: { risk: high }\n" + rules, "out of range"},
		{"only a document marker", "---\n", ""},
		{"second document is broken", file("", "") + "---\na: [\n", "YAML syntax error"},
		{"syntax error", "a: [\n", "YAML syntax error"},
		{"syntax error without a line", "\"a\\x\"", "YAML syntax error"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(".riskmap.yml", []byte(c.text))
			if err == nil {
				t.Fatalf("Parse succeeded, want an error mentioning %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error does not mention %q:\n%v", c.want, err)
			}
		})
	}
}

func TestSmallHelpers(t *testing.T) {
	if got := Kind(99).String(); got != "unknown" {
		t.Errorf("Kind(99).String() = %q", got)
	}
	cfg, err := Parse(".riskmap.yml", []byte(valid))
	if err != nil {
		t.Fatal(err)
	}
	if _, found := cfg.Dimension("no-such-dimension"); found {
		t.Error("Dimension found a dimension that is not declared")
	}
	problems := map[string]Problem{
		"f.yml: m":     {File: "f.yml", Message: "m"},
		"f.yml:3: m":   {File: "f.yml", Line: 3, Message: "m"},
		"f.yml:3:7: m": {File: "f.yml", Line: 3, Column: 7, Message: "m"},
	}
	for want, problem := range problems {
		if got := problem.String(); got != want {
			t.Errorf("Problem.String() = %q, want %q", got, want)
		}
	}
}
