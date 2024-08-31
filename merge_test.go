package butanex

import (
	"github.com/google/go-cmp/cmp"
	yaml "gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"slices"
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
		files  []string
	}{
		{
			name: "simple",
			config: &Options{
				FilesDir: "./simple",
			},
			files: []string{
				"input1.yaml",
				"input2.yaml",
			},
		},
		{
			name: "overwrite",
			config: &Options{
				FilesDir:         "./overwrite",
				DefaultOverWrite: true,
			},
			files: []string{
				"input1.yaml",
				"input2.yaml",
			},
		},
		{
			name: "resolve-path",
			config: &Options{
				DefaultOverWrite: true,
				FilesDir:         "./resolve-path",
				ResolvePath: []string{
					".local",
				},
			},
			files: []string{
				"common/input1.yaml",
				"host-dir/input2.yaml",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(tc.name, "want.yaml"))
			if err != nil {
				t.Fatalf("error reading want file: %s", err)
			}
			files := slices.Clone(tc.files)
			for i, name := range files {
				files[i] = filepath.Join(tc.name, name)
			}
			got, err := MergeFiles(tc.config, tc.files...)
			if err != nil {
				t.Fatalf("Error merging files: %s", err)
			}
			if diff := cmp.Diff(mustUnmarshal(t, want), mustUnmarshal(t, got)); diff != "" {
				t.Errorf("MergeFiles() got diff: -want/+got: %s", diff)
				t.Logf("GOT:\n%s\n", got)
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
