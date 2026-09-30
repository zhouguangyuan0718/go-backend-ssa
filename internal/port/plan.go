// Package port implements the pinned Go source inspection and migration tool.
package port

import (
	"fmt"
	"go/build/constraint"
	"go/token"
	"path"
	"regexp"
	"strings"
)

const ToolVersion = "syncgo-m3-v72"

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
	BuildVariant           string // Explicit native variant made unconditional after exact constraint validation.
	Functions              []string
	FunctionRegions        map[string]FunctionRegionRule
	FunctionContinuations  map[string]FunctionContinuationRule
	Declarations           []string // Types, complete constant groups, and Receiver.Method names.
	ExportConstants        bool     // Generate public aliases for this file's exported native constants.
	Owner                  *OwnerRule
	WholeFile              bool                // Preserve types, constants and existing receivers.
	ZeroValues             []string            // Reviewed zero-valued struct globals replaced by fresh values.
	ReadTables             []string            // Constant array/slice/map lookups localized in their consuming functions.
	ConstantFields         []ConstantFieldRule // Stateless reads of immutable scalar fields in an instance-owned array.
	FixedSizeof            []FixedSizeofRule
	SliceReinterpretations []SliceReinterpretationRule
	HostFields             []HostFieldRule
	HostSignatures         []HostSignatureRule
	HostConstants          []HostConstantRule
	HostResources          []HostResourceRule
	HostCalls              []HostCallRule
	State                  []StateRule
	ReceiverState          *ReceiverStateRule
	Context                *ContextRule
	ContextMethods         map[string]string      // Free function -> method name when the native Context already uses that name.
	ContextParamCalls      []ContextParamCallRule // Explicit bindings for exported value methods in another package.
	ContextParamExports    []string               // Explicit signature adaptations for public value methods without an owner.
	ContextParams          []string               // Reviewed receiver methods that take an explicit context parameter.
	Literals               []LiteralRule          // Reviewed external struct values bound to an instance.
	MethodCalls            []MethodCallRule
	Calls                  []CallRule          // Reviewed external functions bound to context methods.
	Nullable               map[string][]string // Reviewed owned pointers/receivers that accept nil.
	Checks                 map[string]string   // Selected function/method -> context argument validator.
}

// FunctionRegionRule extracts a contiguous sequence of complete native statements.
// Inputs are original pointer locals/parameters; their types are inferred from
// the pinned declarations. This is a stage boundary, not a body rewrite.
type FunctionRegionRule struct {
	// SourceFunction selects a second interval from an original function under
	// this rule's key. Empty keeps the original declaration name.
	SourceFunction string
	First, Last    SourceCallAnchor
	Inputs         []string
	EndBefore      bool
}

// FunctionContinuationRule delegates a complete native function suffix to a
// typed instance adapter. The suffix is preserved in a closure with its native
// captures; the adapter either runs it now or owns a later continuation.
type FunctionContinuationRule struct {
	Before SourceCallAnchor
	Method string
	State  string // a unique local struct declared before the boundary, passed by address
}

type SourceCallAnchor struct {
	// Assignment anchors a direct assignment to a package variable or field.
	// It is mutually exclusive with the imported-call anchor fields.
	Assignment      string
	Package, Member string
	// Enclosing selects a complete top-level range or if containing the call.
	// Empty requires a direct call statement.
	Enclosing      string
	StringArgument *SourceStringArgument
}
type SourceStringArgument struct {
	Index int
	Value string
}

// ContextParamCallRule passes Field from the caller Context to a selected
// ContextParamExports method. ContextType must match the defining unit.
type ContextParamCallRule struct{ Source, Method, Field, ContextType string }

// HostFieldRule replaces a private *os.File field with an interface carrying
// only reviewed native stream methods. It does not authorize any file factory.
type HostFieldRule struct {
	Type, Field string
	Methods     []string
}

