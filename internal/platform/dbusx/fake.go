// Fake: an in-memory D-Bus for adapter tests.

package dbusx

import (
	"context"
	"fmt"
	"sync"
)

// Fake is an in-memory Bus for adapter tests. Unset function fields answer
// with ErrUnavailable so a test only wires what it exercises.
type Fake struct {
	CallFn     func(ctx context.Context, dest, path, method string, args ...any) ([]any, error)
	PropertyFn func(ctx context.Context, dest, path, iface, name string) (any, error)
	NamesFn    func(ctx context.Context) ([]string, error)

	mu      sync.Mutex
	subs    []fakeSub
	closed  bool
	Calls   []string // "dest path method" in call order, for assertions
	Emitted int
}

type fakeSub struct {
	iface, member string
	ch            chan Signal
	ctx           context.Context
}

// Call implements Bus.
func (f *Fake) Call(ctx context.Context, dest, path, method string, args ...any) ([]any, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, dest+" "+path+" "+method)
	f.mu.Unlock()
	if f.CallFn == nil {
		return nil, fmt.Errorf("%w: fake has no CallFn", ErrUnavailable)
	}
	return f.CallFn(ctx, dest, path, method, args...)
}

// Property implements Bus.
func (f *Fake) Property(ctx context.Context, dest, path, iface, name string) (any, error) {
	f.mu.Lock()
	f.Calls = append(f.Calls, dest+" "+path+" "+iface+"."+name)
	f.mu.Unlock()
	if f.PropertyFn == nil {
		return nil, fmt.Errorf("%w: fake has no PropertyFn", ErrUnavailable)
	}
	return f.PropertyFn(ctx, dest, path, iface, name)
}

// Names implements Bus.
func (f *Fake) Names(ctx context.Context) ([]string, error) {
	if f.NamesFn == nil {
		return nil, fmt.Errorf("%w: fake has no NamesFn", ErrUnavailable)
	}
	return f.NamesFn(ctx)
}

// Subscribe implements Bus.
func (f *Fake) Subscribe(ctx context.Context, iface, member string) (<-chan Signal, error) {
	ch := make(chan Signal, 16)
	f.mu.Lock()
	f.subs = append(f.subs, fakeSub{iface: iface, member: member, ch: ch, ctx: ctx})
	f.mu.Unlock()
	go func() {
		<-ctx.Done()
		f.mu.Lock()
		defer f.mu.Unlock()
		for i, s := range f.subs {
			if s.ch == ch {
				f.subs = append(f.subs[:i], f.subs[i+1:]...)
				break
			}
		}
		close(ch)
	}()
	return ch, nil
}

// Emit delivers s to every live subscription whose interface (and member, when
// set) matches. Emission is best-effort: a full subscriber buffer drops it.
func (f *Fake) Emit(s Signal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Emitted++
	for _, sub := range f.subs {
		if !sigMatches(s.Name, sub.iface, sub.member) {
			continue
		}
		select {
		case sub.ch <- s:
		default:
		}
	}
}

func sigMatches(name, iface, member string) bool {
	if member != "" {
		return name == iface+"."+member
	}
	prefix := iface + "."
	return len(name) > len(prefix) && name[:len(prefix)] == prefix
}

// Close implements Bus.
func (f *Fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// Closed reports whether Close was called.
func (f *Fake) Closed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}
