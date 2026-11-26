// Package config reads and validates a rule file (.riskmap.yml), following
// sections 2 and 4 of SPEC.md.
//
// Parse either returns a Config that is known to be valid, or an error that
// lists every problem with its line and column. There is no half-valid result:
// the specification forbids falling back to defaults for an invalid file.
//
// This package only looks at the file itself. Checks against a file tree
// (globs that match nothing, the "tests" block) live elsewhere.
package config

import (
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/jdde/riskmap/internal/glob"
)

// FileName is the name every rule file has.
const FileName = ".riskmap.yml"

// Kind is the type of a dimension. It decides how several values are merged.
type Kind int

const (
	Ordered Kind = iota // one of a fixed list of values; the highest wins
	Set                 // a list of words; all are collected
	Flag                // true or false; true wins
)

// String returns the name used in the rule file.
func (k Kind) String() string {
	switch k {
	case Ordered:
		return "ordered"
	case Set:
		return "set"
	case Flag:
		return "flag"
	}
	return "unknown"
}

// Dimension is one declared label, for example "risk".
type Dimension struct {
	Name string
	Kind Kind
	// Values is only used by ordered dimensions. It lists the allowed values
	// from mildest to strictest.
	Values []string
}

// Rank returns the position of a value in an ordered dimension, or -1 if the
// value is not allowed. A higher rank is stricter.
func (d Dimension) Rank(value string) int {
	for i, v := range d.Values {
		if v == value {
			return i
		}
	}
	return -1
}

// Value is the value of one label. Which field is used depends on the kind of
// the dimension: Level for ordered, Set for set, Flag for flag.
type Value struct {
	Level string
	Set   []string // sorted, without duplicates
	Flag  bool
}

// Labels maps a dimension name to a value.
//
// Go note: a map is what Dart calls Map and PHP calls an associative array.
type Labels map[string]Value

// Rule is one entry of "rules".
type Rule struct {
	// ID is the name written in the file, or the position in the list
	// (starting at 0) if the rule has none.
	ID     string
	Match  []*glob.Pattern
	Except []*glob.Pattern
	Labels Labels
	Reason string
	Line   int // where the rule starts in the file
}

// Test is one entry of "tests": a path and the labels it must get.
type Test struct {
	Path   string
	Expect Labels
	Line   int
}

// Config is a valid rule file.
type Config struct {
	// File is the name used in messages and in the result.
	File string
	// Dimensions holds every dimension including the predefined "risk",
	// sorted by name.
	Dimensions []Dimension
	Default    Labels
	Rules      []Rule
	Tests      []Test
}

// Dimension looks up a dimension by name. The second result is false if no
// such dimension is declared.
func (c *Config) Dimension(name string) (Dimension, bool) {
	for _, d := range c.Dimensions {
		if d.Name == name {
			return d, true
		}
	}
	return Dimension{}, false
}

// Problem is one reason why a rule file is not valid.
type Problem struct {
	File    string
	Line    int // starts at 1; 0 if the problem has no position
	Column  int // starts at 1; 0 if unknown
	Message string
}

// String formats a problem the way compilers do: file:line:column: message.
func (p Problem) String() string {
	switch {
	case p.Line == 0:
		return fmt.Sprintf("%s: %s", p.File, p.Message)
	case p.Column == 0:
		return fmt.Sprintf("%s:%d: %s", p.File, p.Line, p.Message)
	}
	return fmt.Sprintf("%s:%d:%d: %s", p.File, p.Line, p.Column, p.Message)
}

// InvalidError is returned by Parse for a file that is not valid. It carries
// all problems, not only the first.
type InvalidError struct {
	Problems []Problem
}

func (e *InvalidError) Error() string {
	lines := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		lines[i] = p.String()
	}
	return strings.Join(lines, "\n")
}

// riskDimension is predefined by the specification and cannot be redefined.
var riskDimension = Dimension{Name: "risk", Kind: Ordered, Values: []string{"low", "medium", "high"}}

// RiskLevels returns the values of the predefined dimension "risk", mildest
// first. They are the same for every rule file.
func RiskLevels() []string {
	return append([]string(nil), riskDimension.Values...)
}

// Parse reads the rule file in the root of a repository. The file name is only
// used in messages.
//
// If the error is not nil it is always an *InvalidError.
func Parse(file string, data []byte) (*Config, error) {
	p := &parser{file: file, dimensions: map[string]dimension{}}
	p.dimensions["risk"] = dimension{Dimension: riskDimension}

	cfg := &Config{File: file}
	if root, ok := p.load(data); ok {
		p.parseRoot(root, cfg)
	}

	if len(p.problems) > 0 {
		// Report in the order of the file, whatever order we checked in.
		sort.SliceStable(p.problems, func(i, j int) bool {
			a, b := p.problems[i], p.problems[j]
			if a.Line != b.Line {
				return a.Line < b.Line
			}
			return a.Column < b.Column
		})
		return nil, &InvalidError{Problems: p.problems}
	}
	return cfg, nil
}

// parser collects problems while it walks the file.
type parser struct {
	file       string
	problems   []Problem
	dimensions map[string]dimension
}

