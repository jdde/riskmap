package config

import (
	"bytes"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// This file holds everything that deals with YAML itself: loading the text
// into a tree of nodes, rejecting the YAML features the specification forbids,
// and deciding what type a scalar has.
//
// We work on the node tree (yaml.Node) instead of letting the library fill Go
// structs. Only the tree shows anchors, aliases, tags and the line and column
// of every value.

// load parses the text into a node tree and returns the top-level node of the
// single document in it. Problems are reported through p.
//
// Go note: a function can return several values. Here the second one says
// whether the first one is usable.
func (p *parser) load(data []byte) (*yaml.Node, bool) {
	if !utf8.Valid(data) {
		p.addAt(0, 0, "file is not valid UTF-8")
		return nil, false
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))

	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			p.addAt(0, 0, "file is empty")
		} else {
			p.addSyntaxError(err)
		}
		return nil, false
	}

	// A YAML stream may hold several documents separated by "---". A rule file
	// is exactly one.
	var second yaml.Node
	switch err := decoder.Decode(&second); {
	case err == nil:
		p.addAt(second.Line, second.Column, "file contains more than one YAML document")
		return nil, false
	case !errors.Is(err, io.EOF):
		p.addSyntaxError(err)
		return nil, false
	}

	// The library wraps the content in a document node with one child.
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		p.addAt(0, 0, "file is empty")
		return nil, false
	}
	root := doc.Content[0]

	before := len(p.problems)
	p.checkPlainYAML(root)
	return root, len(p.problems) == before
}

// yamlLine finds the line number in an error message of the YAML library,
// which looks like "yaml: line 3: mapping values are not allowed here".
var yamlLine = regexp.MustCompile(`^yaml: line (\d+): (.*)$`)

func (p *parser) addSyntaxError(err error) {
	message := err.Error()
	if m := yamlLine.FindStringSubmatch(message); m != nil {
		line, _ := strconv.Atoi(m[1])
		p.addAt(line, 0, "YAML syntax error: "+m[2])
		return
	}
	p.addAt(0, 0, "YAML syntax error: "+strings.TrimPrefix(message, "yaml: "))
}

// checkPlainYAML walks the whole tree and reports what the specification
// forbids in section 2: anchors, aliases and tags. It also reports duplicate
// keys and keys that are not plain text, because the library accepts both.
func (p *parser) checkPlainYAML(n *yaml.Node) {
	if n.Kind == yaml.AliasNode {
		p.add(n, "aliases (*name) are not allowed")
		return // do not walk into the node the alias points to
	}
	if n.Anchor != "" {
		p.add(n, "anchors (&name) are not allowed")
	}
	if n.Style&yaml.TaggedStyle != 0 {
		p.add(n, "tags (%s) are not allowed", n.Tag)
	}

	if n.Kind == yaml.MappingNode {
		// The children of a mapping are key, value, key, value, ...
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode {
				p.add(key, "a key must be plain text, not a list or mapping")
				continue
			}
			if seen[key.Value] {
				p.add(key, "duplicate key %q", key.Value)
			}
			seen[key.Value] = true
		}
	}

	for _, child := range n.Content {
		p.checkPlainYAML(child)
	}
}

// scalarType is the type of a scalar under the YAML 1.2 core schema.
type scalarType int

const (
	typeString scalarType = iota
	typeNull
	typeBool
	typeInt
	typeFloat
)

// The patterns of the YAML 1.2 core schema (section 10.3.2 of the YAML
// specification). We apply them ourselves because the library follows older
// rules for some values: it reads 012 as the octal number 10 and 1_000 as the
// number 1000. Under YAML 1.2, 012 is the number 12 and 1_000 is text.
var (
	coreNull  = regexp.MustCompile(`^(null|Null|NULL|~|)$`)
	coreBool  = regexp.MustCompile(`^(true|True|TRUE|false|False|FALSE)$`)
	coreInt   = regexp.MustCompile(`^([-+]?[0-9]+|0o[0-7]+|0x[0-9a-fA-F]+)$`)
	coreFloat = regexp.MustCompile(`^([-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?|[-+]?\.(inf|Inf|INF)|\.(nan|NaN|NAN))$`)
)

// typeOf returns the type of a scalar node. Quoted and block scalars are
// always text. Unquoted scalars are typed by their content.
func typeOf(n *yaml.Node) scalarType {
	const textStyles = yaml.DoubleQuotedStyle | yaml.SingleQuotedStyle | yaml.LiteralStyle | yaml.FoldedStyle
	if n.Style&textStyles != 0 {
		return typeString
	}
	switch {
	case coreNull.MatchString(n.Value):
		return typeNull
	case coreBool.MatchString(n.Value):
		return typeBool
	case coreInt.MatchString(n.Value):
		return typeInt
	case coreFloat.MatchString(n.Value):
		return typeFloat
	}
	return typeString
}

