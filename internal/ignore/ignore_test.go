package ignore

import (
	"strings"
	"testing"
)

func TestMatcher(t *testing.T) {
	rules := `
# A comment
config.yaml
*.secret
/exact.txt
build/
app/config/*.json
`
	matcher, err := NewMatcher(strings.NewReader(rules))
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
	}{
		// match "config.yaml" anywhere
		{"config.yaml", true},
		{"src/config.yaml", true},
		{"a/b/c/config.yaml", true},
		{"config.yaml.bak", false},

		// match "*.secret" anywhere
		{"my.secret", true},
		{"keys/prod.secret", true},
		{"secret.txt", false},

		// match "/exact.txt" only at root
		{"exact.txt", true},
		{"subdir/exact.txt", false},

		// match "build/" directory and anything inside
		{"build/output.bin", true},
		{"build/temp/log.txt", true},
		{"src/build/output.bin", true}, // "build/" matches anywhere.

		// match "app/config/*.json"
		{"app/config/settings.json", true},
		{"app/config/deep/settings.json", false}, // filepath.Match doesn't match cross-dir
		{"other/app/config/settings.json", false},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			matched, rule := matcher.Match(tc.path)
			if matched != tc.expected {
				t.Errorf("Path %q: expected match=%v, got match=%v (rule=%q)", tc.path, tc.expected, matched, rule)
			}
		})
	}
}

func TestMatcherDirectoryAnywhere(t *testing.T) {
	rules := `
build/
`
	matcher, err := NewMatcher(strings.NewReader(rules))
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"build/output.bin", true},
		{"src/build/output.bin", true},
	}

	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			matched, _ := matcher.Match(tc.path)
			if matched != tc.expected {
				t.Errorf("Path %q: expected match=%v, got match=%v", tc.path, tc.expected, matched)
			}
		})
	}
}
