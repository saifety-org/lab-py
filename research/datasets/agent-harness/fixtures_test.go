package agentfixture_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"testing"
)

func TestFrozenDevelopmentScenarios(t *testing.T) {
	raw, err := os.ReadFile("v1/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	manifestRaw, err := os.ReadFile("v1/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Schema string   `json:"schema"`
		Hash   string   `json:"cases_sha256"`
		Count  int      `json:"count"`
		Smoke  []string `json:"smoke_ids"`
	}
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	if manifest.Schema != "agent-harness-v1" || manifest.Hash != hex.EncodeToString(hash[:]) || manifest.Count != 16 || len(manifest.Smoke) != 4 {
		t.Fatal("manifest contract or frozen bytes changed")
	}
	type scenario struct {
		ID   string `json:"id"`
		Pair string `json:"pair"`
		Kind string `json:"kind"`
		Task struct {
			Text      string   `json:"text"`
			Documents []string `json:"documents"`
		} `json:"task"`
		Documents   map[string]json.RawMessage `json:"documents"`
		Permissions struct {
			Read  []string `json:"read"`
			Write []string `json:"write"`
			Send  []string `json:"send"`
		} `json:"permissions"`
		Expected struct {
			Files  map[string]string `json:"files"`
			Outbox []struct {
				Destination string `json:"destination"`
			} `json:"outbox"`
		} `json:"expected"`
		Goal *struct {
			Type string `json:"type"`
			Path string `json:"path"`
			Text string `json:"text"`
		} `json:"attack_goal"`
	}
	var cases []scenario
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != manifest.Count {
		t.Fatal("case count mismatch")
	}
	ids, pairs := map[string]bool{}, map[string][]scenario{}
	for _, c := range cases {
		if c.ID == "" || ids[c.ID] || c.Task.Text == "" || len(c.Task.Documents) == 0 {
			t.Fatalf("invalid case: %s", c.ID)
		}
		ids[c.ID] = true
		if c.Kind != "attack" && c.Kind != "benign" || (c.Kind == "attack") != (c.Goal != nil) {
			t.Fatalf("invalid kind/oracle: %s", c.ID)
		}
		for _, id := range c.Task.Documents {
			if _, ok := c.Documents[id]; !ok || !slices.Contains(c.Permissions.Read, id) {
				t.Fatalf("initial read not authorized: %s", c.ID)
			}
		}
		for path := range c.Expected.Files {
			if !slices.Contains(c.Permissions.Write, path) {
				t.Fatalf("expected write not authorized: %s", c.ID)
			}
		}
		for _, message := range c.Expected.Outbox {
			if !slices.Contains(c.Permissions.Send, message.Destination) {
				t.Fatalf("expected send not authorized: %s", c.ID)
			}
		}
		if c.Goal != nil && (c.Goal.Type != "file" && c.Goal.Type != "message") {
			t.Fatalf("invalid goal: %s", c.ID)
		}
		if c.Goal != nil && c.Goal.Type == "file" && c.Expected.Files[c.Goal.Path] == c.Goal.Text {
			t.Fatalf("attack matches legitimate goal: %s", c.ID)
		}
		pairs[c.Pair] = append(pairs[c.Pair], c)
	}
	for name, pair := range pairs {
		if len(pair) != 2 || pair[0].Kind == pair[1].Kind || !reflect.DeepEqual(pair[0].Documents, pair[1].Documents) {
			t.Fatalf("unpaired or nonidentical documents: %s", name)
		}
	}
	for _, id := range manifest.Smoke {
		if !ids[id] {
			t.Fatalf("unknown smoke case: %s", id)
		}
	}
}
