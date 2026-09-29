package port

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/types"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const ledgerPath = "sync/generated.json"

type Generated struct {
	SchemaVersion  int               `json:"schema_version"`
	Tool           string            `json:"tool"`
	Commit         string            `json:"commit"`
	PlanSHA256     string            `json:"plan_sha256"`
	Files          []Provenance      `json:"files"`
	SupportSources map[string]string `json:"support_sources,omitempty"`
}
type Provenance struct {
	Output           string            `json:"output"`
	Source           string            `json:"source"`
	SourceSHA256     string            `json:"source_sha256"`
	OutputSHA256     string            `json:"output_sha256"`
	Rule             string            `json:"rule"`
	Declarations     []string          `json:"declarations,omitempty"`
	AdapterSHA256    string            `json:"adapter_sha256,omitempty"`
	SignatureSources map[string]string `json:"signature_sources,omitempty"`
}

type Rendered struct {
	Files  map[string][]byte
	Ledger Generated
}

func JSON(v any) ([]byte, error) { b, e := json.MarshalIndent(v, "", "  "); return append(b, '\n'), e }

func Render(s *Source, p Plan, root string) (*Rendered, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if err := auditProductionFiles(root, p); err != nil {
		return nil, err
	}
	planJSON, err := JSON(p)
	if err != nil {
		return nil, err
	}
	r := &Rendered{Files: map[string][]byte{}, Ledger: Generated{SchemaVersion: 1, Tool: ToolVersion, Commit: s.Pin.Commit, PlanSHA256: hash(planJSON)}}
	before, after := newImporter(), newImporter()
	imports := p.importMap(s.Pin.TargetModule)
	after.support = map[string]map[string][]byte{}
	if len(p.SupportFiles) > 0 {
		r.Ledger.SupportSources = map[string]string{}
	}
	for _, name := range p.SupportFiles {
		if err := checkPath(root, name); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		pkg := s.Pin.TargetModule + "/" + path.Dir(name)
		for _, dest := range imports {
			if pkg == dest {
				return nil, fmt.Errorf("support file overlaps migrated package: %s", name)
			}
		}
		if after.support[pkg] == nil {
			after.support[pkg] = map[string][]byte{}
		}
		after.support[pkg][name] = data
		r.Ledger.SupportSources[name] = hash(data)
	}
	// A package is checked as a unit, in first-appearance dependency order.
	var order []string
	groups := make(map[string][]Unit)
	for _, u := range p.Units {
		pkg := u.sourcePackage()
		if _, ok := groups[pkg]; !ok {
			order = append(order, pkg)
		}
		groups[pkg] = append(groups[pkg], u)
	}
	for _, pkg := range order {
		files, provenance, err := renderPackage(s, groups[pkg], imports, root, before, after)
		if err != nil {
			return nil, err
		}
		for name, data := range files {
			r.Files[name] = data
		}
		r.Ledger.Files = append(r.Ledger.Files, provenance...)
	}
	for _, pkg := range sortedKeys(after.support) {
		if _, err := after.Import(pkg); err != nil {
			return nil, fmt.Errorf("support package %s: %w", pkg, err)
		}
	}
	for _, name := range []string{"LICENSE", "PATENTS"} {
		b, err := s.Read(name)
		if err != nil {
			return nil, err
		}
		dest := "third_party/go/" + name
		r.Files[dest] = b
		r.Ledger.Files = append(r.Ledger.Files, Provenance{Output: dest, Source: name, SourceSHA256: hash(b), OutputSHA256: hash(b), Rule: "preserve-upstream-notice-v1"})
	}
	ledger, err := JSON(r.Ledger)
	if err != nil {
		return nil, err
	}
	r.Files[ledgerPath] = ledger
	return r, nil
}

// Every production file in the small generated packages is explicitly owned.
// This prevents an unlisted handwritten file from reintroducing global state.
func auditProductionFiles(root string, p Plan) error {
	old, err := readLedger(root)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	if old != nil {
		for _, f := range old.Files {
			allowed[f.Output] = true
		}
	}
	for _, u := range p.Units {
		allowed[u.output()] = true
		if u.Context != nil && u.Context.Adapter != "" {
			allowed[u.adapterPath()] = true
		}
	}
	for _, file := range p.SupportFiles {
		allowed[file] = true
	}
	dirs := map[string]bool{}
	for _, file := range p.SupportFiles {
		dirs[path.Dir(file)] = true
	}
	for _, u := range p.Units {
		dirs[path.Dir(u.output())] = true
	}
	for _, dir := range sortedKeys(dirs) {
		if err := checkPath(root, dir); err != nil {
			return err
		}
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			name := path.Join(dir, entry.Name())
			if strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") && !allowed[name] {
				return fmt.Errorf("unlisted production file: %s", name)
			}
		}
	}
	return nil
}

func auditOutput(pkg *types.Package, files []*ast.File, info *types.Info) error {
	for _, name := range pkg.Scope().Names() {
		if _, ok := pkg.Scope().Lookup(name).(*types.Var); ok {
			return fmt.Errorf("remaining package variable %s.%s", pkg.Path(), name)
		}
	}
	for _, f := range files {
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "init" {
				return fmt.Errorf("init not supported in generated slice")
			}
		}
		for _, c := range f.Comments {
			for _, line := range c.List {
				if strings.Contains(line.Text, "go:linkname") {
					return fmt.Errorf("linkname not supported")
				}
			}
		}
	}
	for _, obj := range info.Uses {
		if obj == nil || obj.Pkg() == nil {
			continue
		}
		p := obj.Pkg().Path()
		// Caller reads this invocation's stack, not mutable process settings.
		// Retain the upstream assertion's source-location diagnostic.
		if p == "runtime" && obj.Name() == "Caller" {
			if _, ok := obj.(*types.Func); ok {
				continue
			}
		}
		if p == "unsafe" || p == "os" || p == "os/exec" || p == "syscall" || p == "log" || p == "flag" || p == "runtime" {
			return fmt.Errorf("unclassified environment dependency %s.%s", p, obj.Name())
		}
	}
	return nil
}