// HostSignatureRule projects one native *os.File parameter or result onto
// explicitly reviewed stream operations. Index is zero-based within that list.
// Bodies and all other signature slots remain native.
type HostSignatureRule struct {
	Function string
	Result   bool
	Index    int
	Methods  []string
}

// HostResourceRule binds a reviewed standard-library resource factory to a
// compilation method. Only listed members on its nonescaping local result are
// admitted by the output audit; other host operations remain rejected.
type HostResourceRule struct {
	Package, Factory, Method string
	Members                  []string
}

// HostCallRule binds reviewed compiler diagnostics to an explicit instance
// service. File factories return only the reviewed stream interface.
// Environment and executable lookup calls accept reviewed literal keys only.
type HostConstantRule struct{ Package, Constant, Field string }

type HostCallRule struct {
	StreamMethods             []string // Explicit os.Open result interface capabilities.
	Package, Function, Method string
	StringArgument            string
	StringArguments           []string // Alternative literal keys; mutually exclusive with StringArgument.
}

// MethodCallRule binds direct calls to an already migrated native method. The
// adapter receives the original receiver as its first explicit parameter.
type MethodCallRule struct{ Source, Method, Adapter string }

// CallRule derives the upstream signature from Source; no replacement package
// or invented compiler declaration is emitted into the library.
type CallRule struct {
	Source, Function, Method string
	// KeepDefinition retains a selected local native body while routing calls
	// through its typed stage adapter. The adapter may invoke the retained body.
	KeepDefinition bool
}

// LiteralRule wraps explicit native struct literals and local typed zero
// declarations with a func(T) T instance method, or reviewed address literals
// with a func(*T) *T method when Pointer is set.
type LiteralRule struct {
	Source, Type, Method string
	Pointer              bool // Bind an explicit &T{...} through func(*T) *T, preserving its allocation.
	NilPointer           bool // Bind an explicit (*T)(nil) to its instance-owned native default receiver.
	MakeSlice            bool // Bind a native make([]T, len[, cap]) allocation through func([]T) []T.
}

