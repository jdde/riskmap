// Package eval classifies a set of paths with a rule file. It implements
// sections 5 and 7 of SPEC.md.
//
// The central idea is merging: every rule that matches a path contributes its
// labels, and for each dimension the strictest value wins. There is no
// "first match", so the order of rules and paths never changes the result.
package eval

import (
	"bytes"
	"sort"
	"unicode/utf8"

	"github.com/jdde/riskmap/internal/config"
)

// Status values of a result (section 6).
const (
	StatusOK         = "ok"
	StatusIncomplete = "incomplete"
)

// Kinds of trace entries (section 6).
const (
	KindRule            = "rule"
	KindDefault         = "default"
	KindBuiltin         = "builtin"
	KindEmptyChange     = "empty-change"
	KindIncompleteInput = "incomplete-input"
	KindUnreadablePath  = "unreadable-path"
)

// BuiltinRuleID is the name of the built-in rule that protects rule files.
const BuiltinRuleID = "riskmap-config"

// Because is one trace entry: one reason why a path or a change got its
// labels. Fields that do not apply to a kind stay empty.
type Because struct {
	Kind   string
	Config string // rule file the rule comes from
	Rule   string // rule id
	Glob   string // the glob of the rule that matched
	Reason string // the "reason" text of the rule
}

// PathResult is the result for a single path.
type PathResult struct {
	Path    []byte
	Labels  config.Labels
	Because []Because
}

// Result is the result for a whole change.
type Result struct {
	Status string
	Labels config.Labels
	// Because holds reasons that apply to the change as a whole and not to a
	// single path: an empty or an incomplete set of paths.
	Because []Because
	Paths   []PathResult
}

// Evaluate classifies the paths of one change.
//
// complete says whether the caller can vouch that the list holds every changed
// path. If it cannot, the result is the strictest one possible.
//
// Go note: [][]byte is a list of byte strings. Paths are bytes and not text
// because a file name is not guaranteed to be valid UTF-8.
func Evaluate(cfg *config.Config, paths [][]byte, complete bool) *Result {
	result := &Result{Status: StatusOK, Labels: config.Labels{}}

	for _, path := range sortedUnique(paths) {
		pathResult := evaluatePath(cfg, path)
		merge(cfg, result.Labels, pathResult.Labels)
		result.Paths = append(result.Paths, pathResult)
	}

	switch {
	case !complete:
		// Whatever the known paths say, the unknown ones could be worse.
		result.Status = StatusIncomplete
		result.Labels = Strictest(cfg)
		result.Because = []Because{{Kind: KindIncompleteInput}}
	case len(result.Paths) == 0:
		// An empty change gets the default, never something milder.
		merge(cfg, result.Labels, cfg.Default)
		result.Because = []Because{{Kind: KindEmptyChange}}
	}

	fillEmpty(cfg, result.Labels)
	return result
}

// EvaluatePath classifies a single path. It is what the "tests" block of a
// rule file runs against.
func EvaluatePath(cfg *config.Config, path []byte) PathResult {
	return evaluatePath(cfg, path)
}

func evaluatePath(cfg *config.Config, path []byte) PathResult {
	result := PathResult{Path: path, Labels: config.Labels{}}

	// A path we cannot represent cannot be matched reliably. It gets the
	// strictest result instead of slipping through.
	if !utf8.Valid(path) || bytes.IndexByte(path, 0) >= 0 || len(path) == 0 {
		result.Labels = Strictest(cfg)
		result.Because = []Because{{Kind: KindUnreadablePath}}
		return result
	}

	// Step 1: every rule with a matching glob and no matching exception.
	for _, rule := range cfg.Rules {
		glob, matched := matchRule(rule, path)
		if !matched {
			continue
		}
		merge(cfg, result.Labels, rule.Labels)
		result.Because = append(result.Because, Because{
			Kind: KindRule, Config: cfg.File, Rule: rule.ID, Glob: glob, Reason: rule.Reason,
		})
	}
	// The trace must not depend on the order of the rules in the file.
	sort.Slice(result.Because, func(i, j int) bool { return result.Because[i].Rule < result.Because[j].Rule })

	// Step 3: without a matching rule the default applies.
	if len(result.Because) == 0 {
		merge(cfg, result.Labels, cfg.Default)
		result.Because = append(result.Because, Because{Kind: KindDefault, Config: cfg.File})
	}

	// Step 2: the built-in rule. It comes on top and never replaces the
	// default.
	if isRuleFile(path) {
		merge(cfg, result.Labels, selfProtection(cfg))
		result.Because = append(result.Because, Because{
			Kind: KindBuiltin, Rule: BuiltinRuleID, Reason: "rule files are always in the strictest level",
		})
	}

	fillEmpty(cfg, result.Labels)
	return result
}

