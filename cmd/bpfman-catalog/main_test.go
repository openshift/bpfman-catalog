package main

import (
	"path/filepath"
	"testing"
)

// TestOutputDirValidation tests that we never allow the current working directory
// to be used as the output directory, preventing accidental deletion of project files.
func TestOutputDirValidation(t *testing.T) {
	tests := []struct {
		name      string
		outputDir string
		wantError bool
		reason    string
	}{
		{
			name:      "reject current directory dot",
			outputDir: ".",
			wantError: true,
			reason:    "current directory '.' must be rejected",
		},
		{
			name:      "reject current directory with slash",
			outputDir: "./",
			wantError: true,
			reason:    "current directory with trailing slash must be rejected",
		},
		{
			name:      "reject relative path to current directory",
			outputDir: "./.",
			wantError: true,
			reason:    "relative path resolving to current directory must be rejected",
		},
		{
			name:      "reject relative path with parent that resolves to current",
			outputDir: "foo/..",
			wantError: true,
			reason:    "relative path with .. that resolves to current directory must be rejected",
		},
		{
			name:      "accept subdirectory",
			outputDir: DefaultArtefactsDir,
			wantError: false,
			reason:    "subdirectory should be accepted",
		},
		{
			name:      "accept simple subdirectory name",
			outputDir: "artefacts",
			wantError: false,
			reason:    "simple subdirectory name should be accepted",
		},
		{
			name:      "accept nested subdirectory",
			outputDir: "./foo/bar",
			wantError: false,
			reason:    "nested subdirectory should be accepted",
		},
		{
			name:      "accept absolute path",
			outputDir: "/tmp/test-output",
			wantError: false,
			reason:    "absolute path should be accepted",
		},
		{
			name:      "accept subdirectory with parent traversal",
			outputDir: "./foo/../bar",
			wantError: false,
			reason:    "subdirectory with parent traversal should be accepted if it doesn't resolve to .",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This is the validation logic from the commands
			cleaned := filepath.Clean(tt.outputDir)
			isCurrentDir := cleaned == "."

			if tt.wantError && !isCurrentDir {
				t.Errorf("%s: expected validation to reject %q (cleans to %q), but it was accepted",
					tt.reason, tt.outputDir, cleaned)
			}
			if !tt.wantError && isCurrentDir {
				t.Errorf("%s: expected validation to accept %q (cleans to %q), but it was rejected",
					tt.reason, tt.outputDir, cleaned)
			}
		})
	}
}

// TestFilePathCleaningBehaviour documents and tests filepath.Clean behaviour for our use case.
func TestFilePathCleaningBehaviour(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{".", "."},
		{"./", "."},
		{"./.", "."},
		{".//", "."},
		{"foo/..", "."},
		{"./foo/../", "."},
		{DefaultArtefactsDir, DefaultArtefactsDir},
		{"artefacts", "artefacts"},
		{"./foo/bar", "foo/bar"},
		{"/tmp/test", "/tmp/test"},
		{"./foo/../bar", "bar"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := filepath.Clean(tt.input)
			if result != tt.expected {
				t.Errorf("filepath.Clean(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
