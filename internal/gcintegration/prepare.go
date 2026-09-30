// Package gcintegration creates isolated Go source trees for replacement tests.
package gcintegration

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go-backend-ssa/internal/port"
	"go/format"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

//go:embed main.go.tmpl
var driver []byte

type Manifest struct {
	Upstream, Version, Library, LibraryRevision, Bootstrap string
	Packages                                               []string
	Files                                                  map[string]string
}

// Prepare never changes repo, root or bootstrap and never overwrites output.
// It exports the exact Git pin, copies matching bootstrap tools and generated
// source files, and vendors the complete production closure of compiler.
// Building and installing the replacement compiler are separate explicit steps.
func Prepare(ctx context.Context, repo, root, output, bootstrap string) (*Manifest, error) {
	var err error
	for _, p := range []*string{&repo, &root, &output, &bootstrap} {
		*p, err = filepath.Abs(*p)
		if err != nil {
			return nil, err
		}
	}
	pin := port.Baseline()
	source, err := port.OpenSource(ctx, repo, pin)
	if err != nil {
		return nil, err
	}
	version, err := os.ReadFile(filepath.Join(bootstrap, "VERSION"))
	if err != nil {
		return nil, err
	}
	if strings.SplitN(string(version), "\n", 2)[0] != pin.Tag {
		return nil, fmt.Errorf("bootstrap distribution must be %s", pin.Tag)
	}
	goenv := append(os.Environ(), "GOROOT="+bootstrap, "GOTOOLCHAIN=local", "GOWORK=off", "GOFLAGS=", "GOOS="+runtime.GOOS, "GOARCH="+runtime.GOARCH, "GOEXPERIMENT=", "CGO_ENABLED=0", "GOPROXY=off", "GOSUMDB=off")
	list := exec.CommandContext(ctx, filepath.Join(bootstrap, "bin/go"), "list", "-deps", "-json", "./compiler")
	list.Dir, list.Env = root, goenv
	var stderr bytes.Buffer
	list.Stderr = &stderr
	data, err := list.Output()
	if err != nil {
		return nil, fmt.Errorf("library dependency closure: %w: %s", err, &stderr)
	}
	m := &Manifest{Upstream: pin.Commit, Version: pin.Tag, Bootstrap: bootstrap, Files: map[string]string{}}
	contents := map[string][]byte{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	for {
		var pkg struct {
			ImportPath, Dir                                        string
			GoFiles, SFiles, CFiles, HFiles, SysoFiles, EmbedFiles []string
		}
		if err := decoder.Decode(&pkg); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(pkg.ImportPath, "go-backend-ssa/") {
			continue
		}
		rel, err := filepath.Rel(root, pkg.Dir)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("library package outside root: %s", pkg.Dir)
		}
		m.Packages = append(m.Packages, pkg.ImportPath)
		files := append(append(append(append(append(pkg.GoFiles, pkg.SFiles...), pkg.CFiles...), pkg.HFiles...), pkg.SysoFiles...), pkg.EmbedFiles...)
		for _, file := range files {
			key := filepath.ToSlash(filepath.Join(rel, file))
			b, err := os.ReadFile(filepath.Join(root, rel, file))
			if err != nil {
				return nil, err
			}
			contents[key] = b
		}
	}
	if len(m.Packages) == 0 {
		return nil, fmt.Errorf("no library compiler dependencies found")
	}
	sort.Strings(m.Packages)
	h := sha256.New()
	h.Write([]byte(pin.Commit))
	h.Write(driver)
	keys := make([]string, 0, len(contents))
	for key := range contents {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		sum := sha256.Sum256(contents[key])
		digest := hex.EncodeToString(sum[:])
		m.Files[key] = digest
		fmt.Fprintf(h, "\n%s\x00%s", key, digest)
	}
	m.Library = hex.EncodeToString(h.Sum(nil))
	rev := exec.CommandContext(ctx, "git", "-C", root, "rev-parse", "HEAD")
	if b, e := rev.Output(); e == nil {
		m.LibraryRevision = strings.TrimSpace(string(b))
	}
	if err = os.Mkdir(output, 0755); err != nil {
		return nil, err
	}
	archive := exec.CommandContext(ctx, "git", "-C", repo, "archive", "--format=tar", source.Pin.Commit)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return nil, err
	}
	archive.Stderr = &stderr
	if err = archive.Start(); err != nil {
		return nil, err
	}
	extractErr := extractTar(output, tar.NewReader(pipe))
	if extractErr != nil {
		_ = pipe.Close()
	}
	archiveErr := archive.Wait()
	if extractErr != nil {
		return nil, extractErr
	}
	if archiveErr != nil {
		return nil, fmt.Errorf("export pinned Go source: %w: %s", archiveErr, &stderr)
	}
	// The matching distribution contributes tools and generated, untracked Go
	// sources (zbootstrap/zversion etc.). Existing pinned source is never replaced.
	for _, tree := range []string{"bin", "pkg"} {
		if err = copyTree(filepath.Join(bootstrap, tree), filepath.Join(output, tree), false); err != nil {
			return nil, err
		}
	}
	if err = copyTree(filepath.Join(bootstrap, "src"), filepath.Join(output, "src"), true); err != nil {
		return nil, err
	}
	vendor := filepath.Join(output, "src/cmd/vendor/go-backend-ssa")
	for _, key := range keys {
		if err = write(filepath.Join(vendor, filepath.FromSlash(key)), contents[key], 0644); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"LICENSE", "PATENTS"} {
		b, e := os.ReadFile(filepath.Join(root, name))
		if e == nil {
			if err = write(filepath.Join(vendor, name), b, 0644); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(e) {
			return nil, e
		}
	}
	gomod := filepath.Join(output, "src/cmd/go.mod")
	b, err := os.ReadFile(gomod)
	if err != nil {
		return nil, err
	}
	b = bytes.Replace(b, []byte("\ngo 1.27\n"), []byte("\ngo 1.27.1\n"), 1)
	b = append(b, []byte("\nrequire go-backend-ssa v0.0.0\n")...)
	if err = os.WriteFile(gomod, b, 0644); err != nil {
		return nil, err
	}
	modules := filepath.Join(output, "src/cmd/vendor/modules.txt")
	b, err = os.ReadFile(modules)
	if err != nil {
		return nil, err
	}
	b = append(b, []byte("# go-backend-ssa v0.0.0\n## explicit; go 1.27.1\n"+strings.Join(m.Packages, "\n")+"\n")...)
	if err = os.WriteFile(modules, b, 0644); err != nil {
		return nil, err
	}
	mainSource, err := format.Source(bytes.ReplaceAll(driver, []byte("{{CONTENT_HASH}}"), []byte(m.Library)))
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(output, "src/cmd/compile/main.go"), mainSource, 0644); err != nil {
		return nil, err
	}
	b, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(output, "go-backend-ssa.json"), append(b, '\n'), 0644); err != nil {
		return nil, err
	}
	return m, nil
}

