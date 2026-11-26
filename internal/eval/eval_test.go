package eval

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jdde/riskmap/internal/config"
)

const rules = `version: 1
dimensions:
  domain:    { type: set }
  protected: { type: flag }
  data:      { type: ordered, values: [none, internal, personal] }
default: { risk: high, domain: [unknown] }
rules:
  - id: app
    match: ["app/**"]
    except: ["app/**/*.md"]
    labels: { risk: medium, domain: [app] }
    reason: "application code"
  - id: auth
    match: ["app/auth/**"]
    labels: { risk: high, domain: [auth], protected: true }
    reason: "login"
  - id: docs
    match: ["**/*.md"]
    labels: { risk: low, domain: [docs] }
    reason: "documentation"
  - id: models
    match: ["app/models/**"]
    labels: { data: personal }
    reason: "personal data"
  - id: yaml
    match: ["**/*.yml"]
    labels: { risk: low, domain: [config] }
    reason: "configuration files"
`

func parse(t *testing.T, text string) *config.Config {
	// Go note: t.Helper makes a failure point to the caller's line.
	t.Helper()
	cfg, err := config.Parse(".riskmap.yml", []byte(text))
	if err != nil {
		t.Fatalf("rule file is invalid:\n%v", err)
	}
	return cfg
}

// summary writes labels as one line, for example
// "data=personal domain=[app] protected=false risk=medium".
func summary(cfg *config.Config, labels config.Labels) string {
	var parts []string
	for _, d := range cfg.Dimensions {
		value, present := labels[d.Name]
		if !present {
			continue
		}
		switch d.Kind {
		case config.Ordered:
			parts = append(parts, d.Name+"="+value.Level)
		case config.Set:
			parts = append(parts, d.Name+"=["+strings.Join(value.Set, ",")+"]")
		case config.Flag:
			if value.Flag {
				parts = append(parts, d.Name+"=true")
			} else {
				parts = append(parts, d.Name+"=false")
			}
		}
	}
	return strings.Join(parts, " ")
}

// kinds lists the trace of a path, for example "rule:app rule:auth".
func kinds(because []Because) string {
	var parts []string
	for _, b := range because {
		if b.Rule != "" {
			parts = append(parts, b.Kind+":"+b.Rule)
		} else {
			parts = append(parts, b.Kind)
		}
	}
	return strings.Join(parts, " ")
}

func TestEvaluatePath(t *testing.T) {
	cfg := parse(t, rules)
	cases := []struct {
		path   string
		labels string
		trace  string
	}{
		// One rule.
		{"app/main.go", "domain=[app] protected=false risk=medium", "rule:app"},
		// Two rules: the stricter level wins, sets are united, flags are ored.
		{"app/auth/login.go", "domain=[app,auth] protected=true risk=high", "rule:app rule:auth"},
		// A rule that only sets another dimension does not lower anything.
		{"app/models/user.go", "data=personal domain=[app] protected=false risk=medium", "rule:app rule:models"},
		// "except" only takes the path out of its own rule.
		{"app/README.md", "domain=[docs] protected=false risk=low", "rule:docs"},
		{"app/auth/NOTES.md", "domain=[auth,docs] protected=true risk=high", "rule:auth rule:docs"},
		// No rule: the default.
		{"Makefile", "domain=[unknown] protected=false risk=high", "default"},
		{"application/main.go", "domain=[unknown] protected=false risk=high", "default"},
		// Self-protection: without a matching rule the default applies, too.
		{"sub/.riskmap.yaml", "domain=[unknown] protected=false risk=high", "default"},
		// Self-protection on top of a mild rule: every ordered dimension and
		// flag is at its strictest, sets get the default's values in addition.
		{".riskmap.yml", "data=personal domain=[config,unknown] protected=true risk=high", "rule:yaml builtin:riskmap-config"},
		{"packages/a/.riskmap.yml", "data=personal domain=[config,unknown] protected=true risk=high", "rule:yaml builtin:riskmap-config"},
		{"my.riskmap.yml", "domain=[config] protected=false risk=low", "rule:yaml"},
		// Paths that cannot be represented get the strictest result.
		{"docs/\xff.md", "data=personal domain=[app,auth,config,docs,unknown] protected=true risk=high", "unreadable-path"},
		{"docs/a\x00b.md", "data=personal domain=[app,auth,config,docs,unknown] protected=true risk=high", "unreadable-path"},
		{"", "data=personal domain=[app,auth,config,docs,unknown] protected=true risk=high", "unreadable-path"},
		// Another spelling of a path must not slip past the strict rules.
		{"./docs/setup.md", "data=personal domain=[app,auth,config,docs,unknown] protected=true risk=high", "unreadable-path"},
		{"x/../docs/setup.md", "data=personal domain=[app,auth,config,docs,unknown] protected=true risk=high", "unreadable-path"},
		{"/docs/setup.md", "data=personal domain=[app,auth,config,docs,unknown] protected=true risk=high", "unreadable-path"},
		{"docs//setup.md", "data=personal domain=[app,auth,config,docs,unknown] protected=true risk=high", "unreadable-path"},
		{"docs/", "data=personal domain=[app,auth,config,docs,unknown] protected=true risk=high", "unreadable-path"},
	}
	for _, c := range cases {
		got := EvaluatePath(cfg, []byte(c.path))
		if s := summary(cfg, got.Labels); s != c.labels {
			t.Errorf("%q: labels %s, want %s", c.path, s, c.labels)
		}
		if k := kinds(got.Because); k != c.trace {
			t.Errorf("%q: trace %s, want %s", c.path, k, c.trace)
		}
	}
}

