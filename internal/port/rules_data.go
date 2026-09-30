package port

// dataMigration preserves the complete native package DATA finalization order.
func dataMigration(units []Unit) []Unit {
	const root = "src/cmd/compile/internal/"
	for i := range units {
		u := &units[i]
		if u.Source != root+"gc/obj.go" {
			continue
		}
		u.Functions = append(u.Functions, "dumpdata", "dumpembeds")
		u.State = append(u.State, StateRule{Source: root + "reflectdata/reflect.go", Variable: "ZeroSize", Field: "Package.Noder.ReflectData.ZeroSize"})
		for _, x := range []struct{ file, name, method string }{
			{"reflectdata/reflect.go", "WriteGCSymbols", "Package.Noder.ReflectData.WriteGCSymbols"},
			{"reflectdata/reflect.go", "WritePluginTable", "Package.Noder.ReflectData.WritePluginTable"},
			{"staticdata/data.go", "WriteFuncSyms", "Package.Noder.ReflectData.StaticData.WriteFuncSyms"},
			{"staticdata/embed.go", "WriteEmbed", "Package.Noder.ReflectData.StaticData.WriteEmbed"},
			{"base/link.go", "PkgLinksym", "Package.Noder.Typecheck.IR.Base.PkgLinksym"},
		} {
			u.Calls = append(u.Calls, CallRule{Source: root + x.file, Function: x.name, Method: x.method})
		}
	}
	return units
}
