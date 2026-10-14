package record

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativeRecordHelper(t *testing.T) {
	if os.Getenv("HA_RECORD_HELPER") != "1" {
		return
	}
	dir := os.Getenv("HA_RECORD_DIR")
	switch os.Getenv("HA_RECORD_MODE") {
	case "hold":
		unlock, err := lock(filepath.Join(dir, "lock"))
		if err != nil {
			panic(err)
		}
		defer unlock()
		if err := os.WriteFile(filepath.Join(dir, "held"), []byte("locked"), 0o600); err != nil {
			panic(err)
		}
		for {
			time.Sleep(time.Second)
		}
	case "append":
		for i := 0; i < 5; i++ {
			e := &Note{Header: Header{Kind: "note", HaVersion: "synthetic"}, Note: "input", Quote: fmt.Sprintf("%d/%d", os.Getpid(), i)}
			if _, err := Append(filepath.Join(dir, "runs.jsonl"), filepath.Join(dir, "lock"), e, time.Now(), nil); err != nil {
				panic(err)
			}
		}
	default:
		panic("unknown record helper mode")
	}
	os.Exit(0)
}

func recordHelper(t *testing.T, dir, mode string) *exec.Cmd {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestNativeRecordHelper$")
	cmd.Env = append(os.Environ(), "HA_RECORD_HELPER=1", "HA_RECORD_DIR="+dir, "HA_RECORD_MODE="+mode)
	return cmd
}

func TestNativeConcurrentProcessesPreserveRecords(t *testing.T) {
	dir := t.TempDir()
	var commands []*exec.Cmd
	for i := 0; i < 4; i++ {
		cmd := recordHelper(t, dir, "append")
		cmd.Stdout, cmd.Stderr = &bytes.Buffer{}, &bytes.Buffer{}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("record helper: %v %s", err, cmd.Stderr)
		}
	}
	lines, bad, err := Read(filepath.Join(dir, "runs.jsonl"))
	if err != nil || bad != nil || len(lines) != 20 {
		t.Fatalf("native chain: %d %v %v", len(lines), bad, err)
	}
	quotes := map[string]bool{}
	for _, line := range lines {
		quotes[line.Quote] = true
	}
	if len(quotes) != 20 {
		t.Fatalf("lost or duplicated caller results: %d", len(quotes))
	}
}

func TestNativeLockIsReleasedWhenOwnerExits(t *testing.T) {
	dir := t.TempDir()
	owner := recordHelper(t, dir, "hold")
	var ownerOutput bytes.Buffer
	owner.Stdout, owner.Stderr = &ownerOutput, &ownerOutput
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = owner.Process.Kill() })
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "held")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("lock owner never acquired the lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	follower := recordHelper(t, dir, "append")
	finished := make(chan error, 1)
	go func() { finished <- follower.Run() }()
	select {
	case err := <-finished:
		t.Fatalf("append bypassed held lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = owner.Wait()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		_ = follower.Process.Kill()
		t.Fatal("OS lock remained held after owner exit")
	}
	lines, bad, err := Read(filepath.Join(dir, "runs.jsonl"))
	if err != nil || bad != nil || len(lines) != 5 {
		t.Fatalf("post-exit records: %d %v %v", len(lines), bad, err)
	}
	// A damaged tail is still refused; lock release is not permission to repair.
	f, err := os.OpenFile(filepath.Join(dir, "runs.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("{partial")
	f.Close()
	_, err = Append(filepath.Join(dir, "runs.jsonl"), filepath.Join(dir, "lock"), note("after damage"), time.Now(), nil)
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("damaged history accepted: %v", err)
	}
}