func write(path string, data []byte, mode fs.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, data, mode)
}
func extractTar(root string, r *tar.Reader) error {
	for {
		header, err := r.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(header.Name)
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(filepath.Clean(name), ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid archive path %q", name)
		}
		name = filepath.Join(root, name)
		switch header.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			if err = os.MkdirAll(name, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			data, err := io.ReadAll(r)
			if err != nil {
				return err
			}
			if err = write(name, data, fs.FileMode(header.Mode)&0777); err != nil {
				return err
			}
		case tar.TypeSymlink:
			target := filepath.FromSlash(header.Linkname)
			resolved := filepath.Clean(filepath.Join(filepath.Dir(name), target))
			rel, err := filepath.Rel(root, resolved)
			if err != nil || filepath.IsAbs(target) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("invalid archive symlink %q", header.Name)
			}
			if err = os.MkdirAll(filepath.Dir(name), 0755); err != nil {
				return err
			}
			if err = os.Symlink(target, name); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported archive entry %s type %d", header.Name, header.Typeflag)
		}
	}
}
func copyTree(from, to string, missingOnly bool) error {
	return filepath.WalkDir(from, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		if missingOnly {
			if _, err = os.Lstat(target); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if d.Type()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported bootstrap file %s", path)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return write(target, b, info.Mode().Perm()|0200)
	})
}
