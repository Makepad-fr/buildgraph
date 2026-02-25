package backend

import (
	"fmt"
	"sort"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	backends map[string]Backend
}

func NewRegistry() *Registry {
	return &Registry{backends: map[string]Backend{}}
}

func (r *Registry) Register(b Backend) error {
	if b == nil {
		return fmt.Errorf("backend cannot be nil")
	}
	name := b.Name()
	if name == "" {
		return fmt.Errorf("backend name cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.backends[name]; ok {
		return fmt.Errorf("backend %q already registered", name)
	}
	r.backends[name] = b
	return nil
}

func (r *Registry) Get(name string) (Backend, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	b, ok := r.backends[name]
	return b, ok
}

func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.backends))
	for name := range r.backends {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
