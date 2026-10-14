package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/history"
)

func TestNativeHistoryRequestIDPresence(t *testing.T) {
	decision := `"decision":{"kind":"관측","summary":"request identity"}`
	for _, tc := range []struct {
		name  string
		input string
		args  []string
	}{
		{"empty_flag_value", "", []string{"add", "--kind", "관측", "--summary", "request identity", "--request-id", ""}},
		{"empty_flag_equals", "", []string{"add", "--kind", "관측", "--summary", "request identity", "--request-id="}},
		{"empty_json", `{"request_id":"",` + decision + `}`, []string{"add", "--input", "request.json"}},
		{"null_json", `{"request_id":null,` + decision + `}`, []string{"add", "--input", "request.json"}},
		{"empty_json_with_flag", `{"request_id":"",` + decision + `}`, []string{"add", "--input", "request.json", "--request-id", "supplied"}},
		{"null_json_with_flag", `{"request_id":null,` + decision + `}`, []string{"add", "--input", "request.json", "--request-id", "supplied"}},
		{"valid_json_with_empty_flag", `{"request_id":"supplied",` + decision + `}`, []string{"add", "--input", "request.json", "--request-id="}},
		{"show_empty_flag", "", []string{"show", "--request-id="}},
		{"show_event_and_empty_flag", "", []string{"show", "D1", "--request-id="}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeHistoryFixture(t, false)
			runs := f.read(scenarioGoal + "/runs.jsonl")
			store := nativeHistoryStore(t, f)
			if tc.input != "" {
				f.put("request.json", tc.input)
			}
			args := append([]string{"history", scenarioGoal}, tc.args...)
			out := nativeHistoryCommand(t, f.dir, exitUsage, args...)
			if !strings.Contains(out, "invalid_input") {
				t.Fatalf("empty request id lost its public error: %s", out)
			}
			if _, err := os.Stat(store); !os.IsNotExist(err) {
				t.Fatalf("invalid request created history storage: %v", err)
			}
			if !bytes.Equal(runs, f.read(scenarioGoal+"/runs.jsonl")) {
				t.Fatal("invalid request changed execution records")
			}
		})
	}

	t.Run("omitted_and_valid_ids_preserve_replay", func(t *testing.T) {
		f := newNativeHistoryFixture(t, false)
		type receipt struct {
			ID        string `json:"id"`
			RequestID string `json:"request_id"`
			SHA256    string `json:"sha256"`
			Replayed  bool   `json:"replayed"`
		}
		add := func(args ...string) receipt {
			t.Helper()
			argv := append([]string{"history", scenarioGoal, "add"}, args...)
			out := nativeHistoryCommand(t, f.dir, exitOK, argv...)
			var result receipt
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatalf("decode receipt: %v: %s", err, out)
			}
			return result
		}
		checkReplay := func(first, replay receipt) {
			t.Helper()
			if first.Replayed || !replay.Replayed || first.ID != replay.ID || first.RequestID != replay.RequestID || first.SHA256 != replay.SHA256 {
				t.Fatalf("request identity did not replay: first=%+v replay=%+v", first, replay)
			}
		}
		flags := []string{"--kind", "관측", "--summary", "request identity"}
		auto := add(flags...)
		if !strings.HasPrefix(auto.RequestID, "auto-") || history.CheckRequestID(auto.RequestID) != nil {
			t.Fatalf("omitted flag did not generate a valid id: %+v", auto)
		}
		checkReplay(auto, add(append(flags, "--request-id", auto.RequestID)...))
		f.put("request.json", `{`+decision+`}`)
		jsonAuto := add("--input", "request.json")
		if !strings.HasPrefix(jsonAuto.RequestID, "auto-") || history.CheckRequestID(jsonAuto.RequestID) != nil || jsonAuto.RequestID == auto.RequestID {
			t.Fatalf("omitted JSON field did not generate a fresh valid id: %+v", jsonAuto)
		}
		f.put("request.json", `{"request_id":"`+jsonAuto.RequestID+`",`+decision+`}`)
		checkReplay(jsonAuto, add("--input", "request.json", "--request-id", jsonAuto.RequestID))
		supplied := add(append(flags, "--request-id=supplied")...)
		if supplied.RequestID != "supplied" {
			t.Fatalf("explicit request id changed: %+v", supplied)
		}
		checkReplay(supplied, add(append(flags, "--request-id", "supplied")...))
		if v := nativeHistoryView(t, f, "show", "--request-id", auto.RequestID); v.Event == nil || v.Event.ID != auto.ID {
			t.Fatalf("generated request id cannot be looked up: %+v", v)
		}
		if v := nativeHistoryView(t, f, "list"); v.Events != 3 {
			t.Fatalf("replay appended events: %+v", v)
		}
	})
}
