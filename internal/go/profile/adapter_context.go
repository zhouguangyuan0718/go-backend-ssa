package profile

// Context owns the native decoder dispatch tables and error identities. Parsed
// message objects retain this owner; independent parsing sessions share no state.
type Context struct {
	errMalformed, ErrNoData                                       error
	profileDecoder, valueTypeDecoder, sampleDecoder, labelDecoder []decoder
	mappingDecoder, locationDecoder, lineDecoder, functionDecoder []decoder
}

func NewContext() *Context { return newContext() }
