package buildcfg

// Context supplies the compiler target/version used in object headers. Every
// field is caller input; it does not read the host environment.
type Context struct {
	GOOS, GOARCH, Version string
	GO386                 string
	GOAMD64               int
	GOARM                 GoarmFeatures
	GOARM64               Goarm64Features
	GOMIPS, GOMIPS64      string
	GOPPC64, GORISCV64    int
	GOWASM                gowasmFeatures
}
