package amd64

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"go-backend-ssa/internal/cmd/bio"
	"go-backend-ssa/internal/cmd/goobj"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/dwarfgen"
	"go-backend-ssa/internal/gc"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/noder"
	"go-backend-ssa/internal/pkginit"
	"go-backend-ssa/internal/ssagen"
	"go-backend-ssa/internal/staticinit"
	"go-backend-ssa/internal/types"
	"go-backend-ssa/internal/types2"
)

type objectOutput struct {
	data                  []byte
	offset                int64
	writeError, seekError error
}

func (b *objectOutput) Write(p []byte) (int, error) {
	if b.writeError != nil {
		return 0, b.writeError
	}
	end := b.offset + int64(len(p))
	if end > int64(len(b.data)) {
		b.data = append(b.data, make([]byte, end-int64(len(b.data)))...)
	}
	copy(b.data[b.offset:end], p)
	b.offset = end
	return len(p), nil
}
func (b *objectOutput) Seek(n int64, whence int) (int64, error) {
	if b.seekError != nil {
		return 0, b.seekError
	}
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		n += b.offset
	case io.SeekEnd:
		n += int64(len(b.data))
	default:
		return 0, fmt.Errorf("invalid whence")
	}
	if n < 0 {
		return 0, fmt.Errorf("negative offset")
	}
	b.offset = n
	return n, nil
}
func objectBackend() *ssagen.Context {
	c := testBackend(true)
	dwarfgen.NewContext(c, c.SSA.ReflectData)
	_, pp := flushFunction(c, "objectFunction")
	pp.Free()
	c.Typecheck.IR.Base.Ctxt.DwFixups.Finalize("example/p", false)
	gc.NewContext(c.SSA.ReflectData.Runtime.Writer).AddGCLocals()
	return c
}
func TestNativeObjectContexts(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			a, b := objectBackend(), objectBackend()
			prefix := []byte("prefix:")
			left := &objectOutput{data: bytes.Clone(prefix), offset: int64(len(prefix))}
			right := new(objectOutput)
			if err := a.SSA.ReflectData.Runtime.Writer.WriteObject(left); err != nil {
				t.Fatal(err)
			}
			saved := bytes.Clone(left.data)
			if err := b.SSA.ReflectData.Runtime.Writer.WriteObject(right); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(left.data[:len(prefix)], prefix) || !bytes.Equal(left.data[len(prefix):], right.data) || !bytes.Equal(left.data, saved) || left.offset != int64(len(left.data)) {
				t.Fatal("object output contexts or initial offsets crossed")
			}
			if !bytes.HasPrefix(right.data, []byte(goobj.Magic)) {
				t.Fatal("missing native object header")
			}
			// Reject repeated serialization before touching the next output stream.
			unused := new(objectOutput)
			func() {
				defer func() {
					if recover() == nil {
						t.Error("repeated object write accepted")
					}
				}()
				_ = a.SSA.ReflectData.Runtime.Writer.WriteObject(unused)
			}()
			if len(unused.data) != 0 {
				t.Fatal("rejected write emitted bytes")
			}
		})
	}
}
func TestObjectOutputErrors(t *testing.T) {
	for _, kind := range []string{"write", "seek"} {
		t.Run(kind, func(t *testing.T) {
			c := objectBackend()
			stream := new(objectOutput)
			failure := errors.New("host " + kind + " failure")
			if kind == "write" {
				stream.writeError = failure
			} else {
				stream.seekError = failure
			}
			err := c.SSA.ReflectData.Runtime.Writer.WriteObject(stream)
			if !errors.Is(err, failure) {
				t.Fatalf("host error lost: %v", err)
			}
			var native *bio.OutputError
			if !errors.As(err, &native) {
				t.Fatalf("expected preserved native I/O diagnostic, got %T", err)
			}
			// Failure on one compilation does not alter a new compiler instance.
			next := objectBackend()
			if err := next.SSA.ReflectData.Runtime.Writer.WriteObject(new(objectOutput)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type objectInput struct {
	*bytes.Reader
	closed bool
}

func (r *objectInput) Close() error { r.closed = true; return nil }
func TestObjectFileInputs(t *testing.T) {
	for _, kind := range []string{"ok", "missing", "size"} {
		t.Run(kind, func(t *testing.T) {
			c := objectBackend()
			link := c.Typecheck.IR.Base.Ctxt
			payload := []byte("explicit-object-file-input-payload")
			sym := link.Lookup("example/p.embedded")
			sym.Type = objabi.SRODATA
			sym.Size = int64(len(payload))
			fi := sym.NewFileInfo()
			fi.Name = "virtual/input"
			fi.Size = int64(len(payload))
			link.Data = append(link.Data, sym)
			input := &objectInput{Reader: bytes.NewReader(payload)}
			if kind != "missing" {
				link.OpenObjectInput = func(name string) (io.ReadCloser, error) {
					if name != fi.Name {
						t.Fatal(name)
					}
					return input, nil
				}
			}
			if kind == "size" {
				fi.Size++
			}
			stream := new(objectOutput)
			err := c.SSA.ReflectData.Runtime.Writer.WriteObject(stream)
			if kind == "ok" {
				if err != nil || !input.closed || !bytes.Contains(stream.data, payload) {
					t.Fatal(err, input.closed)
				}
			} else {
				if err == nil || link.Errors == 0 {
					t.Fatal("input diagnostic missing")
				}
				if kind == "missing" && !strings.Contains(err.Error(), "not configured") {
					t.Fatal(err)
				}
				if kind == "size" && !input.closed {
					t.Fatal("failed input not closed")
				}
			}
		})
	}
}
func TestObjectNumberingPreflight(t *testing.T) {
	c := objectBackend()
	link := c.Typecheck.IR.Base.Ctxt
	before := link.Text[0].PkgIdx
	func() {
		defer func() {
			if recover() == nil {
				t.Error("unconfigured numbering accepted")
			}
		}()
		link.NumberSyms()
	}()
	if link.Text[0].PkgIdx != before {
		t.Fatal("failed preflight numbered symbols")
	}
	out := new(objectOutput)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("unnumbered serialization accepted")
			}
		}()
		obj.WriteObjFile(link, bio.NewWriter(out))
	}()
	if len(out.data) != 0 {
		t.Fatal("unnumbered output written")
	}
	if err := c.SSA.ReflectData.Runtime.Writer.WriteObject(out); err != nil {
		t.Fatal(err)
	}
}

func globalFinalizer(t *testing.T, backend *ssagen.Context) *gc.Context {
	t.Helper()
	n, err := noder.NewContext(types2.NewContext(), backend.Typecheck)
	if err != nil {
		t.Fatal(err)
	}
	n.ReflectData = backend.SSA.ReflectData
	n.StaticInit = staticinit.NewContext(n.Typecheck, n.ReflectData)
	c := gc.NewContext(n.ReflectData.Runtime.Writer)
	c.BindPackage(pkginit.NewContext(n))
	return c
}
func TestObjectGlobalFinalization(t *testing.T) {
	for _, mode := range []string{"readonly", "asan", "linkname", "coverage"} {
		t.Run(mode, func(t *testing.T) {
			b := testBackend(true)
			dwarfgen.NewContext(b, b.SSA.ReflectData)
			c := globalFinalizer(t, b)
			g := b.Typecheck.IR
			u := g.Types
			n := g.NewNameAt(u.AutogeneratedPos, u.LocalPkg.Lookup("global"), u.Types[types.TINT64])
			n.Class = ir.PEXTERN
			switch mode {
			case "readonly":
				n.MarkReadonly()
			case "asan":
				g.Base.Flag.ASan = true
				c.Package.InstrumentGlobalsMap = map[string]ir.Node{"global": n}
			case "linkname":
				n.Sym().Linkname = "external.global"
			case "coverage":
				n.SetCoverageAuxVar(true)
				n.Linksym().Type = objabi.SCOVERAGE_COUNTER
			}
			c.DumpGlobal(n)
			s := n.Linksym()
			size := int64(8)
			if mode == "asan" {
				size += pkginit.GetRedzoneSizeForGlobal(size)
			}
			if !s.OnList() || s.Size != size || s.Gotype == nil {
				t.Fatal("global definition", s)
			}
			if mode == "readonly" && s.Type != objabi.SRODATA {
				t.Fatal(s.Type)
			}
			if mode == "coverage" && s.Type != objabi.SCOVERAGE_COUNTER {
				t.Fatal(s.Type)
			}
			if mode == "linkname" && s.Pkg != "_" {
				t.Fatal(s.Pkg)
			}
			other := testBackend(true)
			foreign := other.Typecheck.IR.NewNameAt(other.Typecheck.IR.Types.AutogeneratedPos, other.Typecheck.IR.Types.LocalPkg.Lookup("foreign"), other.Typecheck.IR.Types.Types[types.TINT64])
			func() {
				defer func() {
					if recover() == nil {
						t.Error("foreign global accepted")
					}
				}()
				c.DumpGlobal(foreign)
			}()
			if foreign.Linksym().OnList() {
				t.Fatal("foreign definition changed")
			}
		})
	}
}
