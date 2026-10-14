package run

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type failedTermination struct{ actual processController }

func TestNativeAdmissionFailureHasNoOutputDigest(t *testing.T) {
	starter := func(*exec.Cmd) (processController, error) {
		return nil, errors.Join(errUnfinalizedProcess, errors.New("synthetic admission cleanup failure"))
	}
	res, err := execWithStarter(t.TempDir(), []string{"unused"}, time.Second, filepath.Join(t.TempDir(), "output.log"), nil, starter)
	if err == nil || !res.Attempted || res.Outcome != Error || res.Digest != "" || res.Exit != nil {
		t.Fatalf("admission error produced final evidence: %+v %v", res, err)
	}
}

func (f failedTermination) kill() error   { return errors.New("synthetic termination failure") }
func (f failedTermination) finish() error { return errors.New("synthetic shutdown unconfirmed") }

func TestNativeUnconfirmedShutdownHasNoOutputDigest(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "heartbeat")
	var actual processController
	starter := func(cmd *exec.Cmd) (processController, error) {
		group, err := startProcess(cmd)
		if err != nil {
			return nil, err
		}
		actual = group
		return failedTermination{group}, nil
	}
	defer func() {
		if actual != nil {
			actual.kill()
			actual.finish()
		}
	}()
	res, err := execWithStarter(dir, helperCommand(t), time.Second, filepath.Join(dir, "output.log"),
		[]string{"HA_NATIVE_HELPER=1", "HA_NATIVE_MODE=parent", "HA_NATIVE_MARKER=" + marker}, starter)
	if err == nil || !res.Attempted || res.Outcome != Error || res.Digest != "" || res.Exit != nil {
		t.Fatalf("unconfirmed process produced final output evidence: %+v %v", res, err)
	}
	if data, readErr := os.ReadFile(marker); readErr != nil || len(data) == 0 {
		t.Fatalf("fixture never spawned the child: %v", readErr)
	}
}

// A Go helper keeps these process/argv/timeout tests independent of any shell.
func TestNativeProcessHelper(t *testing.T) {
	if os.Getenv("HA_NATIVE_HELPER") != "1" {
		return
	}
	switch os.Getenv("HA_NATIVE_MODE") {
	case "arguments":
		fmt.Println(strings.Join(os.Args, "\n"))
		fmt.Println(os.Getenv("HA_NATIVE_VALUE"))
		code, _ := strconv.Atoi(os.Getenv("HA_NATIVE_EXIT"))
		os.Exit(code)
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestNativeProcessHelper$")
		child.Env = append(os.Environ(), "HA_NATIVE_MODE=heartbeat")
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			panic(err)
		}
		for {
			time.Sleep(time.Second)
		}
	case "heartbeat":
		for {
			f, err := os.OpenFile(os.Getenv("HA_NATIVE_MARKER"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
			if err != nil {
				panic(err)
			}
			fmt.Fprintln(f, "child")
			f.Close()
			time.Sleep(10 * time.Millisecond)
		}
	default:
		panic("unknown native test mode")
	}
}

func helperCommand(t *testing.T) []string {
	t.Helper()
	p, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return []string{p, "-test.run=^TestNativeProcessHelper$", "--", "space argument", "한글", `C:\path with spaces\file`}
}

func TestNativeArgvEnvironmentAndOutcomes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "space 한글")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, code := range []int{0, 3} {
		log := filepath.Join(dir, fmt.Sprintf("%d.log", code))
		res, err := ExecWithEnv(dir, helperCommand(t), 10*time.Second, log,
			[]string{"HA_NATIVE_HELPER=1", "HA_NATIVE_MODE=arguments", "HA_NATIVE_VALUE=한글 value", fmt.Sprintf("HA_NATIVE_EXIT=%d", code)})
		want := Pass
		if code != 0 {
			want = Fail
		}
		if err != nil || res.Outcome != want || res.Exit == nil || *res.Exit != code || res.Digest == "" {
			t.Fatalf("native outcome: %+v, %v", res, err)
		}
		data, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{"space argument", "한글", `C:\path with spaces\file`, "한글 value"} {
			if !strings.Contains(string(data), value) {
				t.Fatalf("argument/environment lost: %q", value)
			}
		}
	}
	res, err := Exec(dir, []string{"ha-nonexistent-native-checker"}, time.Second, filepath.Join(dir, "missing.log"))
	if err != nil || !res.Attempted || res.Outcome != Error || res.Reason != "process_start" || res.Exit != nil {
		t.Fatalf("missing process: %+v %v", res, err)
	}
}

func TestNativeTimeoutStopsDescendantWrites(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "heartbeat")
	res, err := ExecWithEnv(dir, helperCommand(t), time.Second, filepath.Join(dir, "timeout.log"),
		[]string{"HA_NATIVE_HELPER=1", "HA_NATIVE_MODE=parent", "HA_NATIVE_MARKER=" + marker})
	if err != nil || res.Outcome != Error || res.Reason != "process_timeout" || res.Exit != nil {
		t.Fatalf("timeout: %+v %v", res, err)
	}
	before, err := os.ReadFile(marker)
	if err != nil || len(before) == 0 {
		t.Fatalf("child did not run: %v", err)
	}
	time.Sleep(150 * time.Millisecond)
	after, err := os.ReadFile(marker)
	if err != nil || string(before) != string(after) {
		t.Fatalf("descendant still writes after timeout: %v", err)
	}
}
