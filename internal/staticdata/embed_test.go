package staticdata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"go-backend-ssa/internal/cmd/hash"
	"go-backend-ssa/internal/cmd/obj"
	"go-backend-ssa/internal/cmd/objabi"
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"io"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

type observedFile struct {
	fs.File
	closed               int
	readError, statError error
	size                 *int64
}

func (f *observedFile) Close() error { f.closed++; return f.File.Close() }
func (f *observedFile) Read(p []byte) (int, error) {
	if f.readError != nil {
		return 0, f.readError
	}
	return f.File.Read(p)
}
func (f *observedFile) Stat() (fs.FileInfo, error) {
	if f.statError != nil {
		return nil, f.statError
	}
	i, e := f.File.Stat()
	if e == nil && f.size != nil {
		return sizedInfo{i, *f.size}, nil
	}
	return i, e
}

type sizedInfo struct {
	fs.FileInfo
	size int64
}

func (i sizedInfo) Size() int64 { return i.size }

func TestNativeFileInputs(t *testing.T) {
	for i := range 8 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			c := testContext(8, binary.LittleEndian)
			for _, size := range []int{0, 1, 1024, 1025, 8192} {
				data := bytes.Repeat([]byte{byte(i + 1)}, size)
				input := fstest.MapFS{"input": &fstest.MapFile{Data: data}}
				var files []*observedFile
				c.OpenFile = func(path string) (fs.File, error) {
					f, e := input.Open(path)
					if e != nil {
						return nil, e
					}
					o := &observedFile{File: f}
					files = append(files, o)
					return o, nil
				}
				var saved *obj.LSym
				for _, readonly := range []bool{true, false, true, false} {
					digest := make([]byte, 16)
					s, n, e := c.fileStringSym(src.XPos{}, "input", readonly, digest)
					if e != nil || n != int64(size) {
						t.Fatal(n, e)
					}
					sum := hash.Sum32(data)
					want := sum[:]
					if size > 1024 {
						h := hash.New32()
						h.Write(data)
						want = h.Sum(nil)
					}
					if !bytes.Equal(digest, want[:16]) {
						t.Fatal("hash")
					}
					if size <= 1024 {
						if !bytes.Equal(s.P, data) || s.File() != nil {
							t.Fatal("inline bytes")
						}
					} else {
						if s.File() == nil || s.File().Name != "input" || s.File().Size != int64(size) || len(s.P) != 0 {
							t.Fatal("streamed bytes metadata")
						}
					}
					if readonly {
						if s.Type != objabi.SRODATA || !s.DuplicateOK() || (saved != nil && s != saved) {
							t.Fatal("readonly sharing")
						}
						saved = s
					} else if s == saved || s.Type != objabi.SNOPTRDATA {
						t.Fatal("mutable sharing")
					}
				}
				for _, f := range files {
					if f.closed != 1 {
						t.Fatal("input not closed exactly once", f.closed)
					}
				}
			}
		})
	}
}

func TestNativeFileFailures(t *testing.T) {
	marker := errors.New("input failed")
	for _, test := range []struct {
		name, content    string
		size             int64
		readErr, statErr error
		want             string
	}{
		{name: "stat", statErr: marker}, {name: "small-read", content: "abc", readErr: marker}, {name: "large-read", content: strings.Repeat("x", 2048), readErr: marker},
		{name: "small-changed", content: "abc", size: 2, want: "file changed between reads"}, {name: "large-changed", content: strings.Repeat("x", 2048), size: 2049, want: "file changed between reads"}, {name: "too-large", size: maxFileSize + 1, want: "file too large"},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := testContext(8, binary.LittleEndian)
			raw, _ := (fstest.MapFS{"input": &fstest.MapFile{Data: []byte(test.content)}}).Open("input")
			f := &observedFile{File: raw, readError: test.readErr, statError: test.statErr}
			if test.size != 0 {
				f.size = &test.size
			}
			c.OpenFile = func(string) (fs.File, error) { return f, nil }
			s, n, e := c.fileStringSym(src.XPos{}, "input", true, nil)
			if s != nil || n != 0 || e == nil || f.closed != 1 {
				t.Fatal(s, n, e, f.closed)
			}
			if test.want != "" {
				if !strings.Contains(e.Error(), test.want) {
					t.Fatal(e)
				}
			} else if e != marker {
				t.Fatal("error identity", e)
			}
		})
	}
	c := testContext(8, binary.LittleEndian)
	if _, _, e := c.fileStringSym(src.XPos{}, "/etc/passwd", true, nil); e == nil || !strings.Contains(e.Error(), "not configured") {
		t.Fatal("ambient read", e)
	}
	c.OpenFile = func(string) (fs.File, error) { return nil, marker }
	if _, _, e := c.fileStringSym(src.XPos{}, "input", true, nil); e != marker {
		t.Fatal("open error", e)
	}
	directory, _ := (fstest.MapFS{"dir": &fstest.MapFile{Mode: fs.ModeDir}}).Open("dir")
	f := &observedFile{File: directory}
	c.OpenFile = func(string) (fs.File, error) { return f, nil }
	if _, _, e := c.fileStringSym(src.XPos{}, "dir", true, nil); e == nil || e.Error() != "not a regular file" || f.closed != 1 {
		t.Fatal(e)
	}
}

