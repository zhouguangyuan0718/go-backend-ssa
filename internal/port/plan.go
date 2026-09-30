// Package port implements the pinned Go source inspection and migration tool.
package port

import (
	"fmt"
	"go/token"
	"path"
	"regexp"
	"strings"
)

const ToolVersion = "syncgo-m3-v45"

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

// ContextSource keeps a native signature, deriving its instance from a reviewed
// argument/receiver through a typed adapter resolver.
type ContextSource struct{ Argument, Resolver string }

type Unit struct {
	// AdapterMethods are selected native receiver methods implemented in this
	// unit's handwritten adapter, with exactly the original receiver/signature.
	AdapterMethods []string

	StatelessFunctions     []string // Keep reviewed state-free functions at package scope.
	ContextFrom            map[string]ContextSource
	ContextAfterGuards     map[string]int // Preserve reviewed pure early returns before owner binding.
	Source                 string
	Functions              []string
	Declarations           []string // Types, complete constant groups, and Receiver.Method names.
	ExportConstants        bool     // Generate public aliases for this file's exported native constants.
	Owner                  *OwnerRule
	WholeFile              bool                // Preserve types, constants and existing receivers.
	ZeroValues             []string            // Reviewed zero-valued struct globals replaced by fresh values.
	ReadTables             []string            // Constant array/slice/map lookups localized in their consuming functions.
	ConstantFields         []ConstantFieldRule // Stateless reads of immutable scalar fields in an instance-owned array.
	FixedSizeof            []FixedSizeofRule
	SliceReinterpretations []SliceReinterpretationRule
	HostResources          []HostResourceRule
	HostCalls              []HostCallRule
	State                  []StateRule
	ReceiverState          *ReceiverStateRule
	Context                *ContextRule
	ContextMethods         map[string]string   // Free function -> method name when the native Context already uses that name.
	ContextParams          []string            // Reviewed receiver methods that take an explicit context parameter.
	Literals               []LiteralRule       // Reviewed external struct values bound to an instance.
	Calls                  []CallRule          // Reviewed external functions bound to context methods.
	Nullable               map[string][]string // Reviewed owned pointers/receivers that accept nil.
	Checks                 map[string]string   // Selected function/method -> context argument validator.
}

// HostResourceRule binds a reviewed standard-library resource factory to a
// compilation method. Only listed members on its nonescaping local result are
// admitted by the output audit; other host operations remain rejected.
type HostResourceRule struct {
	Package, Factory, Method string
	Members                  []string
}

// HostCallRule binds reviewed compiler diagnostics to an explicit instance
// service. Create returns io.WriteCloser instead of exposing the host file.
// Getenv only accepts the reviewed literal key; metrics remain local data.
type HostCallRule struct {
	Package, Function, Method string
	StringArgument            string
}

// CallRule derives the upstream signature from Source; no replacement package
// or invented compiler declaration is emitted into the library.
type CallRule struct{ Source, Function, Method string }

// LiteralRule wraps explicit native struct literals and local typed zero
// declarations with a func(T) T instance method, or reviewed address literals
// with a func(*T) *T method when Pointer is set.
type LiteralRule struct {
	Source, Type, Method string
	Pointer              bool // Bind an explicit &T{...} through func(*T) *T, preserving its allocation.
}

// StateRule redirects typed host state to an instance. Variable may name a
// whole variable, a literal bool feature, or a reviewed direct struct field.
type StateRule struct{ Source, Variable, Field string }

// ConstantFieldRule preserves selected native query signatures while the full
// table moves into its context. The initializer and every table use are checked
// before generating an immutable scalar lookup. No mutable table is shared.
type ConstantFieldRule struct {
	Variable, Field string
	Functions       []string
}

// ReceiverStateRule keeps state in an existing private upstream receiver.
type ReceiverStateRule struct {
	Type, InitMethod string
	Fields           map[string]string
	RunInit          bool
}

type OwnerRule struct {
	Embedded       map[string]string // Type -> path to an embedded value carrying Field; add no duplicate field.
	Existing       bool              // Bind an existing *Context field instead of adding one.
	Types          []string
	Context, Field string
}

type ContextRule struct {
	InitGroup         string // Separate receiver initializer for an independently initialized set of native state.
	Initialize        bool   // Emit a receiver initializer; the adapter supplies dependent instance inputs first.
	PreserveNilArgs   bool   // Keep original nil argument semantics; still reject foreign non-nil owners.
	Embed             string // Handwritten instance fields embedded into the original context.
	RedirectReadFile  bool   // Bind os.ReadFile to an explicit instance input provider.
	RedirectExit      bool   // Replace process exit with a non-returning instance adapter.
	RedirectTrace     bool   // Route stdout/stderr tracing through instance adapter methods.
	Selective         bool   // Only bind free functions that depend on state or owned allocation.
	Existing          bool   // Extend the upstream Type and run its original Constructor first.
	Type, Constructor string
	Fields            map[string]string
	Adapter           string // Handwritten filename in the ported package.
	DeferInit         bool   // Reviewed init outside a partial selection; may not reference selected state or functions.
	Init              bool   // Move this file's reviewed init bodies into per-context initialization.
}

