// Package actions holds the router-independent parts of the action contract
// (contracts/actions.md): per-session request de-duplication and server-side
// hold leases. Both run on an injected clock so tests stay deterministic.
package actions

import (
	"bytes"
	"encoding/json"
	"sync"
	"time"

	"bear-den-tv/internal/clock"
	"bear-den-tv/internal/contract"
)

// DedupWindow and DedupSize are the contract's replay-cache bounds: the last
// 256 request ids per device session, remembered for 60 seconds.
const (
	DedupWindow = 60 * time.Second
	DedupSize   = 256
)

type dedupEntry struct {
	payload []byte
	result  contract.ActionResult
	at      time.Time
}

type dedupSession struct {
	order   []string
	entries map[string]dedupEntry
}

// Dedup remembers recent request ids per sender so a retried request returns
// its original result and a reused id with a different payload is refused.
type Dedup struct {
	clock    clock.Clock
	mu       sync.Mutex
	sessions map[string]*dedupSession
}

// NewDedup returns an empty replay cache.
func NewDedup(c clock.Clock) *Dedup {
	if c == nil {
		c = clock.Real{}
	}
	return &Dedup{clock: c, sessions: map[string]*dedupSession{}}
}

// Verdict is the outcome of Dedup.Check.
type Verdict int

const (
	// Fresh means the id is unknown; route the request and Remember the result.
	Fresh Verdict = iota
	// Replay means the same id and payload were seen; return the stored result.
	Replay
	// Mismatch means the id was reused with a different payload.
	Mismatch
)

func payloadOf(req contract.ActionRequest) []byte {
	// encoding/json sorts map keys, so equal requests encode identically.
	b, _ := json.Marshal(struct {
		Protocol int            `json:"p"`
		Epoch    int64          `json:"e"`
		Target   string         `json:"t"`
		Action   string         `json:"a"`
		Args     map[string]any `json:"g"`
	}{req.Protocol, req.ContextEpoch, req.Target, req.Action, req.Args})
	return b
}

// Check classifies req for sender. For Replay the stored result is returned.
func (d *Dedup) Check(sender string, req contract.ActionRequest) (Verdict, contract.ActionResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.sessions[sender]
	if s == nil {
		return Fresh, contract.ActionResult{}
	}
	d.pruneLocked(s)
	e, ok := s.entries[req.RequestID]
	if !ok {
		return Fresh, contract.ActionResult{}
	}
	if !bytes.Equal(e.payload, payloadOf(req)) {
		return Mismatch, contract.ActionResult{}
	}
	return Replay, e.result
}

// Remember stores the (latest) result for req. Calling it again for the same
// id updates the stored result, so a replay after an asynchronous action
// completes returns the terminal outcome.
func (d *Dedup) Remember(sender string, req contract.ActionRequest, res contract.ActionResult) {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.sessions[sender]
	if s == nil {
		s = &dedupSession{entries: map[string]dedupEntry{}}
		d.sessions[sender] = s
	}
	d.pruneLocked(s)
	if e, ok := s.entries[req.RequestID]; ok {
		e.result = res
		s.entries[req.RequestID] = e
		return
	}
	s.entries[req.RequestID] = dedupEntry{payload: payloadOf(req), result: res, at: d.clock.Now()}
	s.order = append(s.order, req.RequestID)
	for len(s.order) > DedupSize {
		delete(s.entries, s.order[0])
		s.order = s.order[1:]
	}
}

// Forget drops everything remembered for sender (revocation, logout).
func (d *Dedup) Forget(sender string) {
	d.mu.Lock()
	delete(d.sessions, sender)
	d.mu.Unlock()
}

func (d *Dedup) pruneLocked(s *dedupSession) {
	now := d.clock.Now()
	for len(s.order) > 0 {
		e := s.entries[s.order[0]]
		if now.Sub(e.at) < DedupWindow {
			return
		}
		delete(s.entries, s.order[0])
		s.order = s.order[1:]
	}
}
