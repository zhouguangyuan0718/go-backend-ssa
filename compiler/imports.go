package compiler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
)

// ImportLoader supplies an archive only when the native importer or artifact
// validation needs it. A compilation calls each loader at most once, serially,
// and snapshots its bytes. Independent compilations may call it concurrently.
type ImportLoader func(context.Context) ([]byte, error)

type importInputs struct {
	mu        sync.Mutex
	ctx       context.Context
	data      map[string][]byte
	loaders   map[string]ImportLoader
	failures  map[string]error
	cancelled error
}

func newImportInputs(ctx context.Context, req Request) (*importInputs, error) {
	inputs := &importInputs{ctx: ctx, data: make(map[string][]byte, len(req.Imports)), loaders: maps.Clone(req.ImportLoaders), failures: make(map[string]error)}
	for _, path := range slices.Sorted(maps.Keys(inputs.loaders)) {
		if inputs.loaders[path] == nil {
			return nil, fmt.Errorf("dependency %q has a nil archive loader", path)
		}
		if _, ok := req.Imports[path]; ok {
			return nil, fmt.Errorf("dependency %q has both eager and lazy native archive inputs", path)
		}
		if _, ok := req.Artifacts[path]; ok {
			return nil, fmt.Errorf("dependency %q has both artifact and native archive loader inputs", path)
		}
	}
	for path, data := range req.Imports {
		inputs.data[path] = bytes.Clone(data)
	}
	return inputs, nil
}

func (inputs *importInputs) resolve(path string) ([]byte, bool, error) {
	inputs.mu.Lock()
	defer inputs.mu.Unlock()
	if data, ok := inputs.data[path]; ok {
		return data, true, nil
	}
	if err, ok := inputs.failures[path]; ok {
		return nil, true, err
	}
	load, ok := inputs.loaders[path]
	if !ok {
		return nil, false, nil
	}
	var data []byte
	err := inputs.ctx.Err()
	if err == nil {
		data, err = load(inputs.ctx)
	}
	if cancelled := inputs.ctx.Err(); cancelled != nil {
		inputs.cancelled = cancelled
		err = cancelled
	}
	if err != nil {
		inputs.failures[path] = err
		return nil, true, err
	}
	data = bytes.Clone(data)
	inputs.data[path] = data
	return data, true, nil
}

func (inputs *importInputs) failure() error {
	inputs.mu.Lock()
	defer inputs.mu.Unlock()
	var failures []error
	for _, path := range slices.Sorted(maps.Keys(inputs.failures)) {
		failures = append(failures, inputs.failures[path])
	}
	return errors.Join(failures...)
}