// StateRule redirects typed host state to an instance. Variable may name a
// whole variable, a literal bool feature, or a reviewed direct struct field.
type StateRule struct {
	Source, Variable, Field string
	Input                   bool
	TypeSource              string // Pinned generator for a bootstrap constant initializer.
}

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

		for name, r := range u.FunctionContinuations {
			selected := u.WholeFile
			for _, f := range u.Functions {
				selected = selected || f == name
			}
			if !selected || u.Context == nil || !token.IsIdentifier(name) || name == "init" || !token.IsIdentifier(r.Method) || !token.IsIdentifier(r.State) || r.State == "_" || r.Before.Assignment != "" || r.Before.Enclosing != "" || r.Before.StringArgument != nil || !safePath(r.Before.Package) || !memberPath(r.Before.Member) {
				return fmt.Errorf("invalid function continuation: %s", name)
			}
			if _, ok := u.FunctionRegions[name]; ok {
				return fmt.Errorf("function region also has a continuation: %s", name)
			}
		}
		for name, r := range u.FunctionRegions {
			selected := false
			for _, f := range u.Functions {
				selected = selected || f == name
			}
			if u.WholeFile || !selected || !token.IsIdentifier(name) || name == "init" || u.Context == nil {
				return fmt.Errorf("invalid function region: %s", name)
			}
			if r.SourceFunction != "" && (!token.IsIdentifier(r.SourceFunction) || r.SourceFunction == "init" || r.SourceFunction == name) {
				return fmt.Errorf("invalid function region source: %s", r.SourceFunction)
			}
			for _, a := range []SourceCallAnchor{r.First, r.Last} {
				if a.Assignment != "" {
					if !memberPath(a.Assignment) || a.Package != "" || a.Member != "" || (a.Enclosing != "" && a.Enclosing != "if" && a.Enclosing != "range") || a.StringArgument != nil {
						return fmt.Errorf("invalid assignment region anchor: %s", name)
					}
					continue
				}
				if a.Package == "" || !safePath(a.Package) || !memberPath(a.Member) || (a.Enclosing != "" && a.Enclosing != "range" && a.Enclosing != "if") || (a.StringArgument != nil && a.StringArgument.Index < 0) {
					return fmt.Errorf("invalid function region anchor: %s", name)
				}
			}
			seen := map[string]bool{}
			for _, input := range r.Inputs {
				if !token.IsIdentifier(input) || input == "_" || seen[input] {
					return fmt.Errorf("invalid function region input: %s", input)
				}
				seen[input] = true
			}
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
		for _, name := range u.ContextParamExports {
			receiver, method, ok := strings.Cut(name, ".")
			if u.Context == nil || !ok || !token.IsIdentifier(receiver) || !token.IsExported(receiver) || !token.IsIdentifier(method) || !token.IsExported(method) || parameterMethods[name] {
				return fmt.Errorf("invalid exported context parameter method: %s", name)
			}
			parameterMethods[name] = true
		}
		seenParamCalls := map[string]bool{}
		for _, call := range u.ContextParamCalls {
			receiver, method, ok := strings.Cut(call.Method, ".")
			key := call.Source + ":" + call.Method
			if u.Context == nil || !safePath(call.Source) || portPath(call.Source) == "" || !ok || !token.IsIdentifier(receiver) || !token.IsIdentifier(method) || !memberPath(call.Field) || !token.IsIdentifier(call.ContextType) || seenParamCalls[key] {
				return fmt.Errorf("invalid external context method: %s", key)
			}
			seenParamCalls[key] = true
			declared := false
			for _, target := range p.Units {
				if target.Source == call.Source && target.Context != nil && target.Context.Type == call.ContextType {
					for _, export := range target.ContextParamExports {
						declared = declared || export == call.Method
					}
				}
			}
			if !declared {
				return fmt.Errorf("external context method is not an explicit export: %s", key)
			}
		}
		for _, state := range u.State {
			if state.TypeSource != "" && !safePath(state.TypeSource) {
				return fmt.Errorf("invalid generated constant type source")
			}
			if state.Input && (strings.TrimPrefix(path.Dir(state.Source), "src/") != u.sourcePackage() || !token.IsIdentifier(state.Variable) || !token.IsIdentifier(state.Field)) {
				return fmt.Errorf("invalid local host input")
			}
			if u.Context == nil || !safePath(state.Source) || portPath(state.Source) == "" || !memberPath(state.Variable) || !memberPath(state.Field) {
				return fmt.Errorf("invalid external state binding")
			}
		}
		for _, literal := range u.Literals {
			if u.Context == nil || !safePath(literal.Source) || portPath(literal.Source) == "" || !token.IsIdentifier(literal.Type) || !memberPath(literal.Method) || literal.Pointer && literal.NilPointer || literal.MakeSlice && (literal.Pointer || literal.NilPointer) {
				return fmt.Errorf("invalid literal binding")
			}
		}

		if u.BuildVariant != "" {
			if _, err := constraint.Parse("//go:build " + u.BuildVariant); err != nil {
				return fmt.Errorf("invalid source build variant: %w", err)
			}
		}
		seenHostFields := map[string]bool{}
		for _, field := range u.HostFields {
			key := field.Type + "." + field.Field
			if !token.IsIdentifier(field.Type) || !token.IsIdentifier(field.Field) || token.IsExported(field.Field) || field.Field == "_" || len(field.Methods) == 0 || seenHostFields[key] {
				return fmt.Errorf("invalid host stream field: %s", key)
			}
			seenHostFields[key] = true
			methods := map[string]bool{}
			for _, name := range field.Methods {
				if !hostStreamMethod(name) || methods[name] {
					return fmt.Errorf("invalid host stream method: %s", name)
				}
				methods[name] = true
			}
		}
		seenHostSignatures := map[string]bool{}
		for _, rule := range u.HostSignatures {
			key := fmt.Sprintf("%s:%t:%d", rule.Function, rule.Result, rule.Index)
			if !memberPath(rule.Function) || rule.Index < 0 || len(rule.Methods) == 0 || seenHostSignatures[key] {
				return fmt.Errorf("invalid host stream signature: %s", key)
			}
			seenHostSignatures[key] = true
			seen := map[string]bool{}
			for _, name := range rule.Methods {
				if !hostStreamMethod(name) || seen[name] {
					return fmt.Errorf("invalid host stream signature method: %s", name)
				}
				seen[name] = true
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
		seenHostConstants := map[string]bool{}
		for _, r := range u.HostConstants {
			key := r.Package + "." + r.Constant
			allowed := key == "runtime.GOOS" || key == "os.O_APPEND" || key == "os.O_RDWR" || key == "os.O_CREATE" || key == "os.O_TRUNC"
			if !allowed || u.Context == nil || u.Context.Adapter == "" || !memberPath(r.Field) || seenHostConstants[key] {
				return fmt.Errorf("invalid host constant: %s", key)
			}
			seenHostConstants[key] = true
		}
		seenHostCalls := map[string]bool{}
		for _, call := range u.HostCalls {
			key := call.Package + "." + call.Function
			allowed := key == "flag.Lookup" || key == "os.Create" || key == "os.Open" || key == "os.OpenFile" || key == "os.Getwd" || key == "os.ExpandEnv" || key == "os.MkdirAll" || key == "os/exec.LookPath" || key == "os.Getenv" || key == "runtime.ReadMemStats" || key == "time.Now"
			keys := call.stringArguments()
			needsKeys := key == "os.Getenv" || key == "os/exec.LookPath"
			allowed = allowed || key == "math/rand.Perm"
			if !allowed || u.Context == nil || u.Context.Adapter == "" || !token.IsIdentifier(call.Method) || call.Method == "_" || seenHostCalls[key] || needsKeys != (len(keys) != 0) || (call.StringArgument != "" && len(call.StringArguments) != 0) {
				return fmt.Errorf("invalid host call binding: %s", key)
			}
			if len(call.StreamMethods) != 0 {
				if key != "os.Open" {
					return fmt.Errorf("stream methods require os.Open")
				}
				seen := map[string]bool{}
				for _, name := range call.StreamMethods {
					if !hostStreamMethod(name) || seen[name] {
						return fmt.Errorf("invalid host factory stream method: %s", name)
					}
					seen[name] = true
				}
				if !seen["Read"] || !seen["Close"] {
					return fmt.Errorf("input stream requires Read and Close")
				}
			}
			seenKeys := map[string]bool{}
			for _, value := range keys {
				if value == "" || seenKeys[value] {
					return fmt.Errorf("invalid host call argument: %s", key)
				}
				seenKeys[value] = true
			}
			seenHostCalls[key] = true
		}
		seenMethodCalls := map[string]bool{}
		for _, r := range u.MethodCalls {
			typ, method, ok := strings.Cut(r.Method, ".")
			if u.Context == nil || !safePath(r.Source) || portPath(r.Source) == "" || !ok || !token.IsIdentifier(typ) || !token.IsIdentifier(method) || !memberPath(r.Adapter) || seenMethodCalls[r.Source+":"+r.Method] {
				return fmt.Errorf("invalid method call binding")
			}
			seenMethodCalls[r.Source+":"+r.Method] = true
		}
		for _, call := range u.Calls {
			if u.Context == nil || !safePath(call.Source) || portPath(call.Source) == "" || !token.IsIdentifier(call.Function) || !memberPath(call.Method) || call.KeepDefinition && strings.TrimPrefix(path.Dir(call.Source), "src/") != u.sourcePackage() {
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
