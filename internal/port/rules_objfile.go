package port

// Object serialization keeps native numbering, hashes and binary encoding.
func objectFileMigration(units []Unit) []Unit {
	const root = "src/cmd/internal/"
	unique := func(a, b []string) []string {
		seen := map[string]bool{}
		for _, n := range a {
			seen[n] = true
		}
		for _, n := range b {
			if !seen[n] {
				a = append(a, n)
				seen[n] = true
			}
		}
		return a
	}
	builtinContext := func(fields map[string]string) *ContextRule {
		return &ContextRule{Type: "Context", Constructor: "initialize", Initialize: true, Init: true, Selective: true, Adapter: "adapter_context.go", Fields: fields}
	}
	for i := range units {
		u := &units[i]
		switch u.Source {
		case root + "goobj/objfile.go":
			u.Functions = []string{"NewWriter"}
			u.Declarations = []string{"stringRefSize", "FingerprintType", "FingerprintType.IsZero", "PkgIdxNone", "BlkAutolib", "Header", "Magic", "Header.Write", "Header.Size", "ImportedPkg", "importedPkgSize", "ImportedPkg.Write", "Sym", "SymSize", "SymABIstatic", "ObjFlagShared", "SymFlagDupok", "SymFlagUsedInIface", "Sym.ABI", "Sym.Type", "Sym.Flag", "Sym.Flag2", "Sym.Siz", "Sym.Align", "Sym.Dupok", "Sym.Local", "Sym.Typelink", "Sym.Leaf", "Sym.NoSplit", "Sym.ReflectMethod", "Sym.IsGoType", "Sym.UsedInIface", "Sym.IsItab", "Sym.IsDict", "Sym.IsPkgInit", "Sym.IsLinkname", "Sym.IsLinknameStd", "Sym.ABIWrapper", "Sym.WasmExport", "Sym.SetName", "Sym.SetABI", "Sym.SetType", "Sym.SetFlag", "Sym.SetFlag2", "Sym.SetSiz", "Sym.SetAlign", "Sym.Write", "Sym.fromBytes", "SymRef", "SymRef.IsZero", "Hash64Type", "Hash64Size", "HashType", "HashSize", "Reloc", "RelocSize", "Reloc.Off", "Reloc.Siz", "Reloc.Type", "Reloc.Add", "Reloc.Sym", "Reloc.SetOff", "Reloc.SetSiz", "Reloc.SetType", "Reloc.SetAdd", "Reloc.SetSym", "Reloc.Set", "Reloc.Write", "Reloc.fromBytes", "Aux", "AuxSize", "AuxGotype", "Aux.Type", "Aux.Sym", "Aux.SetType", "Aux.SetSym", "Aux.Write", "Aux.fromBytes", "RefFlags", "RefFlagsSize", "RefFlags.Sym", "RefFlags.Flag", "RefFlags.Flag2", "RefFlags.SetSym", "RefFlags.SetFlag", "RefFlags.SetFlag2", "RefFlags.Write", "huge", "RefName", "RefNameSize", "RefName.Sym", "RefName.SetSym", "RefName.SetName", "RefName.Write", "Writer", "Writer.AddString", "Writer.stringOff", "Writer.StringRef", "Writer.RawString", "Writer.Bytes", "Writer.Uint64", "Writer.Uint32", "Writer.Uint16", "Writer.Uint8", "Writer.Offset"}
		case root + "goobj/funcinfo.go":
			u.WholeFile = true
			u.Declarations = nil
		case root + "obj/objfile.go":
			u.WholeFile = true
			u.Functions = nil
			u.ContextFrom = map[string]ContextSource{
				"TrimInlineHash":     {Argument: "name", Resolver: "inlineHashLink"},
				"WriteObjFile":       {Argument: "ctxt", Resolver: "dataLink"},
				"contentHash64":      {Argument: "s", Resolver: "statelessDataLink"},
				"writer.writeFile":   {Argument: "w", Resolver: "objectWriterLink"},
				"writer.contentHash": {Argument: "w", Resolver: "objectWriterLink"},
			}
			for _, name := range []string{"writer.Reloc", "writer.aux1", "writer.refFlags", "writer.refNames"} {
				u.ContextFrom[name] = ContextSource{Argument: "w", Resolver: "objectWriterLink"}
			}
			u.ContextFrom["genFuncInfoSyms"] = ContextSource{Argument: "ctxt", Resolver: "dataLink"}
			u.Checks = map[string]string{"WriteObjFile": "checkWriteObjFile"}
			u.HostCalls = []HostCallRule{{Package: "os", Function: "Open", Method: "openObjectInput"}}
		case root + "obj/sym.go":
			u.Context = objectDataContext()
			u.Checks = map[string]string{"Link.NumberSyms": "checkNumberSyms"}
			u.Declarations = unique(u.Declarations, []string{"Link.Int32Sym", "Link.Int64Sym", "Link.Int128Sym", "Link.NumberSyms", "StaticNamePrefix", "traverseFlag", "traverseDefs", "Link.traverseSyms", "Link.traverseFuncAux", "Link.traverseAuxSyms"})
			u.Functions = append(u.Functions, "isNonPkgSym")
			u.Calls = append(u.Calls, CallRule{Source: root + "goobj/builtin.go", Function: "BuiltinIdx", Method: "ObjectBuiltins.BuiltinIdx"})
		case root + "obj/link.go":
			u.Declarations = unique(u.Declarations, []string{"LSym.File", "LSym.NewFileInfo", "FileInfo", "LSym.VarInfo", "Attribute.Leaf", "Attribute.UsedInIface", "Attribute.ContentAddressable", "Attribute.IsPcdata", "Attribute.Indexed", "Attribute.MakeTypelink", "Attribute.ReflectMethod", "Attribute.IsPkgInit"})
		}
	}
	units = append(units,
		Unit{Source: root + "bio/buf.go", Declarations: []string{"Writer", "Writer.MustSeek", "Writer.Offset"}, HostFields: []HostFieldRule{{Type: "Writer", Field: "f", Methods: []string{"Seek"}}}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Selective: true, RedirectExit: true, Adapter: "adapter_context.go"}, ContextFrom: map[string]ContextSource{"Writer.MustSeek": {Argument: "w", Resolver: "writerContext"}, "Writer.Offset": {Argument: "w", Resolver: "writerContext"}}},
		Unit{Source: root + "goobj/builtin.go", WholeFile: true, Context: builtinContext(map[string]string{"builtinMap": "builtinMap"}), State: []StateRule{{Source: "src/internal/buildcfg/exp.go", Variable: "Experiment", Field: "experiment"}}},
		Unit{Source: root + "goobj/builtinlist.go", WholeFile: true, Context: builtinContext(map[string]string{"builtins": "builtins"})},
	)
	units = append(units, Unit{Source: "src/cmd/compile/internal/gc/obj.go", Functions: []string{"addGCLocals", "dumpGlobal", "dumpGlobalConst", "ggloblnod"}, Context: &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true}, State: []StateRule{{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Writer.Link"}, {Source: "src/cmd/compile/internal/base/flag.go", Variable: "Flag", Field: "Package.Noder.Typecheck.IR.Base.Flag"}, {Source: "src/cmd/compile/internal/types/fmt.go", Variable: "LocalPkg", Field: "Writer.Types.LocalPkg"}, {Source: "src/cmd/compile/internal/types/type.go", Variable: "Types", Field: "Writer.Types.Types"}, {Source: "src/cmd/compile/internal/pkginit/initAsanGlobals.go", Variable: "InstrumentGlobalsMap", Field: "Package.InstrumentGlobalsMap"}}, Calls: []CallRule{{Source: "src/cmd/compile/internal/objw/objw.go", Function: "Global", Method: "Writer.Global"}, {Source: "src/cmd/compile/internal/types/size.go", Function: "CalcSize", Method: "Writer.Types.CalcSize"}, {Source: "src/cmd/compile/internal/types/type.go", Function: "TypeSymName", Method: "Writer.Types.TypeSymName"}, {Source: "src/cmd/compile/internal/reflectdata/reflect.go", Function: "TypeLinksym", Method: "Package.Noder.ReflectData.TypeLinksym"}, {Source: "src/cmd/compile/internal/ir/const.go", Function: "ConstOverflow", Method: "Package.Noder.Typecheck.IR.ConstOverflow"}, {Source: "src/cmd/compile/internal/ir/val.go", Function: "IntVal", Method: "Package.Noder.Typecheck.IR.IntVal"}, {Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"}}})
	// The buffered stream must exist before the native binary object records.
	var out []Unit
	for _, u := range units {
		if u.sourcePackage() == "cmd/internal/bio" {
			out = append(out, u)
		}
	}
	for _, u := range units {
		if u.sourcePackage() != "cmd/internal/bio" {
			out = append(out, u)
		}
	}
	return out
}