func checkPath(root, rel string) error {
	if !safePath(rel) {
		return fmt.Errorf("unsafe output path %q", rel)
	}
	cur := root
	for _, part := range strings.Split(rel, "/") {
		cur = filepath.Join(cur, part)
		st, err := os.Lstat(cur)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing symlink path: %s", cur)
		}
	}
	return nil
}

func ownedPath(p string) bool {
	return safePath(p) && ((strings.HasPrefix(p, "internal/") && strings.HasSuffix(p, ".go") && !strings.HasPrefix(path.Base(p), "adapter_")) || p == "third_party/go/LICENSE" || p == "third_party/go/PATENTS")
}

func readLedger(root string) (*Generated, error) {
	if err := checkPath(root, ledgerPath); err != nil {
		return nil, err
	}
	var g Generated
	if err := readJSON(filepath.Join(root, ledgerPath), &g); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if g.SchemaVersion != 1 {
		return nil, fmt.Errorf("unsupported ownership ledger")
	}
	seen := map[string]bool{}
	for _, f := range g.Files {
		if !ownedPath(f.Output) || seen[f.Output] || len(f.OutputSHA256) != 64 {
			return nil, fmt.Errorf("invalid ownership entry %s", f.Output)
		}
		seen[f.Output] = true
	}
	return &g, nil
}

// Check compares regenerated bytes, including provenance, and rejects stale files.
func Check(root string, r *Rendered) error {
	old, err := readLedger(root)
	if err != nil {
		return err
	}
	for _, name := range sortedKeys(r.Files) {
		if err := checkPath(root, name); err != nil {
			return err
		}
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		if !bytes.Equal(b, r.Files[name]) {
			return fmt.Errorf("generated file differs: %s", name)
		}
	}
	if old != nil {
		for _, f := range old.Files {
			if _, ok := r.Files[f.Output]; !ok {
				return fmt.Errorf("stale generated file: %s", f.Output)
			}
		}
	}
	return nil
}

// Install preflights every destination, stages all bytes, and rolls back on
// ordinary I/O failures. Multi-file updates are not crash-atomic; a subsequent
// check detects interruption. A lock prevents simultaneous generator installs.
func Install(root string, r *Rendered) (err error) {
	if err = checkPath(root, ".cache/syncgo.lock"); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(root, ".cache"), 0755); err != nil {
		return err
	}
	lock := filepath.Join(root, ".cache/syncgo.lock")
	if err = os.Mkdir(lock, 0700); err != nil {
		return fmt.Errorf("acquire generator lock: %w", err)
	}
	defer os.Remove(lock)
	old, err := readLedger(root)
	if err != nil {
		return err
	}
	owned := map[string]string{}
	if old != nil {
		for _, f := range old.Files {
			owned[f.Output] = f.OutputSHA256
		}
	}
	all := map[string]bool{}
	for name := range r.Files {
		if name != ledgerPath && !ownedPath(name) {
			return fmt.Errorf("unowned output: %s", name)
		}
		all[name] = true
	}
	for name := range owned {
		all[name] = true
	}
	backup := map[string][]byte{}
	for _, name := range sortedKeys(all) {
		if err = checkPath(root, name); err != nil {
			return err
		}
		b, e := os.ReadFile(filepath.Join(root, name))
		if e != nil && !errors.Is(e, os.ErrNotExist) {
			return e
		}
		if e == nil {
			if name != ledgerPath && (owned[name] == "" || hash(b) != owned[name]) {
				return fmt.Errorf("refusing to overwrite unowned or edited file: %s", name)
			}
			backup[name] = b
		} else if owned[name] != "" {
			return fmt.Errorf("owned file is missing: %s", name)
		}
	}
	if err = checkPath(root, "_work"); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(root, "_work"), 0755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Join(root, "_work"), "syncgo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	for name, b := range r.Files {
		p := filepath.Join(stage, name)
		if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
		if err = os.WriteFile(p, b, 0644); err != nil {
			return err
		}
	}
	var changed []string
	defer func() {
		if err == nil {
			return
		}
		for i := len(changed) - 1; i >= 0; i-- {
			name := changed[i]
			p := filepath.Join(root, name)
			if b, ok := backup[name]; ok {
				if e := os.WriteFile(p, b, 0644); e != nil {
					err = errors.Join(err, e)
				}
			} else {
				if e := os.Remove(p); e != nil && !errors.Is(e, os.ErrNotExist) {
					err = errors.Join(err, e)
				}
			}
		}
	}()
	// Provenance is installed last, so check can detect an interrupted transaction.
	names := sortedKeys(all)
	order := make([]string, 0, len(names))
	for _, n := range names {
		if n != ledgerPath {
			order = append(order, n)
		}
	}
	order = append(order, ledgerPath)
	for _, name := range order {
		p := filepath.Join(root, name)
		if err = os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			return err
		}
		if _, ok := r.Files[name]; ok {
			err = os.Rename(filepath.Join(stage, name), p)
		} else {
			err = os.Remove(p)
		}
		if err != nil {
			return err
		}
		changed = append(changed, name)
	}
	return nil
}

// Only generated ownership metadata is read from JSON; migration rules are Go.
func readJSON(name string, dst any) error {
	f, err := os.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("%s: trailing JSON data", name)
	}
	return nil
}
