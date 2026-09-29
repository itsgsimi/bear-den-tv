// Tests for the DevTools control channel with a scripted Chromium on the
// other end of the pipe (browser.go, cdp.go): the coordinator keeps only its
// own two pipe ends (no other holder can speak on it), the page setup puts
// the report binding and the script in the bearden world only, a report
// that does not come from that world is ignored, and a page asking for
// input outside the closed set gets none.

package web

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"bear-den-tv/internal/applications/adapters"
	"bear-den-tv/internal/config"
)

type fakeProc struct{ done chan struct{} }

func (p fakeProc) PID() int              { return 0 }
func (p fakeProc) Done() <-chan struct{} { return p.done }

// fakeChromium answers every command with an empty result (or a scripted
// one) and lets the test send events.
type fakeChromium struct {
	t      *testing.T
	passed []*os.File // what Start was given
	in     *bufio.Reader
	out    *os.File
	mu     sync.Mutex
	calls  []map[string]any
	reply  func(method string, params map[string]any) any
	wmu    sync.Mutex
}

func (f *fakeChromium) Start(_ context.Context, _ adapters.BrowserInfo, args []string, extra []*os.File) (Process, error) {
	f.passed = extra
	// The child's own copies (what fork/exec would give Chromium).
	r, err := syscall.Dup(int(extra[0].Fd()))
	if err != nil {
		return nil, err
	}
	w, err := syscall.Dup(int(extra[1].Fd()))
	if err != nil {
		return nil, err
	}
	f.in = bufio.NewReader(os.NewFile(uintptr(r), "fd3"))
	f.out = os.NewFile(uintptr(w), "fd4")
	go f.serve()
	return fakeProc{done: make(chan struct{})}, nil
}

func (f *fakeChromium) send(v any) {
	raw, _ := json.Marshal(v)
	f.wmu.Lock()
	_, _ = f.out.Write(append(raw, 0))
	f.wmu.Unlock()
}

func (f *fakeChromium) serve() {
	for {
		raw, err := f.in.ReadBytes(0)
		if err != nil {
			return
		}
		var m map[string]any
		_ = json.Unmarshal(raw[:len(raw)-1], &m)
		f.mu.Lock()
		f.calls = append(f.calls, m)
		f.mu.Unlock()
		params, _ := m["params"].(map[string]any)
		var result any = map[string]any{}
		if f.reply != nil {
			if r := f.reply(m["method"].(string), params); r != nil {
				result = r
			}
		}
		f.send(map[string]any{"id": m["id"], "result": result})
	}
}

func (f *fakeChromium) methods() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		out = append(out, c["method"].(string))
	}
	return out
}

func (f *fakeChromium) call(method string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.calls {
		if c["method"] == method {
			return c
		}
	}
	return nil
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out: %s", what)
}

func TestPipeChannelHasOnlyTwoHoldersAndTheWorldIsIsolated(t *testing.T) {
	fc := &fakeChromium{t: t}
	var evalExpr string
	var mu sync.Mutex
	fc.reply = func(method string, p map[string]any) any {
		switch method {
		case "Runtime.evaluate":
			mu.Lock()
			evalExpr, _ = p["expression"].(string)
			mu.Unlock()
			// A page that asks for a key outside the closed set.
			return map[string]any{"result": map[string]any{"value": map[string]any{"ok": true, "outcome": "keys", "effect": map[string]any{"kind": "keys", "keys": []string{"Delete"}}}}}
		case "Page.getLayoutMetrics":
			return map[string]any{"cssLayoutViewport": map[string]any{"clientWidth": 1280, "clientHeight": 720}}
		}
		return nil
	}
	m := NewManager(Options{DataHome: t.TempDir(), Starter: fc})
	app := config.Application{ID: "hulu", Label: "Hulu", Adapter: "hulu", Launch: config.Launch{Kind: "flatpak", AppID: "org.chromium.Chromium"}, Web: &config.Web{URL: "https://www.hulu.com/"}}
	ad, _ := adapters.ForName("hulu")
	spec, _ := adapters.WebOf(ad)
	if _, err := m.Launch(context.Background(), app, spec); err != nil {
		t.Fatal(err)
	}
	// Chromium's ends were closed in the coordinator after the start.
	for i, f := range fc.passed {
		if f.Fd() != ^uintptr(0) {
			t.Errorf("the coordinator still holds the child's pipe end %d", i+3)
		}
	}
	if got := fc.call("Target.setAutoAttach"); got == nil || got["params"].(map[string]any)["flatten"] != true {
		t.Fatalf("auto-attach: %v", got)
	}
	// A page appears; its setup puts the binding and the script in the
	// bearden world only.
	fc.send(map[string]any{"method": "Target.attachedToTarget", "params": map[string]any{"sessionId": "S1", "targetInfo": map[string]any{"targetId": "F1", "type": "page"}, "waitingForDebugger": true}})
	fc.send(map[string]any{"method": "Runtime.executionContextCreated", "sessionId": "S1", "params": map[string]any{"context": map[string]any{"id": 7, "name": WorldName, "auxData": map[string]any{"frameId": "F1"}}}})
	waitUntil(t, "page setup", func() bool { return fc.call("Runtime.runIfWaitingForDebugger") != nil })
	bind := fc.call("Runtime.addBinding")["params"].(map[string]any)
	if bind["name"] != ReportBinding || bind["executionContextName"] != WorldName {
		t.Fatalf("binding %v", bind)
	}
	script := fc.call("Page.addScriptToEvaluateOnNewDocument")["params"].(map[string]any)
	if script["worldName"] != WorldName {
		t.Fatalf("script world %v", script["worldName"])
	}
	// A report from the page's main world (context 3) is ignored; one from
	// the bearden world (context 7) counts.
	report := func(ctx int, video string) {
		payload, _ := json.Marshal(Status{V: 1, Visible: true, Video: video, Viewport: Viewport{W: 1280, H: 720}})
		fc.send(map[string]any{"method": "Runtime.bindingCalled", "sessionId": "S1", "params": map[string]any{"name": ReportBinding, "payload": string(payload), "executionContextId": ctx}})
	}
	report(3, "playing")
	time.Sleep(50 * time.Millisecond)
	if st, _ := m.Status("hulu"); st.Video == "playing" {
		t.Fatal("a report from outside the bearden world was believed")
	}
	report(7, "paused")
	waitUntil(t, "the bearden report", func() bool { st, _ := m.Status("hulu"); return st.Video == "paused" })

	// The page asks for Delete: refused, and no key event is sent.
	if _, err := m.Apply(context.Background(), "hulu", "media.pause", nil); err == nil {
		t.Fatal("a key outside the closed set was accepted")
	}
	for _, method := range fc.methods() {
		if method == "Input.dispatchKeyEvent" {
			t.Fatal("a key reached the page")
		}
	}
	mu.Lock()
	expr := evalExpr
	mu.Unlock()
	if expr != `__bdtv.apply("media.pause")` {
		t.Fatalf("expression %q", expr)
	}
	// Phones' own words never become expressions: an unknown action is
	// refused before anything is evaluated.
	if _, err := m.Apply(context.Background(), "hulu", `nav.up");alert(1);("`, nil); err == nil {
		t.Fatal("an unknown action was applied")
	}
}
