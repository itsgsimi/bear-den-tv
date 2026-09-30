// Tests for CheckAlive (dial.go), the watchdog's liveness check behind
// `bear-den-tv doctor --ping`, and the quiet hello it sends (server.go).

package shellipc

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"bear-den-tv/internal/contract"
)

// gateHandler builds its snapshot only while the gate is open: a
// coordinator stuck holding its lock.
type gateHandler struct {
	mu   sync.Mutex
	gate chan struct{}
}

func (h *gateHandler) InitialState(*Client) contract.State {
	h.mu.Lock()
	g := h.gate
	h.mu.Unlock()
	if g != nil {
		<-g
	}
	return contract.State{}
}
func (*gateHandler) Connected(*Client)           {}
func (*gateHandler) Disconnected(*Client, error) {}
func (*gateHandler) Receive(*Client, Message)    {}

func TestCheckAliveAnswersAndLogsQuietly(t *testing.T) {
	var logs bytes.Buffer
	var logMu sync.Mutex
	log := slog.New(slog.NewTextHandler(writerFunc(func(p []byte) (int, error) {
		logMu.Lock()
		defer logMu.Unlock()
		return logs.Write(p)
	}), &slog.HandlerOptions{Level: slog.LevelInfo}))
	srv, err := Listen(Options{SocketPath: filepath.Join(t.TempDir(), "s.sock"), Handler: &gateHandler{}, Logger: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := CheckAlive(ctx, srv.Path(), "test"); err != nil {
		t.Fatalf("a responsive coordinator: %v", err)
	}
	// An ordinary cli client is logged at info; the ping's quiet one is not.
	c, err := Dial(ctx, srv.Path(), ClientCLI, "test")
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		logMu.Lock()
		out := logs.String()
		logMu.Unlock()
		if strings.Count(out, "client disconnected") >= 1 {
			if n := strings.Count(out, "client connected"); n != 1 {
				t.Fatalf("want only the ordinary cli client's connect at info, got %d:\n%s", n, out)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no disconnect logged:\n%s", out)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// procState is the one-letter state in /proc/<pid>/stat ("T": stopped).
func procState(pid int) string {
	raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return ""
	}
	s := string(raw)
	return s[strings.LastIndex(s, ")")+2:][:1]
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

// A coordinator stuck under its lock accepts the connection but never
// builds the snapshot: CheckAlive fails at its deadline.
func TestCheckAliveFailsWhenTheCoordinatorIsStuck(t *testing.T) {
	h := &gateHandler{gate: make(chan struct{})}
	srv, err := Listen(Options{SocketPath: filepath.Join(t.TempDir(), "s.sock"), Handler: h})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { close(h.gate); srv.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := CheckAlive(ctx, srv.Path(), "test"); err == nil {
		t.Fatal("a stuck coordinator answered")
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %s, want about the 300ms deadline", d)
	}
}

func TestCheckAliveFailsWithoutACoordinator(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := CheckAlive(ctx, filepath.Join(t.TempDir(), "none.sock"), "test")
	if err == nil || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("want the missing socket named, got %v", err)
	}
}

const aliveHelperEnv = "BDTV_TEST_ALIVE_HELPER_SOCKET"

// TestAliveHelper is not a test: run by TestCheckAliveFailsWhileStopped, it
// serves a socket until killed.
func TestAliveHelper(t *testing.T) {
	sock := os.Getenv(aliveHelperEnv)
	if sock == "" {
		return // only meaningful as the helper process
	}
	if _, err := Listen(Options{SocketPath: sock, Handler: &gateHandler{}}); err != nil {
		fmt.Println("listen:", err)
		os.Exit(1)
	}
	fmt.Println("ready")
	select {}
}

// The incident's case: a coordinator process in state T (SIGSTOP). Its
// socket still accepts connections (the kernel's backlog), yet nothing
// answers; after SIGCONT it answers again.
func TestCheckAliveFailsWhileStopped(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "s.sock")
	cmd := exec.Command(os.Args[0], "-test.run=^TestAliveHelper$")
	cmd.Env = append(os.Environ(), aliveHelperEnv+"="+sock)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGCONT)
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	buf := make([]byte, 64)
	if n, _ := out.Read(buf); !strings.Contains(string(buf[:n]), "ready") {
		t.Fatalf("helper: %q", buf[:n])
	}
	check := func(d time.Duration) error {
		ctx, cancel := context.WithTimeout(context.Background(), d)
		defer cancel()
		return CheckAlive(ctx, sock, "test")
	}
	if err := check(5 * time.Second); err != nil {
		t.Fatalf("running helper: %v", err)
	}
	if err := cmd.Process.Signal(syscall.SIGSTOP); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(5 * time.Second); procState(cmd.Process.Pid) != "T"; time.Sleep(5 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("helper not stopped (state %q)", procState(cmd.Process.Pid))
		}
	}
	if err := check(500 * time.Millisecond); err == nil {
		t.Fatal("a stopped coordinator answered")
	}
	if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
		t.Fatal(err)
	}
	if err := check(5 * time.Second); err != nil {
		t.Fatalf("after SIGCONT: %v", err)
	}
}
