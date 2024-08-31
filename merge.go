// Package butanex contains a demo of how to merge
// multiple butane YAML files together.
//
// At this stage its more a demo of the challenges and corner cases for building
// a single Butane YAML file from a collection of files and demonstrating the
// ambiguity in how to handle overlapping keys between two files.
package butanex

import (
	"cmp"
	"fmt"
	yaml "gopkg.in/yaml.v3"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
)

// Options configure merge behavior for a given key within a YAML mapping node
// (ie a struct field).
//
// Each of the slice fields (ResolvePath, Overwrite, Append) should contain 0 or
// more patterns to apply this behavior to specific keys in a given YAML object.
// Each pattern is a string that matches a context path for example
// `$.storage.files.path`. A pattern can be relative or absolute. A relative
// pattern matches any context path with the same suffix. An absolute pattern
// matches the whole context key. Precedence for patterns is absolute, then
// relative then default.
type Options struct {
	FilesDir    string
	ResolvePath []string

	DefaultOverWrite bool
	Overwrite        []string
	Append           []string
}

// MergeFiles will merge each of the YAML files specified into single
// array of bytes of yaml intended to be passed directly to Butane transformation.
func MergeFiles(options *Options, path ...string) ([]byte, error) {
	if options == nil {
		options = &Options{}
	}
	m := &merge{
		filesDir:    options.FilesDir,
		mergePolicy: buildPolicy(options),
	}
	for _, f := range path {
		if err := m.mergeFile(f); err != nil {
			return nil, fmt.Errorf("file[%s]: %w", path, err)
		}
	}
	return yaml.Marshal(m.root)
}

type merge struct {
	*mergePolicy
	filesDir string
	root     map[string]any
}

func (m *merge) mergeFile(path string) error {
	d, err := os.ReadFile(filepath.Join(m.filesDir, path))
	if err != nil {
		return fmt.Errorf("error file[%s]: %w", path, err)
	}
	if err := m.mergeBytes(filepath.Dir(path), d); err != nil {
		return fmt.Errorf("error during Merge[%s]: %w", path, err)
	}
	return nil
}

func (m *merge) mergeBytes(fileRoot string, data []byte) error {
	config := map[string]any{}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("error reading yaml: %w", err)
	}

	if fileRoot != "" {
		m.resolvePaths(config, fileRoot, "$")
	}
	if m.root == nil {
		m.root = config
		return nil
	}
	if err := m.mergeMapping(m.root, config, "$"); err != nil {
		return err
	}
	return nil
}

func (m *merge) resolvePaths(object map[string]any, fileRoot, ctxpath string) {
	for k, v := range object {
		cpath := ctxpath + "." + k
		if vv, ok := m.resolvePathsValue(v, fileRoot, cpath); ok {
			object[k] = vv
		}
	}
}

func (m *merge) resolvePathsValue(v any, fileRoot, ctxpath string) (any, bool) {
	switch v := v.(type) {
	// Sequence
	case []any:
		var updated []any
		for _, vi := range v {
			if upv, ok := m.resolvePathsValue(vi, fileRoot, ctxpath); ok {
				updated = append(updated, upv)
			}
		}
		// only return true if all values in v were updated
		return updated, len(updated) == len(v)

	// Mapping
	case map[string]any:
		m.resolvePaths(v, fileRoot, ctxpath)

	// Scalar
	case string:
		if m.resolvePath(ctxpath) {
			vv := filepath.Join(fileRoot, v)
			log.Printf("\t Update[%s] %s -> %s", ctxpath, v, vv)
			return vv, true
		}
	}
	return nil, false
}