// describe names what a node is, for error messages.
func describe(n *yaml.Node) string {
	switch n.Kind {
	case yaml.MappingNode:
		return "a mapping"
	case yaml.SequenceNode:
		return "a list"
	case yaml.ScalarNode:
		switch typeOf(n) {
		case typeNull:
			return "empty"
		case typeBool:
			return "a boolean"
		case typeInt, typeFloat:
			return "a number"
		}
		return "text"
	}
	return "something else"
}

// text returns the value of a node that must be text.
//
// A value such as 123, true or null is not accepted as text. The author has
// to quote it. This avoids the classic YAML surprise where an unquoted value
// silently becomes a number or a boolean.
func (p *parser) text(n *yaml.Node, what string) (string, bool) {
	if n.Kind != yaml.ScalarNode {
		p.add(n, "%s must be text, but is %s", what, describe(n))
		return "", false
	}
	if typeOf(n) != typeString {
		if n.Value == "" {
			p.add(n, "%s must be text, but is empty", what)
		} else {
			p.add(n, "%s must be text, but %s is %s; put it in quotes", what, n.Value, describe(n))
		}
		return "", false
	}
	return n.Value, true
}

// nonEmptyText is text that contains more than white space.
func (p *parser) nonEmptyText(n *yaml.Node, what string) (string, bool) {
	value, ok := p.text(n, what)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(value) == "" {
		p.add(n, "%s must not be empty", what)
		return "", false
	}
	return value, true
}

// boolean returns the value of a node that must be true or false.
func (p *parser) boolean(n *yaml.Node, what string) (bool, bool) {
	if n.Kind != yaml.ScalarNode || typeOf(n) != typeBool {
		p.add(n, "%s must be true or false, but is %s", what, describe(n))
		return false, false
	}
	return strings.EqualFold(n.Value, "true"), true
}

// integer returns the value of a node that must be a whole number.
func (p *parser) integer(n *yaml.Node, what string) (int64, bool) {
	if n.Kind != yaml.ScalarNode || typeOf(n) != typeInt {
		p.add(n, "%s must be a whole number, but is %s", what, describe(n))
		return 0, false
	}
	var value int64
	var err error
	switch {
	case strings.HasPrefix(n.Value, "0o"):
		value, err = strconv.ParseInt(n.Value[2:], 8, 64)
	case strings.HasPrefix(n.Value, "0x"):
		value, err = strconv.ParseInt(n.Value[2:], 16, 64)
	default:
		// Base 10 on purpose: a leading zero does not mean octal in YAML 1.2.
		value, err = strconv.ParseInt(n.Value, 10, 64)
	}
	if err != nil {
		p.add(n, "%s is out of range", what)
		return 0, false
	}
	return value, true
}

// list returns the items of a node that must be a list.
func (p *parser) list(n *yaml.Node, what string) ([]*yaml.Node, bool) {
	if n.Kind != yaml.SequenceNode {
		p.add(n, "%s must be a list, but is %s", what, describe(n))
		return nil, false
	}
	return n.Content, true
}

// field is one key of a mapping together with its value.
type field struct {
	name  string
	key   *yaml.Node
	value *yaml.Node
}

// mapping returns the entries of a node that must be a mapping, in the order
// they were written.
func (p *parser) mapping(n *yaml.Node, what string) ([]field, bool) {
	if n.Kind != yaml.MappingNode {
		p.add(n, "%s must be a mapping, but is %s", what, describe(n))
		return nil, false
	}
	fields := make([]field, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := n.Content[i]
		// Keys that are not scalars were already reported by checkPlainYAML,
		// and loading stops after that, so every key here is a scalar.
		fields = append(fields, field{name: key.Value, key: key, value: n.Content[i+1]})
	}
	return fields, true
}

// object reads a mapping with a fixed set of keys. Keys that start with "x-"
// are reserved for extensions and skipped. Any other unknown key is an error.
// The result maps each known key that is present to its value.
func (p *parser) object(n *yaml.Node, what string, known ...string) (map[string]*yaml.Node, bool) {
	fields, ok := p.mapping(n, what)
	if !ok {
		return nil, false
	}
	values := map[string]*yaml.Node{}
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f.name, "x-"):
			// extension, ignored
		case contains(known, f.name):
			values[f.name] = f.value
		default:
			p.add(f.key, "unknown key %q in %s (allowed: %s)", f.name, what, strings.Join(known, ", "))
		}
	}
	return values, true
}

func contains(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}