func (u Unit) output() string        { return portPath(u.Source) }
func (u Unit) sourcePackage() string { return strings.TrimPrefix(path.Dir(u.Source), "src/") }
func (u Unit) adapterPath() string   { return path.Join(path.Dir(u.output()), u.Context.Adapter) }
func (u Unit) constantsOutput() string {
	return strings.TrimSuffix(strings.TrimPrefix(u.output(), "internal/"), ".go") + "_constants.go"
}

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
		if u.ExportConstants {
			facade := u.constantsOutput()
			if !strings.HasPrefix(u.Source, "src/cmd/compile/internal/") || !ownedPath(facade) || strings.Count(facade, "/") != 1 || outputs[facade] {
				return fmt.Errorf("invalid constant facade: %s", facade)
			}
			outputs[facade] = true
		}
		for _, r := range u.ConstantFields {
			if !u.WholeFile || u.Context == nil || u.Context.Fields[r.Variable] == "" || !token.IsIdentifier(r.Variable) || !token.IsIdentifier(r.Field) || len(r.Functions) == 0 {
				return fmt.Errorf("invalid constant field rule: %s.%s", r.Variable, r.Field)
			}
		}
		seenSliceMethods := map[string]bool{}
		for _, r := range u.SliceReinterpretations {
			for _, name := range []string{r.Allocate, r.Free, r.BaseAllocate, r.BaseFree} {
				owner, method, ok := splitMethod(name)
				if !ok || !token.IsIdentifier(owner) || !token.IsIdentifier(method) {
					return fmt.Errorf("invalid slice reinterpretation method %s", name)
				}
			}
			for _, name := range []string{r.Allocate, r.Free} {
				selected := u.WholeFile
				for _, decl := range u.Declarations {
					selected = selected || decl == name
				}
				if !selected || seenSliceMethods[name] {
					return fmt.Errorf("unselected or duplicate slice reinterpretation method %s", name)
				}
				seenSliceMethods[name] = true
			}
		}
		if r := u.ReceiverState; r != nil {
			if !u.WholeFile && (r.RunInit || r.InitMethod != "") || u.Context != nil || u.Owner != nil || !token.IsIdentifier(r.Type) || len(r.Fields) == 0 || r.InitMethod != "" && !token.IsIdentifier(r.InitMethod) || r.RunInit && r.InitMethod == "" {
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

		seenSizes := map[FixedSizeofRule]bool{}
		for _, rule := range u.FixedSizeof {
			validFunction := token.IsIdentifier(rule.Function)
			if receiver, method, ok := strings.Cut(rule.Function, "."); ok {
				validFunction = token.IsIdentifier(receiver) && token.IsIdentifier(method)
			}
			if !validFunction || !token.IsIdentifier(rule.Type) || seenSizes[rule] {
				return fmt.Errorf("invalid fixed sizeof rule: %s", rule.Function)
			}
			seenSizes[rule] = true
		}

		seenAdapters := map[string]bool{}
		for _, name := range u.AdapterMethods {
			receiver, method, ok := strings.Cut(name, ".")
			selected := u.WholeFile
			for _, decl := range u.Declarations {
				selected = selected || decl == name
			}
			if u.Context == nil || u.Context.Adapter == "" || !selected || !ok || !token.IsIdentifier(receiver) || !token.IsIdentifier(method) || seenAdapters[name] {
				return fmt.Errorf("invalid adapter method: %s", name)
			}
			if _, ok := u.ContextFrom[name]; ok {
				return fmt.Errorf("adapter method cannot also derive context: %s", name)
			}
			if _, ok := u.Checks[name]; ok {
				return fmt.Errorf("adapter method owns its entry checks: %s", name)
			}
			seenAdapters[name] = true
		}
		for name, binding := range u.ContextFrom {
			if u.Context == nil || !memberPath(name) || !token.IsIdentifier(binding.Argument) || binding.Argument == "_" || !token.IsIdentifier(binding.Resolver) || binding.Resolver == "_" {
				return fmt.Errorf("invalid context source: %s", name)
			}
			if u.ContextMethods[name] != "" {
				return fmt.Errorf("context source cannot rename function: %s", name)
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
			if u.Context == nil || !ok || !token.IsIdentifier(receiver) || (method != "*" && (!token.IsIdentifier(method) || token.IsExported(method) && token.IsExported(receiver))) || parameterMethods[name] {
				return fmt.Errorf("invalid context parameter method: %s", name)
			}
			parameterMethods[name] = true
		}
		for _, state := range u.State {
			if u.Context == nil || !safePath(state.Source) || portPath(state.Source) == "" || !memberPath(state.Variable) || !memberPath(state.Field) {
				return fmt.Errorf("invalid external state binding")
			}
		}
		for _, literal := range u.Literals {
			if u.Context == nil || !safePath(literal.Source) || portPath(literal.Source) == "" || !token.IsIdentifier(literal.Type) || !memberPath(literal.Method) {
				return fmt.Errorf("invalid literal binding")
			}
		}
		for _, resource := range u.HostResources {
			if u.Context == nil || u.Context.Adapter == "" || resource.Package != "os/exec" || resource.Factory != "Command" || !token.IsIdentifier(resource.Method) || len(resource.Members) == 0 {
				return fmt.Errorf("invalid host resource binding")
			}
			seen := map[string]bool{}
			for _, member := range resource.Members {
				if !token.IsIdentifier(member) || seen[member] {
					return fmt.Errorf("invalid host resource member %s", member)
				}
				seen[member] = true
			}
		}
		seenHostCalls := map[string]bool{}
		for _, call := range u.HostCalls {
			key := call.Package + "." + call.Function
			allowed := key == "os.Create" || key == "os.Getenv" || key == "runtime.ReadMemStats" || key == "time.Now"
			if !allowed || u.Context == nil || u.Context.Adapter == "" || !token.IsIdentifier(call.Method) || call.Method == "_" || seenHostCalls[key] || (key == "os.Getenv") != (call.StringArgument != "") {
				return fmt.Errorf("invalid host call binding: %s", key)
			}
			seenHostCalls[key] = true
		}
		for _, call := range u.Calls {
			if u.Context == nil || !safePath(call.Source) || portPath(call.Source) == "" || !token.IsIdentifier(call.Function) || !memberPath(call.Method) {
				return fmt.Errorf("invalid external call binding")
			}
		}
		for _, name := range u.StatelessFunctions {
			selected := u.WholeFile
			for _, function := range u.Functions {
				selected = selected || name == function
			}
			if u.Context == nil || !selected || !token.IsIdentifier(name) {
				return fmt.Errorf("invalid stateless function: %s", name)
			}
		}
		for name, count := range u.ContextAfterGuards {
			selected := u.WholeFile
			for _, declaration := range append(append([]string(nil), u.Functions...), u.Declarations...) {
				selected = selected || name == declaration
			}
			if u.Context == nil || !selected || count <= 0 {
				return fmt.Errorf("invalid context guard prefix: %s", name)
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
		if u.Context != nil && u.Context.DeferInit && (u.WholeFile || u.Context.Init) {
			return fmt.Errorf("deferred init requires a partial selection without Init: %s", u.Source)
		}
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
			for name, path := range owner.Embedded {
				if !seen[name] || !memberPath(path) || owner.Existing {
					return fmt.Errorf("invalid embedded owner rule: %s", name)
				}
			}
			if u.Context != nil && u.Context.Type != owner.Context {
				return fmt.Errorf("owner/context mismatch")
			}
		}
		if c := u.Context; c != nil {
			if c.InitGroup != "" && (!c.Initialize || c.Existing || !token.IsIdentifier(c.InitGroup) || c.InitGroup == "_" || c.InitGroup == c.Constructor || c.DeferInit) {
				return fmt.Errorf("invalid context initializer group")
			}
			if c.Embed != "" && (!c.Existing || c.Adapter == "" || !token.IsIdentifier(c.Embed) || c.Embed == "_") {
				return fmt.Errorf("invalid native context embed")
			}
			if (c.RedirectTrace || c.RedirectExit || c.RedirectReadFile) && c.Adapter == "" {
				return fmt.Errorf("trace redirection requires an adapter")
			}
			if previous := contexts[pkg]; previous != nil {
				if previous.Initialize != c.Initialize || previous.Type != c.Type || previous.Constructor != c.Constructor || previous.Adapter != c.Adapter || previous.Existing != c.Existing || previous.Embed != c.Embed || previous.RedirectTrace != c.RedirectTrace || previous.RedirectExit != c.RedirectExit || previous.RedirectReadFile != c.RedirectReadFile || previous.PreserveNilArgs != c.PreserveNilArgs {
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

func memberPath(s string) bool {
	for _, name := range strings.Split(s, ".") {
		if !token.IsIdentifier(name) || name == "_" {
			return false
		}
	}
	return true
}
