package port

// Runtime ABI declarations stay byte-for-byte native. Only the compiler's
// target layout and write cursors acquire compilation ownership.
func runtimeABIMigration() []Unit {
	return []Unit{
		{Source: "src/internal/abi/type.go", Declarations: []string{"Type", "Kind", "Invalid", "TFlag", "TFlagUncommon", "NameOff", "TypeOff", "TextOff", "Method", "UncommonType", "Imethod", "ArrayType", "ChanDir", "RecvDir", "ChanType", "InterfaceType", "SliceType", "FuncType", "PtrType", "StructField", "StructType", "Name"}},
		{Source: "src/internal/abi/map.go", Declarations: []string{"MapType", "MapGroupSlotsBits", "MapNeedKeyUpdate"}},
		{Source: "src/internal/abi/iface.go", Declarations: []string{"ITab"}},
		{Source: "src/internal/abi/switch.go", Declarations: []string{"InterfaceSwitch", "InterfaceSwitchCache", "InterfaceSwitchCacheEntry", "TypeAssert", "TypeAssertCache", "TypeAssertCacheEntry"}},
		{Source: "src/internal/abi/compiletype.go", WholeFile: true},
	}
}

func rttypeMigration() []Unit {
	unit := Unit{
		Source: "src/cmd/compile/internal/rttype/rttype.go", WholeFile: true,
		Context:     &ContextRule{Type: "Context", Constructor: "newContext", Adapter: "adapter_context.go", Selective: true, Fields: map[string]string{}},
		Owner:       &OwnerRule{Types: []string{"Cursor"}, Context: "Context", Field: "compilation"},
		ContextFrom: map[string]ContextSource{"ArrayCursor.Elem": {Argument: "a", Resolver: "arrayContext"}},
		Checks:      map[string]string{"Init": "checkInit", "NewCursor": "checkCursor", "NewArrayCursor": "checkArrayCursor"},
		State: []StateRule{
			{Source: "src/cmd/compile/internal/base/link.go", Variable: "Ctxt", Field: "Writer.Link"},
			{Source: "src/cmd/compile/internal/types/size.go", Variable: "PtrSize", Field: "ptrSize"},
			{Source: "src/cmd/compile/internal/types/type.go", Variable: "Types", Field: "Types.Types"},
		},
		Calls: []CallRule{
			{Source: "src/cmd/compile/internal/base/print.go", Function: "Fatalf", Method: "fatalf"},
			{Source: "src/cmd/compile/internal/types/size.go", Function: "CalcSize", Method: "Types.CalcSize"},
			{Source: "src/cmd/compile/internal/types/type.go", Function: "NewStruct", Method: "newStruct"},
		},
	}
	for _, name := range []string{"Type", "ArrayType", "ChanType", "FuncType", "InterfaceType", "MapType", "PtrType", "SliceType", "StructType", "IMethod", "Method", "StructField", "UncommonType", "InterfaceSwitch", "TypeAssert", "ITab"} {
		unit.Context.Fields[name] = name
	}
	for _, name := range []string{"NewPtr", "NewSlice", "NewArray", "NewChan"} {
		unit.Calls = append(unit.Calls, CallRule{Source: "src/cmd/compile/internal/types/type.go", Function: name, Method: "Types." + name})
	}
	for _, name := range []string{"Uintptr", "Uint32", "Uint16", "Uint8", "Bool", "SymPtr", "SymPtrWeak", "SymPtrOff", "SymPtrWeakOff"} {
		unit.Calls = append(unit.Calls, CallRule{Source: "src/cmd/compile/internal/objw/objw.go", Function: name, Method: "Writer." + name})
	}
	return []Unit{unit}
}
