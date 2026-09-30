// Package pgo exposes native raw pprof processing, preprocessed profile data
// and the compiler profile stream format. Each raw parse owns its decoder state.
package pgo

import (
	"bufio"
	compiler "go-backend-ssa/internal/cmd/pgo"
	"io"
)

type Profile = compiler.Profile
type NamedCallEdge = compiler.NamedCallEdge
type NamedEdgeMap = compiler.NamedEdgeMap

func IsSerialized(r *bufio.Reader) (bool, error)    { return compiler.IsSerialized(r) }
func FromSerialized(r io.Reader) (*Profile, error)  { return compiler.FromSerialized(r) }
func WeightInPercentage(value, total int64) float64 { return compiler.WeightInPercentage(value, total) }

func FromPProf(r io.Reader) (*Profile, error) { return compiler.NewContext().FromPProf(r) }
