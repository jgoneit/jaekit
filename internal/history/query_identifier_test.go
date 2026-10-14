package history

import (
	"strings"
	"testing"
)

func TestNativeShowRejectsMalformedIdentifiers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query Query
	}{
		{"zero_event", Query{ID: "D0"}},
		{"leading_zero_event", Query{ID: "D01"}},
		{"event_path", Query{ID: "../escape"}},
		{"event_newline", Query{ID: "D1\nforged diagnostic"}},
		{"event_control", Query{ID: "D1\r\x1b[31m"}},
		{"request_path", Query{RequestID: "../escape"}},
		{"request_newline", Query{RequestID: "stored-1\nforged diagnostic"}},
		{"request_control", Query{RequestID: "stored-1\t"}},
		{"request_too_long", Query{RequestID: strings.Repeat("a", 129)}},
		{"missing_selector", Query{}},
		{"both_selectors", Query{ID: "D1", RequestID: "stored-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.query.Mode = "show"
			for _, in := range []Input{{State: "absent"}, identifierQueryInput()} {
				view, err := Run(in, tc.query)
				if Code(err) != "invalid_input" || view != nil {
					t.Fatalf("history %s: malformed selector returned view=%+v, error=%v", in.State, view, err)
				}
				if strings.ContainsAny(err.Error(), "\r\n\t\x1b") {
					t.Fatalf("diagnostic contains raw control characters: %q", err.Error())
				}
			}
		})
	}
}

func TestNativeShowPreservesValidIdentifierLookup(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query Query
		found bool
	}{
		{"stored_event", Query{ID: "D1"}, true},
		{"stored_request", Query{RequestID: "stored-1"}, true},
		{"unknown_event", Query{ID: "D99"}, false},
		{"unknown_request", Query{RequestID: "valid/request:2._-"}, false},
		{"maximum_request", Query{RequestID: strings.Repeat("a", 128)}, false},
		{"request_is_not_an_event_id", Query{RequestID: "D0"}, false},
		{"request_is_not_a_file_path", Query{RequestID: "a/../escape"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.query.Mode = "show"
			view, err := Run(identifierQueryInput(), tc.query)
			if !tc.found {
				if Code(err) != "unknown_event" || view != nil {
					t.Fatalf("valid absent selector returned view=%+v, error=%v", view, err)
				}
				return
			}
			if err != nil || view == nil || view.Event == nil || view.Event.ID != "D1" || view.Event.RequestID != "stored-1" {
				t.Fatalf("stored selector returned view=%+v, error=%v", view, err)
			}
		})
	}
}

func identifierQueryInput() Input {
	return Input{
		State: "present",
		Start: Ref{Seq: 1, SHA256: startHash},
		Chain: Chain{Hashes: []string{startHash}, Valid: true},
		Events: []Event{{
			ID: "D1", Seq: 1, RequestID: "stored-1",
			Decision: Decision{Kind: "관측", Summary: "stored event"},
			Context:  Context{RunHead: Ref{Seq: 1, SHA256: startHash}},
		}},
	}
}
