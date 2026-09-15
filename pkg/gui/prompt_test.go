package gui

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/engineeringjack/LunarisInjector/pkg/modtracker"
)

func TestShowChangePromptEmpty(t *testing.T) {
	decisions, cancelled, err := ShowChangePrompt(PromptOptions{
		Changes: []modtracker.ModChange{},
	})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if cancelled {
		t.Errorf("Expected cancelled to be false")
	}
	if len(decisions) != 0 {
		t.Errorf("Expected 0 decisions for empty changes")
	}
}

func TestPromptAPIEndpoints(t *testing.T) {
	changes := []modtracker.ModChange{
		{
			Path:     "mods/custom.jar",
			Type:     modtracker.ChangeAdded,
			Size:     1024,
			Category: "mod",
		},
		{
			Path:     "mods/removed.jar",
			Type:     modtracker.ChangeRemoved,
			Size:     2048,
			Category: "mod",
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/changes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"instance_name": "TestInstance",
			"changes":       changes,
		})
	})

	resultChan := make(chan promptResponse, 1)
	mux.HandleFunc("/api/decisions", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Decisions map[string]modtracker.UserDecision `json:"decisions"`
			Cancel    bool                               `json:"cancel"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		resultChan <- promptResponse{
			Decisions: payload.Decisions,
			Cancelled: payload.Cancel,
		}
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// 1. Test GET /api/changes
	resp, err := http.Get(srv.URL + "/api/changes")
	if err != nil {
		t.Fatalf("GET /api/changes failed: %v", err)
	}
	defer resp.Body.Close()

	var getResult struct {
		InstanceName string                 `json:"instance_name"`
		Changes      []modtracker.ModChange `json:"changes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getResult); err != nil {
		t.Fatalf("Decode changes failed: %v", err)
	}
	if getResult.InstanceName != "TestInstance" || len(getResult.Changes) != 2 {
		t.Errorf("Unexpected changes data: %+v", getResult)
	}

	// 2. Test POST /api/decisions
	payload := map[string]interface{}{
		"decisions": map[string]string{
			"mods/custom.jar":  "keep",
			"mods/removed.jar": "revert",
		},
		"cancel": false,
	}
	data, _ := json.Marshal(payload)
	postResp, err := http.Post(srv.URL+"/api/decisions", "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("POST /api/decisions failed: %v", err)
	}
	postResp.Body.Close()

	received := <-resultChan
	if received.Cancelled {
		t.Errorf("Expected cancelled false")
	}
	if received.Decisions["mods/custom.jar"] != modtracker.DecisionKeep {
		t.Errorf("Expected custom.jar decision keep, got %s", received.Decisions["mods/custom.jar"])
	}
	if received.Decisions["mods/removed.jar"] != modtracker.DecisionRevert {
		t.Errorf("Expected removed.jar decision revert, got %s", received.Decisions["mods/removed.jar"])
	}
}
