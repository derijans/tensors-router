package kobold

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	fakeKoboldBehaviorEnv  = "KOBOLD_TEST_FAKE_BEHAVIOR"
	fakeKoboldLaunchLogEnv = "KOBOLD_TEST_FAKE_LAUNCH_LOG"

	fakeKoboldPredatesQueueLimit   = "predates-queuelimit"
	fakeKoboldRejectsOtherArgument = "rejects-other-argument"
)

func runFakeKobold(behavior string, args []string) int {
	recordFakeKoboldLaunch(args)
	switch {
	case behavior == fakeKoboldPredatesQueueLimit && slices.Contains(args, queueLimitFlag):
		fmt.Fprintln(os.Stderr, "koboldcpp: error: unrecognized arguments: --queuelimit 1")
		return 2
	case behavior == fakeKoboldRejectsOtherArgument:
		fmt.Fprintln(os.Stderr, "koboldcpp: error: unrecognized arguments: --bogus")
		return 2
	}
	portIndex := slices.Index(args, "--port")
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", args[portIndex+1]))
	if err != nil {
		return 1
	}
	_ = http.Serve(listener, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	return 0
}

func recordFakeKoboldLaunch(args []string) {
	launchLog, err := os.OpenFile(os.Getenv(fakeKoboldLaunchLogEnv), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer launchLog.Close()
	fmt.Fprintln(launchLog, strings.Join(args, " "))
}

func managerOnFakeKobold(t *testing.T, behavior string) (*Manager, string) {
	t.Helper()
	launchLogPath := filepath.Join(t.TempDir(), "launches.log")
	t.Setenv(fakeKoboldBehaviorEnv, behavior)
	t.Setenv(fakeKoboldLaunchLogEnv, launchLogPath)
	manager, err := NewManager(ProcessConfig{
		BackendURL: "http://" + freeLoopbackAddress(t),
		BinaryPath: os.Args[0],
		ConfigDir:  t.TempDir(),
		DataDir:    t.TempDir(),
		Multiuser:  1,
		NoModel:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.probeClient.Timeout = time.Second
	t.Cleanup(func() { _ = manager.Stop(context.Background()) })
	return manager, launchLogPath
}

func freeLoopbackAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return address
}

func recordedLaunches(t *testing.T, launchLogPath string) []string {
	t.Helper()
	content, err := os.ReadFile(launchLogPath)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

func TestStartRelaunchesWithMultiuserWhenKoboldPredatesQueueLimit(t *testing.T) {
	manager, launchLogPath := managerOnFakeKobold(t, fakeKoboldPredatesQueueLimit)

	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("a KoboldCpp without --queuelimit must start through the legacy flag: %v", err)
	}

	launches := recordedLaunches(t, launchLogPath)
	if len(launches) != 2 {
		t.Fatalf("expected the rejected launch and one legacy relaunch, got %q", launches)
	}
	if !strings.Contains(launches[0], "--queuelimit 1") {
		t.Fatalf("first launch must try --queuelimit: %q", launches[0])
	}
	if !strings.Contains(launches[1], "--multiuser 1") || strings.Contains(launches[1], queueLimitFlag) {
		t.Fatalf("relaunch must pass --multiuser instead of --queuelimit: %q", launches[1])
	}
	args := manager.LaunchArguments()
	expectSequence(t, args, "--multiuser", "1")
	expectAbsent(t, args, "--queuelimit")
}

func TestStartKeepsQueueLimitWhenKoboldRejectsAnotherArgument(t *testing.T) {
	manager, launchLogPath := managerOnFakeKobold(t, fakeKoboldRejectsOtherArgument)

	err := manager.Start(context.Background())
	if err == nil {
		t.Fatal("expected the startup failure to be returned")
	}
	var exitErr *backendExitedError
	if !strings.Contains(err.Error(), "exited during startup") || !errors.As(err, &exitErr) {
		t.Fatalf("expected the original startup exit, got %v", err)
	}
	if launches := recordedLaunches(t, launchLogPath); len(launches) != 1 {
		t.Fatalf("an unrelated argparse rejection must not trigger the legacy relaunch, got %q", launches)
	}
	expectSequence(t, manager.LaunchArguments(), "--queuelimit", "1")
}

func TestQueueLimitRejectionDetectorJoinsSplitWrites(t *testing.T) {
	detector := &queueLimitFlagRejectionDetector{}
	_, _ = detector.Write([]byte("Namespace(queuelimit=1)\nkoboldcpp: error: unrecognized argu"))
	if detector.Rejected() {
		t.Fatal("a partial rejection line must not count as a rejection")
	}
	_, _ = detector.Write([]byte("ments: --quiet --queuelimit 1\n"))
	if !detector.Rejected() {
		t.Fatal("a rejection line split across writes must be detected")
	}
}
