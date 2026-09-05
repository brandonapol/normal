package engine

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

type FaultKind string

const (
	FaultRead    FaultKind = "read"
	FaultWrite   FaultKind = "write"
	FaultRemove  FaultKind = "remove"
	FaultRestart FaultKind = "restart"
	FaultStatus  FaultKind = "status"
)

type Fault struct {
	Kind   FaultKind
	Target string
	Error  *IOError
	Times  int
}

type faultSet struct {
	mu     sync.Mutex
	faults []Fault
}

func newFaultSet(faults []Fault) *faultSet {
	copied := make([]Fault, len(faults))
	copy(copied, faults)
	return &faultSet{faults: copied}
}

func (f *faultSet) match(kind FaultKind, target string) *IOError {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, fault := range f.faults {
		if fault.Kind != kind || fault.Target != target {
			continue
		}
		if fault.Times > 0 {
			f.faults[i].Times--
			if f.faults[i].Times == 0 {
				f.faults = append(f.faults[:i], f.faults[i+1:]...)
			}
		}
		return fault.Error
	}
	return nil
}

type MemoryFileSystem struct {
	mu     sync.Mutex
	files  map[string]string
	faults *faultSet
}

func NewMemoryFileSystem(seed map[string]string, faults *faultSet) *MemoryFileSystem {
	files := make(map[string]string, len(seed))
	for path, contents := range seed {
		files[path] = contents
	}
	return &MemoryFileSystem{files: files, faults: faults}
}

func (m *MemoryFileSystem) Read(_ context.Context, path string) (string, error) {
	if err := m.faults.match(FaultRead, path); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	contents, ok := m.files[path]
	if !ok {
		return "", &IOError{Code: ErrNotFound, Target: path, Message: "no such file"}
	}
	return contents, nil
}

func (m *MemoryFileSystem) Write(_ context.Context, path, contents string) error {
	if err := m.faults.match(FaultWrite, path); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[path] = contents
	return nil
}

func (m *MemoryFileSystem) Remove(_ context.Context, path string) error {
	if err := m.faults.match(FaultRemove, path); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.files[path]; !ok {
		return &IOError{Code: ErrNotFound, Target: path, Message: "no such file"}
	}
	delete(m.files, path)
	return nil
}

func (m *MemoryFileSystem) Exists(_ context.Context, path string) (bool, error) {
	if err := m.faults.match(FaultRead, path); err != nil {
		return false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.files[path]
	return ok, nil
}

func (m *MemoryFileSystem) Snapshot() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]string, len(m.files))
	for path, contents := range m.files {
		out[path] = contents
	}
	return out
}

type MemoryServiceHost struct {
	mu        sync.Mutex
	states    map[string]ServiceState
	restarted []string
	faults    *faultSet
}

func NewMemoryServiceHost(initial map[string]ServiceState, faults *faultSet) *MemoryServiceHost {
	states := make(map[string]ServiceState, len(initial))
	for service, state := range initial {
		states[service] = state
	}
	return &MemoryServiceHost{states: states, faults: faults}
}

func (m *MemoryServiceHost) Restart(_ context.Context, service string) error {
	failure := m.faults.match(FaultRestart, service)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.restarted = append(m.restarted, service)
	if failure != nil {
		m.states[service] = ServiceFailed
		return failure
	}
	m.states[service] = ServiceRunning
	return nil
}

func (m *MemoryServiceHost) Status(_ context.Context, service string) (ServiceState, error) {
	if err := m.faults.match(FaultStatus, service); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.states[service]
	if !ok {
		return ServiceRunning, nil
	}
	return state, nil
}

func (m *MemoryServiceHost) Restarts() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.restarted))
	copy(out, m.restarted)
	return out
}

func (m *MemoryServiceHost) SetState(service string, state ServiceState) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.states[service] = state
}

type StubClock struct {
	mu      sync.Mutex
	start   time.Time
	counter int
	prefix  string
}

func NewStubClock(start time.Time, prefix string) *StubClock {
	return &StubClock{start: start, prefix: prefix}
}

func (c *StubClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.start.Add(time.Duration(c.counter) * time.Second)
}

func (c *StubClock) NextID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter++
	return fmt.Sprintf("%s-%04d", c.prefix, c.counter)
}

type MemoryLogger struct {
	mu     sync.Mutex
	events []LogEvent
}

func (l *MemoryLogger) Log(event LogEvent) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, event)
}

func (l *MemoryLogger) Events() []LogEvent {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]LogEvent, len(l.events))
	copy(out, l.events)
	return out
}

func (l *MemoryLogger) Kinds() []string {
	kinds := make([]string, 0)
	for _, event := range l.Events() {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

type MemoryPorts struct {
	Ports
	FS       *MemoryFileSystem
	Services *MemoryServiceHost
	Logger   *MemoryLogger
}

type MemoryOptions struct {
	Files    map[string]string
	Services map[string]ServiceState
	Faults   []Fault
}

func NewMemoryPorts(options MemoryOptions) MemoryPorts {
	faults := newFaultSet(options.Faults)
	fs := NewMemoryFileSystem(options.Files, faults)
	services := NewMemoryServiceHost(options.Services, faults)
	logger := &MemoryLogger{}
	clock := NewStubClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "txn")
	return MemoryPorts{
		Ports:    Ports{FS: fs, Services: services, Clock: clock, Logger: logger},
		FS:       fs,
		Services: services,
		Logger:   logger,
	}
}

func SortedPaths(files map[string]string) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}
