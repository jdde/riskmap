// Package tests runs the acceptance cases in tests/acceptance against the
// example projects in tests/examples. The format of the cases is described in
// tests/README.md.
package tests

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/jdde/riskmap/internal/config"
	"github.com/jdde/riskmap/internal/eval"
	"github.com/jdde/riskmap/internal/input"
	"github.com/jdde/riskmap/internal/result"
)

// acceptanceFile mirrors tests/acceptance/<example>.yml.
//
// Go note: here the YAML library fills the struct directly. That is fine for
// test data. The rule file itself is read through the node tree instead,
// because it needs strict checks.
type acceptanceFile struct {
	Example string                    `yaml:"example"`
	Files   map[string]map[string]any `yaml:"files"`
	Changes []change                  `yaml:"changes"`
}

type change struct {
	ID       string   `yaml:"id"`
	Patch    string   `yaml:"patch"`
	Paths    []string `yaml:"paths"`
	Complete *bool    `yaml:"complete"` // a pointer, so that "not given" is nil
	Expect   struct {
		Status string         `yaml:"status"`
		Labels map[string]any `yaml:"labels"`
	} `yaml:"expect"`
}

func TestAcceptance(t *testing.T) {
	files, err := filepath.Glob("acceptance/*.yml")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 3 {
		t.Fatalf("found %d acceptance files, want at least 3", len(files))
	}
	for _, file := range files {
		var acceptance acceptanceFile
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(data, &acceptance); err != nil {
			t.Fatalf("%s: %v", file, err)
		}
		t.Run(acceptance.Example, func(t *testing.T) {
			runExample(t, acceptance)
		})
	}
}

func runExample(t *testing.T, acceptance acceptanceFile) {
	root := filepath.Join("examples", acceptance.Example)
	data, err := os.ReadFile(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Parse(config.FileName, data)
	if err != nil {
		t.Fatalf("rule file is invalid:\n%v", err)
	}

	// classify runs the whole chain up to the JSON output and reads the
	// output back, so the test sees what a user of the CLI sees.
	classify := func(paths []string, complete bool) (status string, labels map[string]any) {
		output := result.Classified(cfg, result.Policy{}, eval.Evaluate(cfg, toBytes(paths), complete), nil)
		var doc struct {
			Status string         `json:"status"`
			Labels map[string]any `json:"labels"`
		}
		if err := json.Unmarshal(output, &doc); err != nil {
			t.Fatalf("output is not valid JSON: %v", err)
		}
		return doc.Status, doc.Labels
	}

	// Part 1: every file of the example on its own.
	t.Run("files", func(t *testing.T) {
		for path, expect := range acceptance.Files {
			status, labels := classify([]string{path}, true)
			if status != "ok" {
				t.Errorf("%s: status %s, want ok", path, status)
			}
			compare(t, cfg, path, labels, expect)
		}

		// The list must be complete: a new file in the example needs an
		// expectation.
		for _, path := range listFiles(t, root) {
			if _, listed := acceptance.Files[path]; !listed {
				t.Errorf("%s exists in the example but has no entry under files", path)
			}
		}
		for path := range acceptance.Files {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err != nil {
				t.Errorf("%s has an entry under files but does not exist in the example", path)
			}
		}
	})

	// Part 2: changes of several files and the special cases.
	t.Run("changes", func(t *testing.T) {
		for _, c := range acceptance.Changes {
			complete := c.Complete == nil || *c.Complete
			status, labels := classify(c.Paths, complete)
			if status != c.Expect.Status {
				t.Errorf("%s: status %s, want %s", c.ID, status, c.Expect.Status)
			}
			compare(t, cfg, c.ID, labels, c.Expect.Labels)

			// The patch of a case must yield exactly the paths listed for it.
			if c.Patch == "" {
				continue
			}
			patch, err := os.ReadFile(filepath.Join("acceptance", filepath.FromSlash(c.Patch)))
			if err != nil {
				t.Errorf("%s: %v", c.ID, err)
				continue
			}
			fromPatch, err := input.PathsFromPatch(patch)
			if err != nil {
				t.Errorf("%s: %v", c.ID, err)
				continue
			}
			if got, want := uniqueSorted(fromPatch), uniqueSorted(toBytes(c.Paths)); !reflect.DeepEqual(got, want) {
				t.Errorf("%s: the patch names %q, the case lists %q", c.ID, got, want)
			}
		}
	})
}

func toBytes(paths []string) [][]byte {
	var out [][]byte
	for _, p := range paths {
		out = append(out, []byte(p))
	}
	return out
}

func uniqueSorted(paths [][]byte) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range paths {
		if !seen[string(p)] {
			seen[string(p)] = true
			out = append(out, string(p))
		}
	}
	sort.Strings(out)
	return out
}

// compare applies the rule from tests/README.md: a set without a value counts
// as [], a flag without a value as false, and an ordered dimension that the
// expectation does not name must have no value.
func compare(t *testing.T, cfg *config.Config, name string, got, want map[string]any) {
	t.Helper()
	for dimension := range want {
		if _, declared := cfg.Dimension(dimension); !declared {
			t.Errorf("%s: the expectation names %q, which is not a dimension", name, dimension)
		}
	}
	for _, d := range cfg.Dimensions {
		g, w := normalize(d, got[d.Name]), normalize(d, want[d.Name])
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s: %s is %v, want %v", name, d.Name, g, w)
		}
	}
}

func normalize(d config.Dimension, value any) any {
	switch d.Kind {
	case config.Set:
		// Both YAML and JSON give a list of "any"; turn it into sorted text.
		words := []string{}
		if list, ok := value.([]any); ok {
			for _, item := range list {
				words = append(words, item.(string))
			}
		}
		sort.Strings(words)
		return words
	case config.Flag:
		if value == nil {
			return false
		}
	}
	return value
}

// listFiles returns all files below root, relative to it and with "/".
func listFiles(t *testing.T, root string) []string {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}
