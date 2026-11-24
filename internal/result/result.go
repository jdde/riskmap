// Package result writes the outcome of a classification as JSON, following
// section 6 of SPEC.md.
//
// The same input must always give the same bytes. That is why set values are
// sorted, object keys have a fixed order and nothing like a timestamp is part
// of the output.
package result

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"unicode/utf8"

	"github.com/jdde/riskmap/internal/config"
	"github.com/jdde/riskmap/internal/eval"
)

// FormatVersion is the value of the "riskmap" field.
const FormatVersion = 1

// Status values for results without labels (section 5.1). The other two are
// defined in package eval.
const (
	StatusNoConfig      = "no-config"
	StatusInvalidConfig = "invalid-config"
	// StatusTestsFailed only occurs in the output of "riskmap validate".
	StatusTestsFailed = "tests-failed"
)

// Check is the verdict of "riskmap check". It is not part of the
// specification: the format only classifies, and check is a small gate on
// top of it.
type Check struct {
	FailOn string `json:"fail_on"`
	// Passed is the verdict. It is false whenever the exit code is not 0.
	Passed bool `json:"passed"`
	// FailingPaths counts the paths at or above FailOn. It can be 0 although
	// Passed is false: an empty or incomplete change, or no usable rule file.
	FailingPaths int `json:"failing_paths"`
}

// TestFailure is one failing entry of the "tests" block of a rule file.
type TestFailure struct {
	File      string `json:"file"`
	Line      int    `json:"line"`
	Path      string `json:"path"`
	Dimension string `json:"dimension"`
	Expected  any    `json:"expected"`
	Actual    any    `json:"actual"`
}

// PolicyFile names one rule file that was read, with the hash of its content.
type PolicyFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// NewPolicyFile computes the hash of a rule file.
func NewPolicyFile(path string, content []byte) PolicyFile {
	sum := sha256.Sum256(content)
	return PolicyFile{Path: path, SHA256: hex.EncodeToString(sum[:])}
}

// Policy says where the rules came from.
type Policy struct {
	// Ref is the git commit the rule files were read from. It is empty when
	// they were read from the working directory.
	Ref   string       `json:"ref,omitempty"`
	Files []PolicyFile `json:"files"`
}

// The types below describe the JSON document. The text in backquotes after a
// field is a "struct tag": it tells the JSON encoder which key to use, and
// "omitempty" tells it to leave the key out when the value is empty.

type document struct {
	Riskmap int     `json:"riskmap"`
	Status  string  `json:"status"`
	Policy  *Policy `json:"policy,omitempty"`
	// Dimensions repeats the declarations of the rule file, so that a tool
	// can compare levels without reading the rule file itself.
	Dimensions map[string]dimension `json:"dimensions,omitempty"`
	Labels     map[string]any       `json:"labels,omitempty"`
	Because    []because            `json:"because,omitempty"`
	// Paths is a pointer so that an empty list is still written as [] for a
	// classified change, but left out when there is no classification at all.
	Paths  *[]pathEntry `json:"paths,omitempty"`
	Errors []problem    `json:"errors,omitempty"`
	Check  *Check       `json:"check,omitempty"`
	// The two fields below are only written by "riskmap validate".
	Rules *int   `json:"rules,omitempty"`
	Tests *tests `json:"tests,omitempty"`
}

type dimension struct {
	Type string `json:"type"`
	// Values lists the levels of an ordered dimension, mildest first.
	Values []string `json:"values,omitempty"`
}

type tests struct {
	Total  int           `json:"total"`
	Failed []TestFailure `json:"failed"`
}

type pathEntry struct {
	Path string `json:"path"`
	// PathBase64 holds the exact bytes of a path that is not valid UTF-8.
	// "path" is then only an approximation.
	PathBase64 string         `json:"path_base64,omitempty"`
	Labels     map[string]any `json:"labels"`
	Because    []because      `json:"because"`
}