func embedName(c *Context, name string, typ *types.Type, patterns ...string) *ir.Name {
	n := global(c, name, typ)
	n.Embed = &[]ir.Embed{{Patterns: patterns}}
	return n
}
func TestNativeEmbedLayout(t *testing.T) {
	for _, size := range []int{4, 8} {
		for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
			c := testContext(size, order)
			u := c.IR.Types
			input := fstest.MapFS{"a.txt": &fstest.MapFile{Data: []byte("alpha")}, "dir/b.txt": &fstest.MapFile{Data: bytes.Repeat([]byte("b"), 2048)}}
			c.OpenFile = input.Open
			c.Writer.Link.OpenObjectInput = func(p string) (io.ReadCloser, error) { return input.Open(p) }
			c.IR.Base.Flag.Cfg.Embed.Patterns = map[string][]string{"all": {"dir/b.txt", "a.txt", "a.txt"}, "one": {"a.txt"}}
			c.IR.Base.Flag.Cfg.Embed.Files = map[string]string{"a.txt": "a.txt", "dir/b.txt": "dir/b.txt"}
			s := embedName(c, "String", u.Types[types.TSTRING], "one")
			b := embedName(c, "Bytes", u.NewSlice(u.Types[types.TUINT8]), "one")
			c.WriteEmbed(s)
			c.WriteEmbed(b)
			if string(s.Linksym().R[0].Sym.P) != "alpha" || string(b.Linksym().R[0].Sym.P) != "alpha" || s.Linksym().R[0].Sym == b.Linksym().R[0].Sym {
				t.Fatal("string/bytes")
			}
			decl := c.IR.NewDeclNameAt(src.XPos{}, ir.OTYPE, u.NewPkg("embed", "embed").Lookup("FS"))
			typ := u.NewNamed(decl)
			decl.SetType(typ)
			decl.SetTypecheck(1)
			typ.SetUnderlying(u.NewStruct([]*types.Field{u.NewField(src.XPos{}, nil, u.NewPtr(u.Types[types.TUINT8]))}))
			n := embedName(c, "FS", typ, "all")
			c.WriteEmbed(n)
			if c.IR.Diagnostics.HasErrors() {
				t.Fatal(c.IR.Diagnostics.Diagnostics())
			}
			table := n.Linksym().R[0].Sym
			if table.Size != int64(3*size+3*(4*size+16)) || len(table.R) != 6 || table.R[0].Sym != table || table.R[0].Add != int64(3*size) {
				t.Fatal("FS table", table.Size, table.R)
			}
			var names []string
			for _, r := range table.R[1:] {
				if strings.HasPrefix(r.Sym.Name, "go:string.") && r.Off >= int32(3*size) && (int(r.Off)-3*size)%(4*size+16) == 0 {
					names = append(names, string(r.Sym.P))
				}
			}
			if fmt.Sprint(names) != "[a.txt dir/ dir/b.txt]" {
				t.Fatal("FS ordering", names)
			}
			offset := 3*size + 4*size
			sum := hash.Sum32([]byte("alpha"))
			if !bytes.Equal(table.P[offset:offset+16], sum[:16]) {
				t.Fatal("FS content hash")
			}
		}
	}
}
