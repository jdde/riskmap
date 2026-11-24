package main

// Tests for the output as other programs read it: JSON by default, one
// document on standard output, and a shape that matches the published JSON
// Schemas in the "schema" directory.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// conforms checks a decoded JSON value against a JSON Schema and returns what
// is wrong with it. It knows only the keywords the schemas of this repository
// use; an unknown keyword fails the test, so that a new keyword cannot be
// ignored silently.
//
// Unlike a real validator it also rejects properties the schema does not
// name. The schemas allow them, because readers must tolerate new fields. But
// a field the CLI writes without it being documented is a mistake.
//
// Go note: JSON decoded into "any" gives map[string]any for objects, []any
// for lists, float64 for all numbers, string, bool and nil.
func conforms(root map[string]any, schema map[string]any, value any, where string) []string {
	var problems []string
	fail := func(format string, args ...any) {
		problems = append(problems, where+": "+fmt.Sprintf(format, args...))
	}
	object, isObject := value.(map[string]any)

	for keyword, rule := range schema {
		switch keyword {
		case "$schema", "$id", "$defs", "title", "description", "then":
			// Nothing to check. "then" is handled together with "if".
		case "$ref":
			name := strings.TrimPrefix(rule.(string), "#/$defs/")
			target := root["$defs"].(map[string]any)[name].(map[string]any)
			problems = append(problems, conforms(root, target, value, where)...)
		case "type":
			names, isList := rule.([]any)
			if !isList {
				names = []any{rule}
			}
			if !hasType(value, names) {
				fail("is %T, want %v", value, names)
			}
		case "const":
			if !reflect.DeepEqual(value, rule) {
				fail("is %v, want %v", value, rule)
			}
		case "enum":
			found := false
			for _, allowed := range rule.([]any) {
				found = found || reflect.DeepEqual(value, allowed)
			}
			if !found {
				fail("is %v, want one of %v", value, rule)
			}
		case "required":
			for _, key := range rule.([]any) {
				if _, present := object[key.(string)]; !present {
					fail("has no %q", key)
				}
			}
		case "properties":
			for key, sub := range rule.(map[string]any) {
				if child, present := object[key]; present {
					problems = append(problems, conforms(root, sub.(map[string]any), child, where+"."+key)...)
				}
			}
		case "additionalProperties":
			named, _ := schema["properties"].(map[string]any)
			for key, child := range object {
				if _, isNamed := named[key]; !isNamed {
					problems = append(problems, conforms(root, rule.(map[string]any), child, where+"."+key)...)
				}
			}
		case "items":
			list, _ := value.([]any)
			for i, child := range list {
				problems = append(problems, conforms(root, rule.(map[string]any), child, fmt.Sprintf("%s[%d]", where, i))...)
			}
		case "allOf":
			for _, sub := range rule.([]any) {
				problems = append(problems, conforms(root, sub.(map[string]any), value, where)...)
			}
		case "if":
			if len(conforms(root, rule.(map[string]any), value, where)) == 0 {
				problems = append(problems, conforms(root, schema["then"].(map[string]any), value, where)...)
			}
		default:
			fail("the test does not know the schema keyword %q", keyword)
		}
	}

	// The extra strictness: no property the schema does not name.
	_, open := schema["additionalProperties"]
	if named, hasProperties := schema["properties"].(map[string]any); isObject && hasProperties && !open {
		for key := range object {
			if _, isNamed := named[key]; !isNamed {
				fail("has the property %q, which the schema does not describe", key)
			}
		}
	}
	sort.Strings(problems)
	return problems
}

func hasType(value any, names []any) bool {
	for _, name := range names {
		var ok bool
		switch name {
		case "object":
			_, ok = value.(map[string]any)
		case "array":
			_, ok = value.([]any)
		case "string":
			_, ok = value.(string)
		case "boolean":
			_, ok = value.(bool)
		case "integer":
			number, isNumber := value.(float64)
			ok = isNumber && number == float64(int64(number))
		case "null":
			ok = value == nil
		}
		if ok {
			return true
		}
	}
	return false
}

// checkSchema decodes stdout as exactly one JSON document, checks it against
// schema/<name>.schema.json and returns it.
func checkSchema(t *testing.T, name, stdout string) map[string]any {
	t.Helper()
	text, err := os.ReadFile(filepath.Join("..", "..", "schema", name+".schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(text, &schema); err != nil {
		t.Fatalf("%s schema is not JSON: %v", name, err)
	}
	// Go note: Unmarshal fails if anything but white space follows the
	// document, so this also proves that nothing else was printed.
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout)
	}
	if !strings.HasSuffix(stdout, "}\n") {
		t.Errorf("stdout does not end with a single line break: %q", stdout)
	}
	for _, problem := range conforms(schema, schema, doc, "result") {
		t.Errorf("%s schema: %s", name, problem)
	}
	return doc
}

const failingTests = `version: 1
dimensions:
  domain: { type: set }
  data:   { type: ordered, values: [internal, personal] }
default: { risk: high }
rules:
  - id: docs
    match: ["docs/**"]
    labels: { risk: low, domain: [docs] }
    reason: "Documentation"
tests:
  - path: "docs/a.md"
    expect: { risk: medium, domain: [], data: personal }
`