// dimension is a Dimension during parsing. A declaration that had an error is
// still remembered as broken, so that labels using it do not produce a second,
// misleading "not declared" message.
type dimension struct {
	Dimension
	broken bool
}

// add records a problem at the position of a node.
//
// Go note: "args ...any" accepts any number of extra arguments, which are
// passed on to fmt.Sprintf to fill the %s and %q placeholders.
func (p *parser) add(n *yaml.Node, format string, args ...any) {
	p.addAt(n.Line, n.Column, fmt.Sprintf(format, args...))
}

func (p *parser) addAt(line, column int, message string) {
	p.problems = append(p.problems, Problem{File: p.file, Line: line, Column: column, Message: message})
}

func (p *parser) parseRoot(root *yaml.Node, cfg *Config) {
	top, ok := p.object(root, "the rule file", "version", "dimensions", "default", "rules", "tests")
	if !ok {
		return
	}

	if n, present := top["version"]; !present {
		p.add(root, `missing key "version"`)
	} else if version, ok := p.integer(n, "version"); ok && version != 1 {
		p.add(n, "unsupported version %d; this tool knows version 1", version)
	}

	// Dimensions come first: every label below is checked against them.
	if n, present := top["dimensions"]; present {
		p.parseDimensions(n)
	}
	for _, d := range p.dimensions {
		cfg.Dimensions = append(cfg.Dimensions, d.Dimension)
	}
	sort.Slice(cfg.Dimensions, func(i, j int) bool { return cfg.Dimensions[i].Name < cfg.Dimensions[j].Name })

	if n, present := top["default"]; !present {
		p.add(root, `missing key "default"`)
	} else if labels, ok := p.parseLabels(n, "default"); ok {
		if _, hasRisk := labels["risk"]; !hasRisk {
			p.add(n, `default must contain "risk"`)
		}
		cfg.Default = labels
	}

	if n, present := top["rules"]; !present {
		p.add(root, `missing key "rules"`)
	} else {
		cfg.Rules = p.parseRules(n)
	}

	if n, present := top["tests"]; present {
		cfg.Tests = p.parseTests(n)
	}
}

func (p *parser) parseDimensions(n *yaml.Node) {
	fields, ok := p.mapping(n, "dimensions")
	if !ok {
		return
	}
	for _, f := range fields {
		name := f.name
		switch {
		case typeOf(f.key) != typeString || strings.TrimSpace(name) == "":
			p.add(f.key, "a dimension name must be text")
			continue
		case name == "risk":
			p.add(f.key, `"risk" is predefined and must not be redefined`)
			continue
		case name == "review":
			p.add(f.key, `"review" is reserved for a later version and must not be declared`)
			continue
		case strings.HasPrefix(name, "x-"):
			p.add(f.key, `a dimension name must not start with "x-"; that prefix is reserved for extensions`)
			continue
		}
		d, ok := p.parseDimension(name, f.value)
		p.dimensions[name] = dimension{Dimension: d, broken: !ok}
	}
}

func (p *parser) parseDimension(name string, n *yaml.Node) (Dimension, bool) {
	what := fmt.Sprintf("dimension %q", name)
	d := Dimension{Name: name}

	keys, ok := p.object(n, what, "type", "values")
	if !ok {
		return d, false
	}
	typeNode, present := keys["type"]
	if !present {
		p.add(n, `%s: missing key "type"`, what)
		return d, false
	}
	typeName, ok := p.text(typeNode, "type")
	if !ok {
		return d, false
	}
	switch typeName {
	case "ordered":
		d.Kind = Ordered
	case "set":
		d.Kind = Set
	case "flag":
		d.Kind = Flag
	default:
		p.add(typeNode, "unknown type %q (allowed: ordered, set, flag)", typeName)
		return d, false
	}

	valuesNode, hasValues := keys["values"]
	if d.Kind != Ordered {
		if hasValues {
			p.add(valuesNode, `"values" is only allowed for type ordered`)
			return d, false
		}
		return d, true
	}
	if !hasValues {
		p.add(n, `%s: type ordered needs "values"`, what)
		return d, false
	}
	items, ok := p.list(valuesNode, "values")
	if !ok {
		return d, false
	}
	if len(items) == 0 {
		p.add(valuesNode, "values must not be empty")
		return d, false
	}
	valid := true
	for _, item := range items {
		value, ok := p.nonEmptyText(item, "a value")
		switch {
		case !ok:
			valid = false
		case contains(d.Values, value):
			p.add(item, "duplicate value %q", value)
			valid = false
		default:
			d.Values = append(d.Values, value)
		}
	}
	return d, valid
}

// parseLabels reads a mapping from dimension name to value. It is used for
// "default", for the labels of a rule and for the expectation of a test.
func (p *parser) parseLabels(n *yaml.Node, what string) (Labels, bool) {
	fields, ok := p.mapping(n, what)
	if !ok {
		return nil, false
	}
	labels := Labels{}
	valid := true
	for _, f := range fields {
		d, declared := p.dimensions[f.name]
		if !declared {
			p.add(f.key, "label %q: no such dimension is declared", f.name)
			valid = false
			continue
		}
		if d.broken {
			valid = false
			continue
		}
		value, ok := p.parseValue(f.value, d.Dimension)
		if !ok {
			valid = false
			continue
		}
		labels[f.name] = value
	}
	return labels, valid
}

