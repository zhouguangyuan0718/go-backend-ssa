// gcintegrate prepares an isolated pinned Go tree with a library-backed compiler.
package main

import (
	"context"
	"flag"
	"fmt"
	"go-backend-ssa/internal/gcintegration"
	"os"
)

func main() {
	repo := flag.String("repo", "", "local Go repository containing the exact upstream pin")
	root := flag.String("root", ".", "library source root")
	output := flag.String("out", "", "new isolated toolchain directory; must not exist")
	bootstrap := flag.String("bootstrap", "", "official Go 1.27.1 distribution used to build the replacement")
	flag.Parse()
	if flag.NArg() != 0 || *repo == "" || *output == "" || *bootstrap == "" {
		fmt.Fprintln(os.Stderr, "gcintegrate requires -repo, -out and -bootstrap")
		os.Exit(2)
	}
	manifest, err := gcintegration.Prepare(context.Background(), *repo, *root, *output, *bootstrap)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gcintegrate:", err)
		os.Exit(1)
	}
	fmt.Printf("prepared %s: upstream %s; library %s\n", *output, manifest.Upstream, manifest.Library)
}
