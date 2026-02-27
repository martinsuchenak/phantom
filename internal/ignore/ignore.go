package ignore

import (
	"bufio"
	"io"
	"path/filepath"
	"strings"
)

// Matcher represents a set of rules parsed from a .phantomignore file.
type Matcher struct {
	rules []string
}

// NewMatcher parses ignore rules from the provided reader.
func NewMatcher(r io.Reader) (*Matcher, error) {
	var rules []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rules = append(rules, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return &Matcher{rules: rules}, nil
}

// Rules returns the parsed rules.
func (m *Matcher) Rules() []string {
	return m.rules
}

// Match checks if the given relative path matches any of the parsed rules.
// It returns true and the matched rule if a match is found.
// The input path should be relative to the repository root.
func (m *Matcher) Match(path string) (bool, string) {
	path = filepath.ToSlash(path)

	for _, rule := range m.rules {
		if matchRule(rule, path) {
			return true, rule
		}
	}
	return false, ""
}

func matchRule(rule, path string) bool {
	// 1. Anchored to root
	anchored := strings.HasPrefix(rule, "/")
	if anchored {
		rule = strings.TrimPrefix(rule, "/")
	}

	// 2. Directory rule
	isDirRule := strings.HasSuffix(rule, "/")
	if isDirRule {
		rule = strings.TrimSuffix(rule, "/")
	}

	// 3. Match logic
	// If anchored or contains slash, match from the root
	if anchored || strings.Contains(rule, "/") {
		matched, _ := filepath.Match(rule, path)
		if matched {
			return true
		}

		// Also check if it matches a parent directory
		if strings.HasPrefix(path, rule+"/") {
			return true
		}
	} else {
		// Not anchored, no slashes -> match anywhere in the tree
		// Match against the base filename
		base := filepath.Base(path)
		matched, _ := filepath.Match(rule, base)
		if matched {
			return true
		}

		// Match against any parent directory component
		parts := strings.Split(path, "/")
		for i := 0; i < len(parts)-1; i++ {
			matched, _ := filepath.Match(rule, parts[i])
			if matched {
				return true
			}
		}
	}

	return false
}