type because struct {
	Kind   string `json:"kind"`
	Config string `json:"config,omitempty"`
	Rule   string `json:"rule,omitempty"`
	Glob   string `json:"glob,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type problem struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Message string `json:"message"`
}

// Classified returns the JSON for a change that was classified. check is the
// verdict of "riskmap check", or nil.
func Classified(cfg *config.Config, policy Policy, r *eval.Result, check *Check) []byte {
	doc := document{
		Riskmap:    FormatVersion,
		Status:     r.Status,
		Policy:     &policy,
		Dimensions: dimensionsToJSON(cfg),
		Labels:     labelsToJSON(cfg, r.Labels),
		Because:    becauseToJSON(r.Because),
		Check:      check,
	}
	paths := []pathEntry{}
	for _, p := range r.Paths {
		entry := pathEntry{
			// A path that is not valid UTF-8 cannot be written as a JSON
			// string. The encoder replaces the broken bytes with U+FFFD.
			Path:    string(p.Path),
			Labels:  labelsToJSON(cfg, p.Labels),
			Because: becauseToJSON(p.Because),
		}
		if !utf8.Valid(p.Path) {
			entry.PathBase64 = base64.StdEncoding.EncodeToString(p.Path)
		}
		paths = append(paths, entry)
	}
	doc.Paths = &paths
	return encode(doc)
}

// NoConfig returns the JSON for a repository without a rule file. There are
// no labels: a tool must treat this like the strictest level.
func NoConfig(check *Check) []byte {
	return encode(document{Riskmap: FormatVersion, Status: StatusNoConfig, Check: check})
}

// InvalidConfig returns the JSON for a rule file that is not valid. As with
// NoConfig there are no labels.
func InvalidConfig(policy Policy, problems []config.Problem, check *Check) []byte {
	doc := document{Riskmap: FormatVersion, Status: StatusInvalidConfig, Policy: &policy, Check: check}
	for _, p := range problems {
		doc.Errors = append(doc.Errors, problem{File: p.File, Line: p.Line, Column: p.Column, Message: p.Message})
	}
	return encode(doc)
}

// Validated returns the JSON of "riskmap validate" for a rule file that could
// be read: its dimensions, the number of rules and the outcome of its tests.
func Validated(cfg *config.Config, policy Policy, failures []TestFailure) []byte {
	status := eval.StatusOK
	if len(failures) > 0 {
		status = StatusTestsFailed
	}
	rules := len(cfg.Rules)
	return encode(document{
		Riskmap:    FormatVersion,
		Status:     status,
		Policy:     &policy,
		Dimensions: dimensionsToJSON(cfg),
		Rules:      &rules,
		// A nil list would be written as null instead of [].
		Tests: &tests{Total: len(cfg.Tests), Failed: append([]TestFailure{}, failures...)},
	})
}

func dimensionsToJSON(cfg *config.Config) map[string]dimension {
	out := map[string]dimension{}
	for _, d := range cfg.Dimensions {
		out[d.Name] = dimension{Type: d.Kind.String(), Values: d.Values}
	}
	return out
}

// ValueToJSON turns one label value into a plain value: text for an ordered
// dimension (nil if it has no value), a list for a set and true or false for
// a flag.
func ValueToJSON(d config.Dimension, value config.Value) any {
	switch d.Kind {
	case config.Set:
		// A nil list would be written as null instead of [].
		return append([]string{}, value.Set...)
	case config.Flag:
		return value.Flag
	}
	if value.Level == "" {
		return nil
	}
	return value.Level
}

// labelsToJSON turns labels into plain values: text for ordered dimensions, a
// list for sets and true or false for flags.
//
// Go note: "any" is a value of any type. The JSON encoder writes the keys of
// a map in sorted order, which keeps the output stable.
func labelsToJSON(cfg *config.Config, labels config.Labels) map[string]any {
	out := map[string]any{}
	for _, d := range cfg.Dimensions {
		value, present := labels[d.Name]
		if !present {
			continue
		}
		out[d.Name] = ValueToJSON(d, value)
	}
	return out
}

func becauseToJSON(entries []eval.Because) []because {
	out := []because{}
	for _, e := range entries {
		out = append(out, because{Kind: e.Kind, Config: e.Config, Rule: e.Rule, Glob: e.Glob, Reason: e.Reason})
	}
	return out
}

// encode writes the document as indented JSON with a final newline.
func encode(doc document) []byte {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	// By default Go writes < > & as < and so on, for safe use in HTML.
	// The output here is not HTML, so paths stay readable.
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(doc); err != nil {
		// Cannot happen: the document only holds text, numbers, booleans and
		// lists of those.
		panic(err)
	}
	return buffer.Bytes()
}
