package result

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jdde/riskmap/internal/config"
	"github.com/jdde/riskmap/internal/eval"
)

const rules = `version: 1
dimensions:
  domain:    { type: set }
  protected: { type: flag }
  data:      { type: ordered, values: [none, personal] }
default: { risk: high }
rules:
  - id: docs
    match: ["docs/**"]
    labels: { risk: low, domain: [docs] }
    reason: "Documentation <html> & more"
  - id: ui
    match: ["src/**"]
    labels: { risk: medium, domain: [ui, app] }
    reason: "User interface"
`

func classify(t *testing.T, text string, paths ...string) []byte {
	t.Helper()
	cfg, err := config.Parse(".riskmap.yml", []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	var input [][]byte
	for _, p := range paths {
		input = append(input, []byte(p))
	}
	policy := Policy{Files: []PolicyFile{NewPolicyFile(".riskmap.yml", []byte(text))}}
	return Classified(cfg, policy, eval.Evaluate(cfg, input, true), nil)
}

func TestClassified(t *testing.T) {
	got := string(classify(t, rules, "src/a.ts", "docs/a b.md"))
	want := `{
  "riskmap": 1,
  "status": "ok",
  "policy": {
    "files": [
      {
        "path": ".riskmap.yml",
        "sha256": "` + NewPolicyFile("", []byte(rules)).SHA256 + `"
      }
    ]
  },
  "dimensions": {
    "data": {
      "type": "ordered",
      "values": [
        "none",
        "personal"
      ]
    },
    "domain": {
      "type": "set"
    },
    "protected": {
      "type": "flag"
    },
    "risk": {
      "type": "ordered",
      "values": [
        "low",
        "medium",
        "high"
      ]
    }
  },
  "labels": {
    "domain": [
      "app",
      "docs",
      "ui"
    ],
    "protected": false,
    "risk": "medium"
  },
  "paths": [
    {
      "path": "docs/a b.md",
      "labels": {
        "domain": [
          "docs"
        ],
        "protected": false,
        "risk": "low"
      },
      "because": [
        {
          "kind": "rule",
          "config": ".riskmap.yml",
          "rule": "docs",
          "glob": "docs/**",
          "reason": "Documentation <html> & more"
        }
      ]
    },
    {
      "path": "src/a.ts",
      "labels": {
        "domain": [
          "app",
          "ui"
        ],
        "protected": false,
        "risk": "medium"
      },
      "because": [
        {
          "kind": "rule",
          "config": ".riskmap.yml",
          "rule": "ui",
          "glob": "src/**",
          "reason": "User interface"
        }
      ]
    }
  ]
}
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The hash is the SHA-256 of the file content, as "sha256sum" prints it.
func TestPolicyFileHash(t *testing.T) {
	got := NewPolicyFile(".riskmap.yml", []byte("abc")).SHA256
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Errorf("hash = %s, want %s", got, want)
	}
}

// Reordering rules and paths must not change a single byte.
func TestOutputIsDeterministic(t *testing.T) {
	start := strings.Index(rules, "  - id: docs")
	end := strings.Index(rules, "  - id: ui")
	reordered := rules[:start] + rules[end:] + rules[start:end]

	a := classify(t, rules, "src/a.ts", "docs/a.md", "Makefile")
	b := classify(t, reordered, "Makefile", "docs/a.md", "src/a.ts", "docs/a.md")

	// The hash of the rule file differs, because the file differs. Everything
	// after the policy block must be identical.
	cut := func(output []byte) []byte {
		return output[bytes.Index(output, []byte(`"labels"`)):]
	}
	if !bytes.Equal(cut(a), cut(b)) {
		t.Errorf("outputs differ:\n%s\n%s", a, b)
	}
}

// An empty change still has "labels", an empty "paths" list and its reason.
func TestEmptyChange(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(classify(t, rules), &doc); err != nil {
		t.Fatal(err)
	}
	paths, ok := doc["paths"].([]any)
	if !ok || len(paths) != 0 {
		t.Errorf("paths = %v, want an empty list", doc["paths"])
	}
	because := doc["because"].([]any)[0].(map[string]any)
	if because["kind"] != "empty-change" {
		t.Errorf("because = %v, want empty-change", because)
	}
	labels := doc["labels"].(map[string]any)
	if labels["risk"] != "high" {
		t.Errorf("labels = %v, want the default", labels)
	}
	// An ordered dimension without a value is left out.
	if _, present := labels["data"]; present {
		t.Errorf("labels = %v, want no value for data", labels)
	}
}

// Unusual characters in paths survive the round trip through JSON.
func TestPathsWithSpecialCharacters(t *testing.T) {
	paths := []string{"docs/a b.md", "docs/\"quoted\".md", "docs/line\nbreak.md", "docs/Über.md", "docs/sep .md", "docs/<a>&.md"}
	var doc struct {
		Paths []struct {
			Path string `json:"path"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(classify(t, rules, paths...), &doc); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, p := range doc.Paths {
		seen[p.Path] = true
	}
	for _, p := range paths {
		if !seen[p] {
			t.Errorf("path %q is missing from the output", p)
		}
	}
}

func TestNoConfig(t *testing.T) {
	want := "{\n  \"riskmap\": 1,\n  \"status\": \"no-config\"\n}\n"
	if got := string(NoConfig(nil)); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestInvalidConfig(t *testing.T) {
	text := "version: 2\n"
	_, err := config.Parse(".riskmap.yml", []byte(text))
	invalid, ok := err.(*config.InvalidError)
	if !ok {
		t.Fatalf("got %v, want an *InvalidError", err)
	}
	policy := Policy{Files: []PolicyFile{NewPolicyFile(".riskmap.yml", []byte(text))}}

	var doc struct {
		Status string         `json:"status"`
		Labels map[string]any `json:"labels"`
		Errors []struct {
			File    string `json:"file"`
			Line    int    `json:"line"`
			Column  int    `json:"column"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(InvalidConfig(policy, invalid.Problems, nil), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Status != "invalid-config" || doc.Labels != nil {
		t.Errorf("status %q with labels %v, want invalid-config without labels", doc.Status, doc.Labels)
	}
	if len(doc.Errors) != 3 {
		t.Fatalf("got %d errors, want 3: %+v", len(doc.Errors), doc.Errors)
	}
	// Problems are sorted by position; the version is the last one in line 1.
	last := doc.Errors[2]
	if last.File != ".riskmap.yml" || last.Line != 1 || last.Column != 10 || !strings.Contains(last.Message, "unsupported version 2") {
		t.Errorf("last error = %+v", last)
	}
}

// A path that is not valid UTF-8 keeps its exact bytes in "path_base64".
func TestPathThatIsNotUTF8(t *testing.T) {
	var doc struct {
		Paths []struct {
			Path       string `json:"path"`
			PathBase64 string `json:"path_base64"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(classify(t, rules, "docs/a.md", "docs/\xff.md"), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Paths[0].PathBase64 != "" {
		t.Errorf("a readable path got path_base64 %q", doc.Paths[0].PathBase64)
	}
	// "ZG9jcy//Lm1k" is base64 for the bytes of "docs/\xff.md".
	if got := doc.Paths[1]; got.Path != "docs/\ufffd.md" || got.PathBase64 != "ZG9jcy//Lm1k" {
		t.Errorf("got %+v", got)
	}
}

// The verdict of "riskmap check" is added to every kind of result.
func TestCheckVerdict(t *testing.T) {
	check := &Check{FailOn: "high", Passed: false, FailingPaths: 0}
	want := "{\n  \"riskmap\": 1,\n  \"status\": \"no-config\",\n  \"check\": {\n    \"fail_on\": \"high\",\n    \"passed\": false,\n    \"failing_paths\": 0\n  }\n}\n"
	if got := string(NoConfig(check)); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestValidated(t *testing.T) {
	cfg, err := config.Parse(".riskmap.yml", []byte(rules))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := cfg.Dimension("data")
	domain, _ := cfg.Dimension("domain")
	failures := []TestFailure{
		{File: ".riskmap.yml", Line: 7, Path: "a", Dimension: "data", Expected: ValueToJSON(data, config.Value{Level: "personal"}), Actual: ValueToJSON(data, config.Value{})},
		{File: ".riskmap.yml", Line: 8, Path: "b", Dimension: "domain", Expected: ValueToJSON(domain, config.Value{Set: []string{"ui"}}), Actual: ValueToJSON(domain, config.Value{})},
	}
	var doc struct {
		Status string `json:"status"`
		Rules  int    `json:"rules"`
		Tests  struct {
			Total  int              `json:"total"`
			Failed []map[string]any `json:"failed"`
		} `json:"tests"`
	}
	if err := json.Unmarshal(Validated(cfg, Policy{}, failures), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Status != "tests-failed" || doc.Rules != 2 || len(doc.Tests.Failed) != 2 {
		t.Fatalf("got %+v", doc)
	}
	// An ordered dimension without a value is null, an empty set is [].
	if first := doc.Tests.Failed[0]; first["expected"] != "personal" || first["actual"] != nil {
		t.Errorf("first failure = %v", first)
	}
	if actual, ok := doc.Tests.Failed[1]["actual"].([]any); !ok || len(actual) != 0 {
		t.Errorf("second failure = %v", doc.Tests.Failed[1])
	}

	// Without failures the list is written as [] and not as null.
	if got := string(Validated(cfg, Policy{}, nil)); !strings.Contains(got, `"status": "ok"`) || !strings.Contains(got, `"failed": []`) {
		t.Errorf("got:\n%s", got)
	}
}
