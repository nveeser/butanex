package butanex

import (
	"github.com/google/go-cmp/cmp"
	yaml "gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"testing"
)

// Simple Merge
// Overwrite node
// Overwrite sequence
// Resolve paths

func TestMergeFiles(t *testing.T) {
	cases := []struct {
		name   string
		config *Options
	}{
		{
			name:   "simple",
			config: nil,
		},
		{
			name: "overwrite",
			config: &Options{
				DefaultOverWrite: true,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(tc.name, "want.yaml"))
			if err != nil {
				t.Fatalf("error reading want file: %s", err)
			}
			got, err := MergeFiles(tc.config, filepath.Join(tc.name, "input1.yaml"), filepath.Join(tc.name, "input2.yaml"))
			if err != nil {
				t.Errorf("Error merging files: %s", err)
			}
			if diff := cmp.Diff(mustUnmarshal(t, want), mustUnmarshal(t, got)); diff != "" {
				t.Errorf("MergeFiles() got diff: -want/+got: %s", diff)
			}
		})
	}
}

func mustUnmarshal(t *testing.T, d []byte) map[string]any {
	t.Helper()
	m := make(map[string]any)
	if err := yaml.Unmarshal(d, &m); err != nil {
		t.Fatalf("yaml.Unmarshal() got err: %s", err)
	}
	return m
}

func TestMergePolicy(t *testing.T) {
	cases := []struct {
		name    string
		config  *Options
		ctxpath string
		want    bool
	}{
		{
			name: "absolute-path/match",
			config: &Options{
				Overwrite: []string{"$.storage.files"},
			},
			ctxpath: "$.storage.files",
			want:    true,
		},
		{
			name: "absolute-path/no-match",
			config: &Options{
				Overwrite: []string{"$.passwd.users"},
			},
			ctxpath: "$.storage.files",
			want:    false,
		},
		{
			name: "relative-path/match",
			config: &Options{
				Overwrite: []string{".files"},
			},
			ctxpath: "$.storage.files",
			want:    true,
		},
		{
			name: "relative-path/no-match",
			config: &Options{
				Overwrite: []string{".local"},
			},
			ctxpath: "$.storage.files",
			want:    false,
		},
		{
			name: "both-types/absolute-wins",
			config: &Options{
				Overwrite: []string{".local"},
				Append:    []string{"$.storage.files.local"},
			},
			ctxpath: "$.storage.files.local",
			want:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := buildPolicy(tc.config)
			got := m.isOverwrite(tc.ctxpath)
			if got != tc.want {
				t.Errorf("getPolicy() got %t wanted %t", got, tc.want)
			}
		})
	}
}
