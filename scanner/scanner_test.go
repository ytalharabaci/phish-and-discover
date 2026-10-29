package scanner

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ytalharabaci/phish-and-discover/models"
)

// ── ToolRegistry ──────────────────────────────────────────────────────────────

func TestToolRegistryHasShippedTools(t *testing.T) {
	expected := []string{
		"subfinder", "dnsx", "naabu", "httpx", "tlsx",
		"katana", "nuclei", "uncover", "cloudlist", "asnmap",
	}
	for _, name := range expected {
		if _, ok := DefaultRegistry.Get(name); !ok {
			t.Errorf("expected tool %q to be registered in DefaultRegistry", name)
		}
	}
}

func TestToolRegistryRegisterAndGet(t *testing.T) {
	r := &ToolRegistry{tools: make(map[string]Tool)}
	if _, ok := r.Get("nuclei"); ok {
		t.Fatalf("expected fresh registry to be empty")
	}
	r.Register(new(NucleiTool))
	tool, ok := r.Get("nuclei")
	if !ok {
		t.Fatalf("expected nuclei to be registered")
	}
	if tool.Name() != "nuclei" {
		t.Errorf("expected tool name 'nuclei', got %q", tool.Name())
	}
	names := r.Names()
	if len(names) != 1 || names[0] != "nuclei" {
		t.Errorf("expected Names() == [nuclei], got %v", names)
	}
}

// ── Tool.BuildArgs / ExtractTarget ───────────────────────────────────────────

func TestToolBuildArgsIncludesBinaryAndTarget(t *testing.T) {
	const target = "example.com"
	for _, name := range DefaultRegistry.Names() {
		tool, _ := DefaultRegistry.Get(name)
		args := tool.BuildArgs(target, "", models.ScanOptions{})
		if len(args) == 0 {
			t.Errorf("%s: BuildArgs returned no arguments", name)
			continue
		}
		if args[0] != tool.Name() {
			t.Errorf("%s: expected args[0] == %q, got %q", name, tool.Name(), args[0])
		}
	}
}

func TestNaabuBuildArgsAddsInterfaceWhenSupported(t *testing.T) {
	tool := &NaabuTool{}
	args := tool.BuildArgs("example.com", "tun0", models.ScanOptions{})
	found := false
	for i, a := range args {
		if a == "-interface" && i+1 < len(args) && args[i+1] == "tun0" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected naabu args to include '-interface tun0', got %v", args)
	}
}

func TestNucleiExtractTargetPrefersMatchedAt(t *testing.T) {
	tool := &NucleiTool{}
	got := tool.ExtractTarget(map[string]interface{}{
		"matched-at": "https://example.com/admin",
		"host":       "example.com",
	})
	if got != "https://example.com/admin" {
		t.Errorf("expected matched-at to take priority, got %q", got)
	}
}

// ── ScanState ─────────────────────────────────────────────────────────────────

func TestScanStateLockLifecycle(t *testing.T) {
	s := &ScanState{}
	if s.IsScanRunning() {
		t.Fatalf("fresh ScanState should not be running")
	}
	if err := s.AcquireLock("nuclei", "example.com"); err != nil {
		t.Fatalf("expected first AcquireLock to succeed, got %v", err)
	}
	if err := s.AcquireLock("httpx", "other.com"); err == nil {
		t.Fatalf("expected second AcquireLock to fail while a scan is running")
	}
	s.ReleaseLock()
	if s.IsScanRunning() {
		t.Fatalf("expected ScanState to be released")
	}
	if err := s.AcquireLock("httpx", "other.com"); err != nil {
		t.Fatalf("expected AcquireLock to succeed after release, got %v", err)
	}
}

// ── fakeExecutor: a minimal ToolExecutor for orchestration tests ────────────

type fakeExecutor struct {
	mu          sync.Mutex
	calls       []string
	panicOnTool string
	errOnTool   string
}

func (f *fakeExecutor) Execute(ctx context.Context, userID int64, toolName, target, ifaceName string, args []string) error {
	f.mu.Lock()
	f.calls = append(f.calls, toolName)
	f.mu.Unlock()
	if toolName == f.panicOnTool {
		panic("simulated tool failure: " + toolName)
	}
	if toolName == f.errOnTool {
		return context.DeadlineExceeded
	}
	return nil
}