// TestOutputMatchesSchema runs every kind of result through the schemas.
func TestOutputMatchesSchema(t *testing.T) {
	missing := t.TempDir()
	invalid := write(t, "version: 2\nrules: 1\n")
	failing := write(t, failingTests)
	laravel := "../../tests/examples/laravel-api"
	flutter := "../../tests/examples/flutter-app"

	cases := []struct {
		name   string
		schema string
		args   []string
		code   int
		status string
	}{
		{"classify", "result", []string{"classify", "--root", example, "README.md", "src/lib/stripe.ts", ".riskmap.yml", "unknown.txt"}, exitOK, "ok"},
		{"classify, ordered dimension", "result", []string{"classify", "--root", laravel, "routes/api.php", "README.md"}, exitOK, "ok"},
		{"classify, sets", "result", []string{"classify", "--root", flutter, "pubspec.yaml", "lib/main.dart"}, exitOK, "ok"},
		{"classify, empty change", "result", []string{"classify", "--root", example}, exitOK, "ok"},
		{"classify, incomplete", "result", []string{"classify", "--root", example, "--incomplete", "README.md"}, exitOK, "incomplete"},
		{"classify, unreadable path", "result", []string{"classify", "--root", example, "docs/\xff.md"}, exitOK, "ok"},
		{"classify, no rule file", "result", []string{"classify", "--root", missing, "a"}, exitInvalid, "no-config"},
		{"classify, invalid rule file", "result", []string{"classify", "--root", invalid, "a"}, exitInvalid, "invalid-config"},
		{"check, passed", "result", []string{"check", "--root", example, "README.md"}, exitOK, "ok"},
		{"check, failed", "result", []string{"check", "--root", example, ".github/workflows/ci.yml"}, exitTooHigh, "ok"},
		{"check, incomplete", "result", []string{"check", "--root", example, "--incomplete", "README.md"}, exitTooHigh, "incomplete"},
		{"check, no rule file", "result", []string{"check", "--root", missing, "a"}, exitInvalid, "no-config"},
		{"check, invalid rule file", "result", []string{"check", "--root", invalid, "a"}, exitInvalid, "invalid-config"},
		{"validate", "validate", []string{"validate", "--root", example}, exitOK, "ok"},
		{"validate, failing tests", "validate", []string{"validate", "--root", failing}, exitInvalid, "tests-failed"},
		{"validate, no rule file", "validate", []string{"validate", "--root", missing}, exitInvalid, "no-config"},
		{"validate, invalid rule file", "validate", []string{"validate", "--root", invalid}, exitInvalid, "invalid-config"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, stdout, _ := invoke("", c.args...)
			doc := checkSchema(t, c.schema, stdout)
			if code != c.code || doc["status"] != c.status {
				t.Errorf("exit %d with status %v, want exit %d with status %q", code, doc["status"], c.code, c.status)
			}
			// The same input gives the same bytes.
			if _, again, _ := invoke("", c.args...); again != stdout {
				t.Errorf("the second run printed something else")
			}
		})
	}
}

// The schema check itself must notice mistakes, or the test above proves
// nothing.
func TestSchemaCheckNoticesMistakes(t *testing.T) {
	_, stdout, _ := invoke("", "check", "--root", example, "README.md")
	cases := []struct{ name, from, to, want string }{
		{"unknown status", `"status": "ok"`, `"status": "fine"`, "want one of"},
		{"wrong type", `"passed": true`, `"passed": "yes"`, "is string"},
		{"missing field", `"fail_on": "high",`, ``, `has no "fail_on"`},
		{"undocumented field", `"riskmap": 1,`, `"riskmap": 1, "extra": 1,`, "does not describe"},
		{"number with a fraction", `"failing_paths": 0`, `"failing_paths": 0.5`, "is float64"},
	}
	text, _ := os.ReadFile("../../schema/result.schema.json")
	var schema map[string]any
	json.Unmarshal(text, &schema)
	for _, c := range cases {
		if !strings.Contains(stdout, c.from) {
			t.Fatalf("%s: the output does not contain %s", c.name, c.from)
		}
		var doc any
		if err := json.Unmarshal([]byte(strings.Replace(stdout, c.from, c.to, 1)), &doc); err != nil {
			t.Fatal(err)
		}
		if problems := strings.Join(conforms(schema, schema, doc, "result"), "\n"); !strings.Contains(problems, c.want) {
			t.Errorf("%s: got %q, want a problem mentioning %q", c.name, problems, c.want)
		}
	}
	if problems := conforms(schema, map[string]any{"minimum": 1.0}, 1.0, "x"); len(problems) != 1 {
		t.Errorf("an unknown keyword went unnoticed: %v", problems)
	}
}

