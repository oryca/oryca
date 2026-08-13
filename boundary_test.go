// Package oryca holds no code — only the test that keeps the two binaries apart.
package oryca

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/oryca/oryca"

// TestBinariesDoNotImportEachOther keeps the two binaries independent.
//
// The gateway faces the internet and holds no database credentials. It stays that
// way only if it never imports control-plane code. They talk over HTTP and Redis
// instead, see docs/sync-contract.md.
//
// If this fails, move the shared piece into a package both can import, or copy the
// few lines across like the sync payloads already do.
func TestBinariesDoNotImportEachOther(t *testing.T) {
	for _, tc := range []struct {
		dir       string
		forbidden string
	}{
		{dir: "gateway", forbidden: modulePath + "/control-plane"},
		{dir: "control-plane", forbidden: modulePath + "/gateway"},
		{dir: "cmd/oryca-gateway", forbidden: modulePath + "/control-plane"},
		{dir: "cmd/oryca-control-plane", forbidden: modulePath + "/gateway"},
	} {
		t.Run(tc.dir, func(t *testing.T) {
			for file, imports := range importsUnder(t, tc.dir) {
				for _, imp := range imports {
					if imp == tc.forbidden || strings.HasPrefix(imp, tc.forbidden+"/") {
						t.Errorf("%s imports %s — the two binaries must stay independent", file, imp)
					}
				}
			}
		})
	}
}

// importsUnder returns every import path found in the Go files under dir, keyed
// by file, test files included.
func importsUnder(t *testing.T, dir string) map[string][]string {
	t.Helper()

	found := make(map[string][]string)
	fset := token.NewFileSet()

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range file.Imports {
			unquoted, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			found[path] = append(found[path], unquoted)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	if len(found) == 0 {
		t.Fatalf("no Go files found under %s — has the layout changed?", dir)
	}
	return found
}