func (f *fakeExecutor) Collect(ctx context.Context, userID int64, parseAs, target, ifaceName string, args []string) ([]string, error) {
	f.mu.Lock()
	f.calls = append(f.calls, parseAs)
	f.mu.Unlock()
	return nil, nil
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// waitUntilIdle polls until the scan lock is released or the timeout elapses.
func waitUntilIdle(t *testing.T, s *ScanState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !s.IsScanRunning() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("scan did not release its lock within %v", timeout)
}

// ── RunScannerTool: panic recovery ───────────────────────────────────────────

func TestRunScannerToolPanicDoesNotCrashProcess(t *testing.T) {
	state := &ScanState{}
	svc := &DiscoverScanService{
		Executor: &fakeExecutor{panicOnTool: "nuclei"},
		State:    state,
	}

	// scanID 0 skips the DB write in models.UpdateScanTaskProgress, so this
	// test has no database dependency.
	if err := svc.RunScannerTool(1, 0, "nuclei", "example.com", "", models.ScanOptions{}); err != nil {
		t.Fatalf("RunScannerTool returned unexpected error: %v", err)
	}

	// If the panic in the background goroutine were not recovered, it would
	// crash this whole test binary instead of failing gracefully here.
	waitUntilIdle(t, state, 2*time.Second)
}

// ── RunTask: per-goroutine panic recovery in the parallel branch ────────────

func TestRunTaskParallelPanicDoesNotBlockSiblings(t *testing.T) {
	state := &ScanState{}
	exec := &fakeExecutor{panicOnTool: "naabu"}
	svc := &DiscoverScanService{Executor: exec, State: state}

	tools := []string{"naabu", "httpx", "nuclei"}
	opts := models.ScanOptions{Parallel: true}

	if err := svc.RunTask(1, 0, "example.com", "", tools, opts); err != nil {
		t.Fatalf("RunTask returned unexpected error: %v", err)
	}

	waitUntilIdle(t, state, 2*time.Second)

	if got := exec.callCount(); got != len(tools) {
		t.Errorf("expected all %d tools to be invoked despite one panicking, got %d calls", len(tools), got)
	}
}

// ── RunTask: sequential mode stops on context cancellation, not on error ────

func TestRunTaskSequentialRunsAllTools(t *testing.T) {
	state := &ScanState{}
	exec := &fakeExecutor{errOnTool: "httpx"}
	svc := &DiscoverScanService{Executor: exec, State: state}

	tools := []string{"naabu", "httpx", "nuclei"}
	if err := svc.RunTask(1, 0, "example.com", "", tools, models.ScanOptions{}); err != nil {
		t.Fatalf("RunTask returned unexpected error: %v", err)
	}

	waitUntilIdle(t, state, 2*time.Second)

	if got := exec.callCount(); got != len(tools) {
		t.Errorf("expected a tool returning an error to not stop the sequence, got %d/%d calls", got, len(tools))
	}
}

// ── Lock contention surfaces synchronously to the caller ────────────────────

func TestRunScannerToolLockContention(t *testing.T) {
	state := &ScanState{}
	if err := state.AcquireLock("nuclei", "already-running.com"); err != nil {
		t.Fatalf("setup: %v", err)
	}
	defer state.ReleaseLock()

	svc := &DiscoverScanService{Executor: &fakeExecutor{}, State: state}
	err := svc.RunScannerTool(1, 0, "httpx", "example.com", "", models.ScanOptions{})
	if err == nil {
		t.Fatalf("expected RunScannerTool to fail synchronously while a scan is already running")
	}
}

// ── Sanity: registering a custom tool doesn't disturb existing entries ──────

func TestToolRegistryRegisterOverwritesByName(t *testing.T) {
	r := &ToolRegistry{tools: make(map[string]Tool)}
	var calls int32
	r.Register(countingTool{name: "probe", calls: &calls})
	r.Register(countingTool{name: "probe", calls: &calls})
	if len(r.Names()) != 1 {
		t.Fatalf("expected re-registering the same name to overwrite, got %v", r.Names())
	}
}

type countingTool struct {
	name  string
	calls *int32
}

func (c countingTool) Name() string { atomic.AddInt32(c.calls, 1); return c.name }
func (c countingTool) BuildArgs(target, ifaceName string, opts models.ScanOptions) []string {
	return []string{c.name, target}
}
func (c countingTool) ExtractTarget(obj map[string]interface{}) string { return "" }
func (c countingTool) SupportsInterface() bool                         { return false }
func (c countingTool) IsJSONLOutput() bool                             { return true }