func (p *parser) parseValue(n *yaml.Node, d Dimension) (Value, bool) {
	what := fmt.Sprintf("label %q", d.Name)
	switch d.Kind {
	case Ordered:
		level, ok := p.text(n, what)
		if !ok {
			return Value{}, false
		}
		if d.Rank(level) < 0 {
			p.add(n, "%s: %q is not one of %s", what, level, strings.Join(d.Values, ", "))
			return Value{}, false
		}
		return Value{Level: level}, true

	case Flag:
		flag, ok := p.boolean(n, what)
		return Value{Flag: flag}, ok

	default: // Set
		if n.Kind != yaml.SequenceNode {
			p.add(n, "%s must be a list, because the dimension has type set; write [%s]", what, n.Value)
			return Value{}, false
		}
		set := []string{}
		valid := true
		for _, item := range n.Content {
			word, ok := p.nonEmptyText(item, "a value of "+what)
			if !ok {
				valid = false
			} else if !contains(set, word) {
				set = append(set, word)
			}
		}
		sort.Strings(set)
		return Value{Set: set}, valid
	}
}

func (p *parser) parseRules(n *yaml.Node) []Rule {
	items, ok := p.list(n, "rules")
	if !ok {
		return nil
	}
	rules := make([]Rule, 0, len(items))
	// Remember where each id was first used, to report duplicates.
	firstUse := map[string]int{}

	for index, item := range items {
		what := fmt.Sprintf("rule %d", index)
		keys, ok := p.object(item, what, "id", "match", "except", "labels", "reason")
		if !ok {
			continue
		}
		rule := Rule{ID: fmt.Sprint(index), Line: item.Line}

		idNode := item
		if n, present := keys["id"]; present {
			idNode = n
			if id, ok := p.nonEmptyText(n, "id"); ok {
				rule.ID = id
				what = fmt.Sprintf("rule %q", id)
			}
		}
		if line, used := firstUse[rule.ID]; used {
			p.add(idNode, "duplicate rule id %q, first used in line %d", rule.ID, line)
		} else {
			firstUse[rule.ID] = idNode.Line
		}

		// A glob may appear only once in a rule, across match and except.
		seen := map[string]bool{}
		if n, present := keys["match"]; !present {
			p.add(item, `%s: missing key "match"`, what)
		} else {
			rule.Match = p.parseGlobs(n, "match", seen)
			if n.Kind == yaml.SequenceNode && len(n.Content) == 0 {
				p.add(n, "match must not be empty")
			}
		}
		if n, present := keys["except"]; present {
			rule.Except = p.parseGlobs(n, "except", seen)
		}

		if n, present := keys["labels"]; !present {
			p.add(item, `%s: missing key "labels"`, what)
		} else if labels, ok := p.parseLabels(n, "labels"); ok {
			if len(n.Content) == 0 {
				p.add(n, "labels must contain at least one label")
			}
			rule.Labels = labels
		}

		if n, present := keys["reason"]; !present {
			p.add(item, `%s: missing key "reason"`, what)
		} else if reason, ok := p.nonEmptyText(n, "reason"); ok {
			rule.Reason = reason
		}

		rules = append(rules, rule)
	}
	return rules
}

// parseGlobs reads a list of globs. seen holds the globs already used in the
// same rule.
func (p *parser) parseGlobs(n *yaml.Node, what string, seen map[string]bool) []*glob.Pattern {
	items, ok := p.list(n, what)
	if !ok {
		return nil
	}
	patterns := make([]*glob.Pattern, 0, len(items))
	for _, item := range items {
		source, ok := p.text(item, "a glob")
		if !ok {
			continue
		}
		pattern, err := glob.Compile(source)
		if err != nil {
			p.add(item, "%s", err.Error())
			continue
		}
		if seen[source] {
			p.add(item, "glob %q is listed twice in this rule", source)
			continue
		}
		seen[source] = true
		patterns = append(patterns, pattern)
	}
	return patterns
}

func (p *parser) parseTests(n *yaml.Node) []Test {
	items, ok := p.list(n, "tests")
	if !ok {
		return nil
	}
	tests := make([]Test, 0, len(items))
	for index, item := range items {
		what := fmt.Sprintf("test %d", index)
		keys, ok := p.object(item, what, "path", "expect")
		if !ok {
			continue
		}
		test := Test{Line: item.Line}

		if n, present := keys["path"]; !present {
			p.add(item, `%s: missing key "path"`, what)
		} else if path, ok := p.nonEmptyText(n, "path"); ok {
			test.Path = path
		}

		if n, present := keys["expect"]; !present {
			p.add(item, `%s: missing key "expect"`, what)
		} else if labels, ok := p.parseLabels(n, "expect"); ok {
			test.Expect = labels
		}

		tests = append(tests, test)
	}
	return tests
}