// TestCheckJSON checks the verdict that "check" adds to the result.
func TestCheckJSON(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		code    int
		passed  bool
		failing float64
	}{
		{"below the limit", []string{"README.md", "src/components/Button.tsx"}, exitOK, true, 0},
		{"at the limit", []string{"README.md", ".github/workflows/ci.yml", "prisma/schema.prisma"}, exitTooHigh, false, 2},
		{"lower limit", []string{"--fail-on", "medium", "README.md", "src/components/Button.tsx"}, exitTooHigh, false, 1},
		{"empty change", nil, exitTooHigh, false, 0},
		{"incomplete", []string{"--incomplete", "README.md"}, exitTooHigh, false, 0},
	}
	for _, c := range cases {
		code, stdout, _ := invoke("", append([]string{"check", "--root", example}, c.args...)...)
		check, _ := checkSchema(t, "result", stdout)["check"].(map[string]any)
		if code != c.code || check["passed"] != c.passed || check["failing_paths"] != c.failing {
			t.Errorf("%s: exit %d, check %v", c.name, code, check)
		}
	}

	// classify has no verdict.
	_, stdout, _ := invoke("", "classify", "--root", example, "README.md")
	if _, present := checkSchema(t, "result", stdout)["check"]; present {
		t.Errorf("classify printed a check object")
	}
}

// A tool needs the order of the levels to compare them. The result carries
// it, so the tool does not have to read the rule file.
func TestResultNamesTheDimensions(t *testing.T) {
	_, stdout, _ := invoke("", "classify", "--root", example, "README.md")
	dimensions := checkSchema(t, "result", stdout)["dimensions"].(map[string]any)
	risk := dimensions["risk"].(map[string]any)
	if risk["type"] != "ordered" || !reflect.DeepEqual(risk["values"], []any{"low", "medium", "high"}) {
		t.Errorf("risk = %v", risk)
	}
	if domain := dimensions["domain"].(map[string]any); domain["type"] != "set" || domain["values"] != nil {
		t.Errorf("domain = %v", domain)
	}
}

func TestValidateJSON(t *testing.T) {
	code, stdout, stderr := invoke("", "validate", "--root", example)
	doc := checkSchema(t, "validate", stdout)
	if code != exitOK || stderr != "" || doc["rules"] != 8.0 {
		t.Errorf("exit %d, stderr %q, result %v", code, stderr, doc)
	}

	code, stdout, _ = invoke("", "validate", "--root", write(t, failingTests))
	tests := checkSchema(t, "validate", stdout)["tests"].(map[string]any)
	failed := tests["failed"].([]any)
	if code != exitInvalid || tests["total"] != 1.0 || len(failed) != 3 {
		t.Fatalf("exit %d, tests %v", code, tests)
	}
	// One entry per dimension, sorted by the name of the dimension.
	want := []map[string]any{
		{"file": ".riskmap.yml", "line": 12.0, "path": "docs/a.md", "dimension": "data", "expected": "personal", "actual": nil},
		{"file": ".riskmap.yml", "line": 12.0, "path": "docs/a.md", "dimension": "domain", "expected": []any{}, "actual": []any{"docs"}},
		{"file": ".riskmap.yml", "line": 12.0, "path": "docs/a.md", "dimension": "risk", "expected": "medium", "actual": "low"},
	}
	for i, entry := range failed {
		if !reflect.DeepEqual(entry, any(want[i])) {
			t.Errorf("failure %d = %v, want %v", i, entry, want[i])
		}
	}
}

// --format text prints the report for people, with every command.
func TestFormatText(t *testing.T) {
	code, stdout, stderr := invoke("", "classify", "--format", "text", "--root", example, "README.md", ".github/workflows/ci.yml")
	if code != exitOK || stderr != "" || !strings.HasPrefix(stdout, "risk: high") || !strings.Contains(stdout, "  low     README.md\n") {
		t.Errorf("exit %d, stderr %q, stdout:\n%s", code, stderr, stdout)
	}
	// Without a rule file there is no report, only the message.
	code, stdout, _ = invoke("", "classify", "--format", "text", "--root", t.TempDir(), "a")
	if code != exitInvalid || stdout != "" {
		t.Errorf("exit %d, stdout %q", code, stdout)
	}
	for _, dir := range []string{t.TempDir(), write(t, "version: 2\n")} {
		code, stdout, stderr = invoke("", "validate", "--format", "text", "--root", dir)
		if code != exitInvalid || stdout != "" || stderr == "" {
			t.Errorf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	}
}

// With exit code 2 nothing is printed to standard output, so a caller never
// parses half a result.
func TestUsageErrorsPrintNoResult(t *testing.T) {
	cases := [][]string{
		{"classify", "--format", "yaml", "README.md"},
		{"check", "--format", "yaml", "README.md"},
		{"validate", "--format", "yaml"},
		{"check", "--root", example, "--fail-on", "critical", "README.md"},
		{"classify", "--root", example, "--patch", "does-not-exist.patch"},
		{"classify", "--root", example, "--base", "no-such-ref"},
	}
	for _, args := range cases {
		code, stdout, stderr := invoke("", args...)
		if code != exitUsage || stdout != "" || !strings.HasPrefix(stderr, "riskmap: ") {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
		}
	}
}
