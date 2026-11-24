package input

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// Git runs git commands in one directory.
//
// We start the git program instead of using a Go library for git. Its output
// with "-z" is the reference for how paths are written, and it keeps the
// number of dependencies small.
type Git struct {
	// Dir is the directory that holds the rule file. It may be a
	// subdirectory of the repository. All paths are relative to it.
	Dir string
}

// run starts git and returns what it printed. If git fails, the error holds
// what git wrote to its error output.
func (g Git) run(args ...string) ([]byte, error) {
	// -C makes git behave as if it was started in Dir.
	cmd := exec.Command("git", append([]string{"-C", g.Dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("git %s: %s", args[0], message)
		}
		return nil, fmt.Errorf("git %s: %w", args[0], err)
	}
	return out, nil
}

// Commit returns the full hash of the commit a ref (branch, tag, hash) points
// to.
func (g Git) Commit(ref string) (string, error) {
	// "--end-of-options" keeps a ref that starts with "-" from being read as
	// an option.
	out, err := g.run("rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%q is not a commit in this repository", ref)
	}
	return strings.TrimSpace(string(out)), nil
}

// MergeBase returns the commit where head branched off from base. This is the
// state a pull request is compared against.
func (g Git) MergeBase(base, head string) (string, error) {
	out, err := g.run("merge-base", base, head)
	if err != nil {
		return "", fmt.Errorf("no common ancestor of %s and %s; in a shallow clone, fetch more history (e.g. fetch-depth: 0)", base, head)
	}
	return strings.TrimSpace(string(out)), nil
}

// ChangedPaths returns the paths that differ between two commits.
func (g Git) ChangedPaths(from, to string) ([][]byte, error) {
	return g.diff(from, to)
}

// StagedPaths returns the paths that differ between HEAD and the staging
// area, which is what the next commit will contain.
func (g Git) StagedPaths() ([][]byte, error) {
	return g.diff("--cached")
}

func (g Git) diff(what ...string) ([][]byte, error) {
	args := []string{
		"diff",
		// Without rename detection a rename shows up as one deleted and one
		// added file, so both the old and the new path are listed.
		"--no-renames",
		"--name-only",
		// NUL-separated and never quoted.
		"-z",
		// Only paths below Dir, written relative to it.
		"--relative",
		// Ignore the user's configuration of external diff tools.
		"--no-ext-diff",
	}
	args = append(args, what...)
	args = append(args, "--")
	out, err := g.run(args...)
	if err != nil {
		return nil, err
	}
	return SplitNUL(out), nil
}

// ErrNoFile is returned by ReadFile if the commit has no such file.
var ErrNoFile = errors.New("file does not exist in this commit")

// ReadFile returns the content a file in Dir had in a commit.
func (g Git) ReadFile(commit, name string) ([]byte, error) {
	// ls-tree prints one entry if the file exists and nothing if not.
	listing, err := g.run("ls-tree", "-z", commit, "--", name)
	if err != nil {
		return nil, err
	}
	if len(listing) == 0 {
		return nil, ErrNoFile
	}
	// "commit:./name" reads the file relative to Dir.
	return g.run("show", commit+":./"+name)
}

// SplitNUL splits a NUL-separated list of paths. A NUL after the last path,
// as git writes it, does not start another path.
func SplitNUL(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	data = bytes.TrimSuffix(data, []byte{0})
	return bytes.Split(data, []byte{0})
}