func TestEvaluateTrace(t *testing.T) {
	cfg := parse(t, rules)
	got := EvaluatePath(cfg, []byte("app/auth/login.go")).Because
	want := []Because{
		{Kind: KindRule, Config: ".riskmap.yml", Rule: "app", Glob: "app/**", Reason: "application code"},
		{Kind: KindRule, Config: ".riskmap.yml", Rule: "auth", Glob: "app/auth/**", Reason: "login"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("trace = %+v, want %+v", got, want)
	}
}

func TestEvaluateChange(t *testing.T) {
	cfg := parse(t, rules)
	paths := func(list ...string) [][]byte {
		var out [][]byte
		for _, p := range list {
			out = append(out, []byte(p))
		}
		return out
	}
	const strictest = "data=personal domain=[app,auth,config,docs,unknown] protected=true risk=high"

	cases := []struct {
		name     string
		paths    [][]byte
		complete bool
		status   string
		labels   string
		because  string
		count    int // number of path entries
	}{
		{"several paths are merged", paths("README.md", "app/main.go"), true,
			StatusOK, "domain=[app,docs] protected=false risk=medium", "", 2},
		{"mild paths stay mild", paths("README.md", "docs/a.md"), true,
			StatusOK, "domain=[docs] protected=false risk=low", "", 2},
		{"duplicates count once", paths("README.md", "README.md"), true,
			StatusOK, "domain=[docs] protected=false risk=low", "", 1},
		{"empty change gets the default", nil, true,
			StatusOK, "domain=[unknown] protected=false risk=high", KindEmptyChange, 0},
		{"incomplete input gets the strictest result", paths("README.md"), false,
			StatusIncomplete, strictest, KindIncompleteInput, 1},
		{"incomplete and empty", nil, false,
			StatusIncomplete, strictest, KindIncompleteInput, 0},
		{"one unreadable path makes the change strict", paths("README.md", "a\xffb"), true,
			StatusOK, strictest, "", 2},
	}
	for _, c := range cases {
		got := Evaluate(cfg, c.paths, c.complete)
		if got.Status != c.status {
			t.Errorf("%s: status %s, want %s", c.name, got.Status, c.status)
		}
		if s := summary(cfg, got.Labels); s != c.labels {
			t.Errorf("%s: labels %s, want %s", c.name, s, c.labels)
		}
		if k := kinds(got.Because); k != c.because {
			t.Errorf("%s: because %q, want %q", c.name, k, c.because)
		}
		if len(got.Paths) != c.count {
			t.Errorf("%s: %d path entries, want %d", c.name, len(got.Paths), c.count)
		}
	}
}

// A broad mild rule before or after a narrow strict rule gives the same
// result, and so does any order of the paths.
func TestOrderDoesNotMatter(t *testing.T) {
	// Move the first rule ("app") to the end of the file.
	start := strings.Index(rules, "  - id: app")
	end := strings.Index(rules, "  - id: auth")
	reordered := rules[:start] + rules[end:] + rules[start:end]

	a, b := parse(t, rules), parse(t, reordered)
	if a.Rules[0].ID == b.Rules[0].ID {
		t.Fatal("test setup: the rules were not reordered")
	}

	forward := [][]byte{[]byte("app/auth/login.go"), []byte("README.md"), []byte("Makefile")}
	backward := [][]byte{forward[2], forward[1], forward[0]}

	first := Evaluate(a, forward, true)
	second := Evaluate(b, backward, true)
	if !reflect.DeepEqual(first, second) {
		t.Errorf("results differ:\n%+v\n%+v", first, second)
	}
}

// Evaluating must not change the rule file it was given.
func TestEvaluateDoesNotChangeConfig(t *testing.T) {
	cfg := parse(t, rules)
	Evaluate(cfg, [][]byte{[]byte("app/auth/login.go"), []byte("Makefile"), []byte(".riskmap.yml")}, false)
	if !reflect.DeepEqual(cfg, parse(t, rules)) {
		t.Error("the config was changed by Evaluate")
	}
}
