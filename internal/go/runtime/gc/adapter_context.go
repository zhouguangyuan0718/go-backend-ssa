package gc

// Context retains the runtime size-class table used by compiler allocation
// specialization. Its initializer is copied from the pinned runtime source.
type Context struct {
	SizeToSizeClass8 [SmallSizeMax/SmallSizeDiv + 1]uint8
}
