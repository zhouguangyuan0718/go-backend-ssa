package noder

import (
	"go-backend-ssa/internal/cmd/src"
	"go-backend-ssa/internal/go/pkgbits"
	"go-backend-ssa/internal/ir"
	"go-backend-ssa/internal/types"
	"reflect"
	"sync"
	"testing"
)

func TestNativeReaderWriterMetadata(t *testing.T) {
	for _, syncFrames := range []int{-1, 0} {
		t.Run(string(rune('a'+syncFrames+1)), func(t *testing.T) {
			c := writerContext(t)
			c.Debug.SyncFrames = syncFrames
			source := "package p\n//line generated.go:40:3\ntype Box[T any] struct { V T }; func Id[T any](v T) T { return v }; var X=Id(Box[int]{V:3})"
			pkg, info, files := checked(t, c.Checker, source)
			data, err := c.WritePackage(pkg, info, files, nil)
			if err != nil {
				t.Fatal(err)
			}
			pr := c.newPkgReader(pkgbits.NewPkgDecoder(pkg.Path(), data))
			root := pr.newReader(pkgbits.SectionMeta, pkgbits.PublicRootIdx, pkgbits.SyncPublic)
			p := root.pkg()
			if p != c.Types.NewPkg(pkg.Path(), pkg.Name()) {
				t.Fatal("package identity")
			}
			if root.Version().Has(pkgbits.HasInit) {
				root.Bool()
			}
			var names []string
			for range root.Len() {
				ref := root.objInfo()
				n := pr.newReader(pkgbits.SectionName, ref.idx, pkgbits.SyncObject1)
				np, sym := n.qualifiedIdent()
				if np != p {
					t.Fatal("object package")
				}
				names = append(names, sym.Name)
				n.Code(pkgbits.SyncCodeObj)
			}
			root.Sync(pkgbits.SyncEOF)
			if !reflect.DeepEqual(names, pkg.Scope().Names()) {
				t.Fatalf("declaration refs: %v", names)
			}
			var sawPragma bool
			for i := range pr.NumElems(pkgbits.SectionPosBase) {
				b := pr.posBaseIdx(pkgbits.RelElemIdx(i))
				if pr.posBaseIdx(pkgbits.RelElemIdx(i)) != b {
					t.Fatal("position base cache")
				}
				if b.Filename() == "generated.go" || b.AbsFilename() == "/src/generated.go" {
					sawPragma = true
				}
				if b.Pos().Base() != b {
					if b.Line() != 40 || b.Col() != 3 {
						t.Fatal("line pragma coordinates")
					}
					sawPragma = true
				}
			}
			if !sawPragma {
				t.Fatal("missing writer line pragma")
			}
			// Reading the same bitstream twice shares native package identity inside one
			// compiler, but keeps reader-local position caches separate.
			second := c.newPkgReader(pkgbits.NewPkgDecoder(pkg.Path(), data))
			if second.pkgIdx(0) != pr.pkgIdx(0) || second.posBaseIdx(0) == pr.posBaseIdx(0) {
				t.Fatal("reader cache lifetime")
			}
		})
	}
}

func TestNativeReaderInliningPositions(t *testing.T) {
	c := writerContext(t)
	r := &reader{compilation: c, inlCall: &ir.CallExpr{}, inlTreeIndex: 7, inlPosBases: map[*src.PosBase]*src.PosBase{}}
	b := src.NewFileBase("f.go", "/src/f.go")
	x := c.Ctxt.PosTable.XPos(src.MakePos(b, 3, 4))
	inlined := r.inlPos(x)
	p := c.Ctxt.PosTable.Pos(inlined)
	if p.Base().InliningIndex() != 7 || p.Line() != 3 || p.Col() != 4 || p.Base() == b || r.inlPos(x) != inlined {
		t.Fatal("inline position identity")
	}
	r.suppressInlPos = 1
	if r.inlPos(x) != x {
		t.Fatal("original position suppression")
	}
	r.suppressInlPos = 0
	defer func() {
		if recover() == nil {
			t.Fatal("already inlined base accepted")
		}
	}()
	r.inlPosBase(p.Base())
}

func TestConcurrentReaderCachesRetainIdentity(t *testing.T) {
	const count = 12
	contexts := make([]*Context, count)
	readers := make([]*pkgReader, count)
	packages := make([]*types.Pkg, count)
	var wg sync.WaitGroup
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := writerContext(t)
			contexts[i] = c
			pkg, info, files := checked(t, c.Checker, "package p; var X int")
			data, err := c.WritePackage(pkg, info, files, nil)
			if err != nil {
				t.Error(err)
				return
			}
			pr := c.newPkgReader(pkgbits.NewPkgDecoder(pkg.Path(), data))
			readers[i] = pr
			packages[i] = pr.newReader(pkgbits.SectionMeta, pkgbits.PublicRootIdx, pkgbits.SyncPublic).pkg()
			pr.posBaseIdx(0)
		}()
	}
	wg.Wait()
	for i, c := range contexts {
		if readers[i] == nil {
			continue
		}
		if packages[i] != c.Types.NewPkg("example/p", "p") || readers[i].posBaseIdx(0) != readers[i].posBases[0] {
			t.Fatal("retained reader identity")
		}
		if i > 0 && (packages[i] == packages[0] || readers[i].posBaseIdx(0) == readers[0].posBaseIdx(0)) {
			t.Fatal("reader state shared")
		}
	}
}