// matchRule reports whether a rule applies to a path and which of its globs
// matched first.
func matchRule(rule config.Rule, path []byte) (string, bool) {
	for _, except := range rule.Except {
		if except.Match(path) {
			return "", false
		}
	}
	for _, match := range rule.Match {
		if match.Match(path) {
			return match.String(), true
		}
	}
	return "", false
}

// isRuleFile reports whether the path is a .riskmap.yml in any directory.
func isRuleFile(path []byte) bool {
	name := []byte(config.FileName)
	return bytes.Equal(path, name) || bytes.HasSuffix(path, append([]byte("/"), name...))
}

// merge adds the labels in "from" to "into". For each dimension the stricter
// value wins: the higher level, the union of the sets, true over false.
//
// Go note: a map is passed by reference, so changing "into" here changes the
// map of the caller.
func merge(cfg *config.Config, into, from config.Labels) {
	for _, d := range cfg.Dimensions {
		add, present := from[d.Name]
		if !present {
			continue
		}
		have, had := into[d.Name]
		if !had {
			// Copy the set so that later merges cannot change the rule.
			add.Set = append([]string(nil), add.Set...)
			into[d.Name] = add
			continue
		}
		switch d.Kind {
		case config.Ordered:
			if d.Rank(add.Level) > d.Rank(have.Level) {
				have.Level = add.Level
			}
		case config.Set:
			have.Set = union(have.Set, add.Set)
		case config.Flag:
			have.Flag = have.Flag || add.Flag
		}
		into[d.Name] = have
	}
}

// union returns the sorted values of both lists without duplicates.
func union(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, list := range [][]string{a, b} {
		for _, value := range list {
			if !seen[value] {
				seen[value] = true
				out = append(out, value)
			}
		}
	}
	sort.Strings(out)
	return out
}

// fillEmpty gives every set and flag dimension a value: the empty set and
// false. An ordered dimension without a value stays absent, because it has no
// value that means "nothing".
func fillEmpty(cfg *config.Config, labels config.Labels) {
	for _, d := range cfg.Dimensions {
		if _, present := labels[d.Name]; present {
			continue
		}
		switch d.Kind {
		case config.Set:
			labels[d.Name] = config.Value{Set: []string{}}
		case config.Flag:
			labels[d.Name] = config.Value{Flag: false}
		}
	}
}

// Strictest returns the strictest value of every dimension (section 2.2): the
// last level, true, and for a set every value the rule file mentions in
// "default" or in a rule.
func Strictest(cfg *config.Config) config.Labels {
	labels := selfProtection(cfg)
	for _, d := range cfg.Dimensions {
		if d.Kind != config.Set {
			continue
		}
		all := append([]string(nil), cfg.Default[d.Name].Set...)
		for _, rule := range cfg.Rules {
			all = union(all, rule.Labels[d.Name].Set)
		}
		labels[d.Name] = config.Value{Set: union(all, nil)}
	}
	return labels
}

// selfProtection returns the labels of the built-in rule for rule files
// (section 7): the strictest value of every ordered and flag dimension. Set
// dimensions get the values of the default.
func selfProtection(cfg *config.Config) config.Labels {
	labels := config.Labels{}
	for _, d := range cfg.Dimensions {
		switch d.Kind {
		case config.Ordered:
			labels[d.Name] = config.Value{Level: d.Values[len(d.Values)-1]}
		case config.Flag:
			labels[d.Name] = config.Value{Flag: true}
		case config.Set:
			if value, present := cfg.Default[d.Name]; present {
				labels[d.Name] = config.Value{Set: append([]string(nil), value.Set...)}
			}
		}
	}
	return labels
}

// sortedUnique returns the paths in byte order without duplicates, so that the
// order of the input never shows in the output.
func sortedUnique(paths [][]byte) [][]byte {
	sorted := append([][]byte(nil), paths...)
	sort.Slice(sorted, func(i, j int) bool { return bytes.Compare(sorted[i], sorted[j]) < 0 })
	out := sorted[:0]
	for i, path := range sorted {
		if i == 0 || !bytes.Equal(path, sorted[i-1]) {
			out = append(out, path)
		}
	}
	return out
}