func (m *merge) mergeMapping(dst, src map[string]any, ctxpath string) error {
	for key, sv := range src {
		cpath := ctxpath + "." + key
		switch sv := sv.(type) {
		// Sequence
		case []any:
			dv, exists := dst[key]
			dvv, isSlice := dv.([]any) // if exists=false, then dv=nil and isSlice=false
			switch {
			case !exists:
				dst[key] = sv

			case exists && isSlice:
				if !m.isOverwrite(ctxpath) {
					sv = append(dvv, sv...)
				}
				dst[key] = sv

			case exists && !isSlice:
				return fmt.Errorf("key[%s] mismatch: src(%T) vs dst(%T)", cpath, sv, dv)

			case exists && m.isOverwrite(ctxpath):
				return fmt.Errorf("key[%s] duplicated (overrwrite=false)", cpath)
			}

		// Mapping
		case map[string]any:
			dv, exists := dst[key]
			dvv, isMap := dv.(map[string]any) // if exists=false, then dv=nil and isMap=false
			switch {
			case !exists:
				// Dest Missing
				dv := make(map[string]any)
				dst[key] = dv
				err := m.mergeMapping(dv, sv, cpath)
				if err != nil {
					return err
				}
			case isMap:
				// Dest Merge
				err := m.mergeMapping(dvv, sv, cpath)
				if err != nil {
					return err
				}
			default:
				// Dest type mismatch
				return fmt.Errorf("key[%s] mismatch: src(%T) vs dst(%T)", cpath, sv, dv)
			}

		// Scalar
		default:
			dv, ok := dst[key]
			switch {
			case ok && reflect.DeepEqual(sv, dv):
				continue
			case ok && !m.isOverwrite(ctxpath):
				return fmt.Errorf("duplicate Keys(overrwrite=false): %s", cpath)
			default:
				dst[key] = sv
			}
		}
	}
	return nil
}

func buildPolicy(c *Options) *mergePolicy {
	var overwrite []policyEntry[bool]
	for _, pattern := range c.Overwrite {
		overwrite = addPolicy(overwrite, pattern, true)
	}
	for _, pattern := range c.Append {
		overwrite = addPolicy(overwrite, pattern, false)
	}
	// Absolute patterns before relative patterns.
	slices.SortFunc(overwrite, func(a, b policyEntry[bool]) int {
		return cmp.Or(
			compareBool(a.isRelative, b.isRelative),
			cmp.Compare(a.pattern, b.pattern))
	})

	var resolvePaths []policyEntry[bool]
	for _, pattern := range c.ResolvePath {
		resolvePaths = addPolicy(resolvePaths, pattern, true)
	}
	return &mergePolicy{
		overwrite:        overwrite,
		defaultOverwrite: c.DefaultOverWrite,
		resolvePaths:     resolvePaths,
	}
}

type mergePolicy struct {
	overwrite        []policyEntry[bool]
	defaultOverwrite bool
	resolvePaths     []policyEntry[bool]
}

func (m *mergePolicy) isOverwrite(contextPath string) bool {
	for _, entry := range m.overwrite {
		if entry.match(contextPath) {
			return entry.policy
		}
	}
	return m.defaultOverwrite
}

func (m *mergePolicy) resolvePath(contextPath string) bool {
	for _, entry := range m.resolvePaths {
		if entry.match(contextPath) {
			return true
		}
	}
	return false
}

type policyEntry[T comparable] struct {
	pattern    string
	policy     T
	isRelative bool
}

func (e policyEntry[T]) match(contextPath string) bool {
	if e.isRelative && strings.HasSuffix(contextPath, string(e.pattern)) {
		return true
	}
	if e.pattern == contextPath {
		return true
	}
	return false
}

func addPolicy[T comparable](policies []policyEntry[T], pattern string, policy T) []policyEntry[T] {
	if slices.ContainsFunc(policies, func(p policyEntry[T]) bool {
		return p.pattern == pattern && p.policy != policy
	}) {
		panic("config contains conflicting policies")
	}
	if !strings.HasPrefix(pattern, ".") && !strings.HasPrefix(pattern, "$.") {
		pattern = "$." + pattern
	}
	return append(policies, policyEntry[T]{
		pattern:    pattern,
		policy:     policy,
		isRelative: strings.HasPrefix(pattern, "."),
	})
}

func compareBool(a, b bool) int {
	if a == b {
		return 0
	}
	if a {
		return 1
	}
	return -1
}
