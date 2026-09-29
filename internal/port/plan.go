// Package port implements the pinned Go source inspection and migration tool.
package port

import (
	"fmt"
	"go/token"
	"path"
	"regexp"
	"strings"
)

const ToolVersion = "syncgo-m3-v6"

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
	SupportFiles []string // Reviewed handwritten dependencies used by context adapters.
	Units        []Unit
}

type Unit struct {
	Source         string
	Functions      []string
	Declarations   []string // Types, complete constant groups, and Receiver.Method names.
	Owner          *OwnerRule
	WholeFile      bool     // Preserve types, constants and existing receivers.
	ZeroValues     []string // Reviewed zero-valued struct globals replaced by fresh values.
	ReadTables     []string // Constant array lookups localized in their consuming functions.
	State          []StateRule
	ReceiverState  *ReceiverStateRule
	Context        *ContextRule
	ContextMethods map[string]string   // Free function -> method name when the native Context already uses that name.
	ContextParams  []string            // Private receiver methods that take an explicit context parameter.
	Calls          []CallRule          // Reviewed external functions bound to context methods.
	Nullable       map[string][]string // Reviewed owned pointers/receivers that accept nil.
	Checks         map[string]string   // Selected function/method -> context argument validator.
}

// CallRule derives the upstream signature from Source; no replacement package
// or invented compiler declaration is emitted into the library.
type CallRule struct{ Source, Function, Method string }

// StateRule redirects an explicitly typed upstream host variable to instance state.
type StateRule struct{ Source, Variable, Field string }

// ReceiverStateRule keeps state in an existing private upstream receiver.
type ReceiverStateRule struct {
	Type, InitMethod string
	Fields           map[string]string
	RunInit          bool
}

type OwnerRule struct {
	Existing       bool // Bind an existing *Context field instead of adding one.
	Types          []string
	Context, Field string
}

type ContextRule struct {
	PreserveNilArgs   bool   // Keep original nil argument semantics; still reject foreign non-nil owners.
	Embed             string // Handwritten instance fields embedded into the original context.
	RedirectExit      bool   // Replace process exit with a non-returning instance adapter.
	RedirectTrace     bool   // Route stdout/stderr tracing through instance adapter methods.
	Selective         bool   // Only bind free functions that depend on state or owned allocation.
	Existing          bool   // Extend the upstream Type and run its original Constructor first.
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
	supportSeen := map[string]bool{}
	for _, file := range p.SupportFiles {
		if supportSeen[file] || !safePath(file) || path.Ext(file) != ".go" || strings.HasSuffix(file, "_test.go") {
			return fmt.Errorf("invalid support file: %s", file)
		}
		supportSeen[file] = true
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
		if r := u.ReceiverState; r != nil {
			if !u.WholeFile || u.Context != nil || u.Owner != nil || !token.IsIdentifier(r.Type) || len(r.Fields) == 0 || r.InitMethod != "" && !token.IsIdentifier(r.InitMethod) || r.RunInit && r.InitMethod == "" {
				return fmt.Errorf("invalid receiver state rule")
			}
			seen := map[string]bool{}
			for variable, field := range r.Fields {
				if !token.IsIdentifier(variable) || !token.IsIdentifier(field) || seen[field] {
					return fmt.Errorf("invalid receiver state field")
				}
				seen[field] = true
			}
		}
		for source, target := range u.ContextMethods {
			if u.Context == nil || !token.IsIdentifier(source) || !token.IsIdentifier(target) {
				return fmt.Errorf("invalid context method name: %s", source)
			}
		}
		parameterMethods := map[string]bool{}
		for _, name := range u.ContextParams {
			receiver, method, ok := strings.Cut(name, ".")
			if u.Context == nil || !ok || !token.IsIdentifier(receiver) || (method != "*" && (!token.IsIdentifier(method) || token.IsExported(method))) || parameterMethods[name] {
				return fmt.Errorf("invalid context parameter method: %s", name)
			}
			parameterMethods[name] = true
		}
		for _, state := range u.State {
			if u.Context == nil || !safePath(state.Source) || portPath(state.Source) == "" || !token.IsIdentifier(state.Variable) || !token.IsIdentifier(state.Field) {
				return fmt.Errorf("invalid external state binding")
			}
		}
		for _, call := range u.Calls {
			if u.Context == nil || !safePath(call.Source) || portPath(call.Source) == "" || !token.IsIdentifier(call.Function) || !token.IsIdentifier(call.Method) {
				return fmt.Errorf("invalid external call binding")
			}
		}
		for name, params := range u.Nullable {
			selected := u.WholeFile
			for _, declaration := range append(append([]string(nil), u.Functions...), u.Declarations...) {
				selected = selected || name == declaration
			}
			if u.Context == nil || !selected || len(params) == 0 {
				return fmt.Errorf("invalid nullable rule: %s", name)
			}
			seen := map[string]bool{}
			for _, param := range params {
				if !token.IsIdentifier(param) || param == "_" || seen[param] {
					return fmt.Errorf("invalid nullable parameter: %s", param)
				}
				seen[param] = true
			}
		}
		for name, method := range u.Checks {
			selected := u.WholeFile
			for _, declaration := range append(append([]string(nil), u.Functions...), u.Declarations...) {
				selected = selected || name == declaration
			}
			if u.Context == nil || !selected || !token.IsIdentifier(method) {
				return fmt.Errorf("invalid entry check: %s", name)
			}
		}
		pkg := path.Dir(output)
		outputs[output] = true
		if u.WholeFile && (len(u.Functions)+len(u.Declarations) != 0) || !u.WholeFile && len(u.ZeroValues)+len(u.ReadTables) != 0 {
			return fmt.Errorf("incompatible whole-file rules: %s", u.Source)
		}
		if owner := u.Owner; owner != nil {
			if len(owner.Types) == 0 || !token.IsIdentifier(owner.Context) || !token.IsIdentifier(owner.Field) || !owner.Existing && token.IsExported(owner.Field) {
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
			if c.Embed != "" && (!c.Existing || c.Adapter == "" || !token.IsIdentifier(c.Embed) || c.Embed == "_") {
				return fmt.Errorf("invalid native context embed")
			}
			if (c.RedirectTrace || c.RedirectExit) && c.Adapter == "" {
				return fmt.Errorf("trace redirection requires an adapter")
			}
			if previous := contexts[pkg]; previous != nil {
				if previous.Type != c.Type || previous.Constructor != c.Constructor || previous.Adapter != c.Adapter || previous.Existing != c.Existing || previous.Selective != c.Selective || previous.Embed != c.Embed || previous.RedirectTrace != c.RedirectTrace || previous.RedirectExit != c.RedirectExit || previous.PreserveNilArgs != c.PreserveNilArgs {
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
			if !(c.Existing && c.Adapter == "") && (!safePath(c.Adapter) || path.Base(c.Adapter) != c.Adapter || !strings.HasPrefix(c.Adapter, "adapter_") || path.Ext(c.Adapter) != ".go" || strings.HasSuffix(c.Adapter, "_test.go")) {
				return fmt.Errorf("invalid context adapter: %s", c.Adapter)
			}
		}
	}
	return nil
}
