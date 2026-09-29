// Package port implements the pinned Go source inspection and migration tool.
package port

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

const ToolVersion = "syncgo-m1-v4"

// Compiler paths are relative to src/cmd/compile. The two auxiliary internal
// trees get distinct roots to avoid collisions such as compiler abi vs Go abi.
func portPath(source string) string {
	for _, m := range [][2]string{{"src/cmd/compile/", ""}, {"src/cmd/internal/", "internal/cmd/"}, {"src/internal/", "internal/go/"}} {
		if strings.HasPrefix(source, m[0]) {
			return m[1] + strings.TrimPrefix(source, m[0])
		}
	}
	return ""
}

// Pin identifies the official source and the toolchain used to port it.
type Pin struct {
	Tag, Commit, BuildToolchain, TargetModule string
}

// Plan and its rules are ordinary Go values defined in rules.go.
type Plan struct {
	InspectRoots []string
	Units        []Unit
}

type Unit struct {
	Source    string
	Functions []string
	Context   *ContextRule
}

type ContextRule struct {
	Type, Constructor string
	Fields            map[string]string
	Adapter           string // Handwritten filename in the ported package.
}

func (u Unit) output() string        { return portPath(u.Source) }
func (u Unit) sourcePackage() string { return strings.TrimPrefix(path.Dir(u.Source), "src/") }
func (u Unit) adapterPath() string   { return path.Join(path.Dir(u.output()), u.Context.Adapter) }

func (p Plan) importMap(module string) map[string]string {
	imports := make(map[string]string, len(p.Units))
	for _, u := range p.Units {
		imports[u.sourcePackage()] = module + "/" + path.Dir(u.output())
	}
	return imports
}

func (p Pin) Validate() error {
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(p.Commit) {
		return fmt.Errorf("invalid pinned commit")
	}
	if !regexp.MustCompile(`^go1\.27\.[0-9]+$`).MatchString(p.Tag) {
		return fmt.Errorf("only a fixed Go 1.27 release is supported")
	}
	if p.BuildToolchain != "go1.27.1" || p.TargetModule != "go-backend-ssa" {
		return fmt.Errorf("M1 requires go1.27.1 and module go-backend-ssa")
	}
	return nil
}

func safePath(s string) bool {
	return s != "" && s != "." && path.Clean(s) == s && !path.IsAbs(s) && s != ".." && !hasPrefix(s, "../") && !hasPrefix(s, ".git/") && s != ".git" && !containsBackslash(s)
}

func hasPrefix(s, prefix string) bool { return len(s) >= len(prefix) && s[:len(prefix)] == prefix }
func containsBackslash(s string) bool {
	for _, r := range s {
		if r == '\\' {
			return true
		}
	}
	return false
}

func (p Plan) Validate() error {
	if len(p.Units) == 0 || len(p.InspectRoots) == 0 {
		return fmt.Errorf("empty migration plan")
	}
	packages := map[string]bool{}
	for _, u := range p.Units {
		output := u.output()
		if !safePath(u.Source) || !ownedPath(output) || path.Ext(output) != ".go" || len(u.Functions) == 0 {
			return fmt.Errorf("invalid migration source: %s", u.Source)
		}
		pkg := path.Dir(output)
		if packages[pkg] {
			return fmt.Errorf("M1 supports one source unit per package: %s", output)
		}
		packages[pkg] = true
		if c := u.Context; c != nil {
			if len(c.Fields) == 0 || !safePath(c.Adapter) || path.Base(c.Adapter) != c.Adapter || !strings.HasPrefix(c.Adapter, "adapter_") || path.Ext(c.Adapter) != ".go" || strings.HasSuffix(c.Adapter, "_test.go") {
				return fmt.Errorf("invalid context adapter: %s", c.Adapter)
			}
		}
	}
	return nil
}
