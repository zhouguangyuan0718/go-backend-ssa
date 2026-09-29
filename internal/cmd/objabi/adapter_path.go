package objabi

// PathContext supplies the one host setting read by the native AbsFile
// algorithm. Working directory and rewrites are already explicit parameters.
// Paths retain the native build host's filepath semantics.
type PathContext struct{ GOROOT string }
