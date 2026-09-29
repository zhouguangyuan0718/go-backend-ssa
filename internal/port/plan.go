// Package port implements the pinned Go source inspection and migration tool.
package port

import (
	"fmt"
	"go/token"
	"path"
	"regexp"
	"strings"
)

const ToolVersion = "syncgo-m2-v5"

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
	Source       string
	Functions    []string
	Declarations []string // Types, complete constant groups, and Receiver.Method names.
	Owner        *OwnerRule
	WholeFile    bool     // Preserve types, constants and existing receivers.
	ZeroValues   []string // Reviewed zero-valued struct globals replaced by fresh values.
	ReadTables   []string // Constant array lookups localized in their consuming functions.
	Context      *ContextRule
	Calls        []CallRule // Reviewed external functions bound to context methods.
}

// CallRule derives the upstream signature from Source; no replacement package
// or invented compiler declaration is emitted into the library.
type CallRule struct{ Source, Function, Method string }

type OwnerRule struct {
	Types          []string
	Context, Field string
}

type ContextRule struct {
	Type, Constructor string
	Fields            map[string]string
	Adapter           string // Handwritten filename in the ported package.
	Init              bool   // Move this file's reviewed init bodies into per-context initialization.
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
	outputs := map[string]bool{}
	contexts := map[string]*ContextRule{}
	states := map[string]map[string]bool{}
	fields := map[string]map[string]bool{}
	for _, u := range p.Units {
		output := u.output()
		if !safePath(u.Source) || !ownedPath(output) || path.Ext(output) != ".go" || (!u.WholeFile && len(u.Functions)+len(u.Declarations) == 0 && (u.Context == nil || len(u.Context.Fields) == 0)) || outputs[output] {
			return fmt.Errorf("invalid migration source: %s", u.Source)
		}
		for _, call := range u.Calls {
			if u.Context == nil || !safePath(call.Source) || portPath(call.Source) == "" || !token.IsIdentifier(call.Function) || !token.IsIdentifier(call.Method) {
				return fmt.Errorf("invalid external call binding")
			}
		}
		pkg := path.Dir(output)
		outputs[output] = true
		if u.WholeFile && (len(u.Functions)+len(u.Declarations) != 0 || u.Context != nil) || !u.WholeFile && len(u.ZeroValues)+len(u.ReadTables) != 0 {
			return fmt.Errorf("incompatible whole-file rules: %s", u.Source)
		}
		if owner := u.Owner; owner != nil {
			if len(owner.Types) == 0 || !token.IsIdentifier(owner.Context) || !token.IsIdentifier(owner.Field) || token.IsExported(owner.Field) {
				return fmt.Errorf("invalid owner rule")
			}
			seen := map[string]bool{}
			for _, name := range owner.Types {
				if !token.IsIdentifier(name) || seen[name] {
					return fmt.Errorf("invalid owner type %s", name)
				}
				seen[name] = true
			}
			if u.Context != nil && u.Context.Type != owner.Context {
				return fmt.Errorf("owner/context mismatch")
			}
		}
		if c := u.Context; c != nil {
			if previous := contexts[pkg]; previous != nil {
				if previous.Type != c.Type || previous.Constructor != c.Constructor || previous.Adapter != c.Adapter {
					return fmt.Errorf("inconsistent package context: %s", pkg)
				}
			} else {
				contexts[pkg] = c
				states[pkg], fields[pkg] = map[string]bool{}, map[string]bool{}
			}
			if !token.IsIdentifier(c.Type) || !token.IsIdentifier(c.Constructor) {
				return fmt.Errorf("invalid context identifier")
			}
			for state, field := range c.Fields {
				if !token.IsIdentifier(state) || !token.IsIdentifier(field) || states[pkg][state] || fields[pkg][field] {
					return fmt.Errorf("invalid or duplicate context state/field: %s", state)
				}
				states[pkg][state], fields[pkg][field] = true, true
			}
			if !safePath(c.Adapter) || path.Base(c.Adapter) != c.Adapter || !strings.HasPrefix(c.Adapter, "adapter_") || path.Ext(c.Adapter) != ".go" || strings.HasSuffix(c.Adapter, "_test.go") {
				return fmt.Errorf("invalid context adapter: %s", c.Adapter)
			}
		}
	}
	return nil
}
