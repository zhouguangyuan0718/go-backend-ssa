package pgo

import "go-backend-ssa/internal/go/profile"

// Context owns the native raw-profile parsing session. Serialized profile
// readers remain stateless, while raw readers have independent error identities.
type Context struct{ Profiles *profile.Context }

func NewContext() *Context { c := newContext(); c.Profiles = profile.NewContext(); return c }
