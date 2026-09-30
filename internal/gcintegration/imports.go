package gcintegration

import (
	"bytes"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

// The native std vendor resolver only vendors import paths with a dot in the
// first element. This private alias is used solely in the isolated toolchain;
// .invalid cannot accidentally resolve to a downloaded module.
const vendorModule = "go-backend-ssa.invalid"

func vendorPath(path string) string {
	return strings.Replace(path, "go-backend-ssa/", vendorModule+"/", 1)
}

// rewriteLibraryImports changes only parsed import literals, preserving every
// other byte (including comments, diagnostic strings and line positions).
func rewriteLibraryImports(name string, source []byte) ([]byte, error) {
	fs := token.NewFileSet()
	file, err := parser.ParseFile(fs, name, source, parser.ImportsOnly)
	if err != nil {
		return nil, err
	}
	var output bytes.Buffer
	last := 0
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		if !strings.HasPrefix(path, "go-backend-ssa/") {
			continue
		}
		start, end := fs.Position(spec.Path.Pos()).Offset, fs.Position(spec.Path.End()).Offset
		output.Write(source[last:start])
		output.WriteString(strconv.Quote(vendorPath(path)))
		last = end
	}
	output.Write(source[last:])
	return output.Bytes(), nil
}
