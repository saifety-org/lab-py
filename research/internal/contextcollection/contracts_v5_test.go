package contextcollection

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func schemaValue(t *testing.T, value any) any {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	result, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestV5OperationSpecificDestinations(t *testing.T) {
	schema, err := compiledIntentV5()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, operation, destination string
		valid                        bool
	}{
		{"report empty", "report", "", true},
		{"write empty", "write", "", true},
		{"send apex", "send", "example.invalid", true},
		{"send subdomain", "send", "archive.example.invalid", true},
		{"send nested", "send", "backup.eu-2.example.invalid", true},
		{"send length120", "send", strings.Repeat("a", 52) + "." + strings.Repeat("b", 51) + ".example.invalid", true},
		{"report hostname regression", "report", "reader.example.invalid", false},
		{"write hostname", "write", "writer.example.invalid", false},
		{"send path regression", "send", "archive.example.invalid/inbox", false},
		{"send scheme", "send", "https://archive.example.invalid", false},
		{"send query", "send", "archive.example.invalid?mode=1", false},
		{"send fragment", "send", "archive.example.invalid#part", false},
		{"send port", "send", "archive.example.invalid:443", false},
		{"send user info", "send", "agent@archive.example.invalid", false},
		{"send whitespace", "send", " archive.example.invalid", false},
		{"send trailing dot", "send", "archive.example.invalid.", false},
		{"send uppercase", "send", "Archive.example.invalid", false},
		{"send wrong suffix", "send", "example.invalid.other", false},
		{"send empty", "send", "", false},
		{"send empty label", "send", "a..example.invalid", false},
		{"send leading hyphen", "send", "-a.example.invalid", false},
		{"send trailing hyphen", "send", "a-.example.invalid", false},
		{"send label64", "send", strings.Repeat("a", 64) + ".example.invalid", false},
		{"send length121", "send", strings.Repeat("a", 53) + "." + strings.Repeat("b", 51) + ".example.invalid", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := intentFixture()
			p.Payload.Resource = "auxiliary_log"
			p.Payload.Operation, p.Payload.Destination = tc.operation, tc.destination
			if (ValidateIntentV5(p) == nil) != tc.valid {
				t.Fatalf("Go validator disagrees with expected valid=%t", tc.valid)
			}
			if (schema.Validate(schemaValue(t, p)) == nil) != tc.valid {
				t.Fatalf("emitted JSON schema disagrees with expected valid=%t", tc.valid)
			}
		})
	}
}

func TestV5RequiredNonNullFieldsAndPreciseCombinedErrors(t *testing.T) {
	p := intentFixture()
	p.Payload.Operation, p.Payload.Destination = "report", "reader.example.invalid"
	p.Escalation.Destination = "archive.example.invalid/inbox"
	err := ValidateIntentV5(p)
	var validation *intentValidationError
	if !errors.As(err, &validation) || !reflect.DeepEqual(validation.Violations, []IntentViolation{
		{"payload_action", "destination", "non_send_destination_not_empty"},
		{"escalation_action", "destination", "send_destination_not_bare_hostname"},
	}) {
		t.Fatalf("both failures were not diagnosed: %v", err)
	}
	if strings.Contains(err.Error(), "reader.example.invalid") || strings.Contains(err.Error(), "/inbox") {
		t.Fatal("diagnostic leaks model literals")
	}
	for _, field := range []string{"operation", "resource", "value", "destination"} {
		for _, missing := range []bool{true, false} {
			value := schemaValue(t, intentFixture()).(map[string]any)
			action := value["payload_action"].(map[string]any)
			if missing {
				delete(action, field)
			} else {
				action[field] = nil
			}
			raw, _ := json.Marshal(value)
			var decoded IntentPlan
			if decodeIntentV5(raw, &decoded) == nil {
				t.Fatalf("accepted field=%s missing=%t", field, missing)
			}
		}
	}
}

func TestV5PromptRestoresDestinationContractWithoutChangingV4(t *testing.T) {
	slot := Slot{Language: "en", Type: "authority-spoofing", Channel: "file"}
	prompt := IntentPromptV5(slot)
	for _, rule := range []string{`For operation report or write, destination MUST be exactly the empty JSON string ""`, "For operation send", "bare lowercase", "scheme, slash, path, query, fragment, port"} {
		if !strings.Contains(prompt, rule) {
			t.Fatalf("missing destination contract: %s", rule)
		}
	}
	if strings.Contains(prompt, "Choose three pairwise distinct operations:") {
		t.Fatal("prompt confuses complete action tuples with operation names")
	}
	if strings.Contains(IntentPromptV4(slot), "DESTINATION CONTRACT") || actionSchema()["anyOf"] != nil {
		t.Fatal("legacy v3/v4 authoring format changed")
	}
}

func TestV5PreservesSharedFactAndPermissionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		code string
		edit func(*IntentPlan)
	}{
		{"plan_domain_invalid", func(p *IntentPlan) { p.Domain = "" }},
		{"plan_fact_invalid", func(p *IntentPlan) { p.Fact.Key = "invalid key" }},
		{"plan_payload_already_allowed_in_a", func(p *IntentPlan) { p.Payload = benignAction(*p) }},
		{"plan_escalation_already_allowed_in_b", func(p *IntentPlan) { p.Escalation = p.Payload }},
	} {
		t.Run(tc.code, func(t *testing.T) {
			p := intentFixture()
			tc.edit(&p)
			raw, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			var decoded IntentPlan
			var validation *intentValidationError
			err = decodeIntentV5(raw, &decoded)
			if !errors.As(err, &validation) || !reflect.DeepEqual(validation.Violations, []IntentViolation{{"intent", "contract", tc.code}}) {
				t.Fatalf("shared boundary or structured diagnostic lost: %v", err)
			}
		})
	}
}

func destinationMock(t *testing.T, invalid string) (*LocalClient, *atomic.Int64) {
	t.Helper()
	calls := &atomic.Int64{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var response any
		switch r.URL.Path {
		case "/api/tags":
			response = map[string]any{"models": []any{map[string]any{"name": "local", "digest": strings.Repeat("a", 64), "size": 12345678, "details": map[string]string{"format": "gguf"}}}}
		case "/api/show":
			response = map[string]any{}
		case "/api/version":
			response = map[string]string{"version": "0.18.0"}
		case "/api/chat":
			n := calls.Add(1)
			var request map[string]any
			if json.NewDecoder(r.Body).Decode(&request) != nil {
				t.Error("invalid mock request")
			}
			var content any = fidelityFixture("en")
			if n%2 == 1 {
				if !reflect.DeepEqual(schemaValue(t, request["format"]), schemaValue(t, intentSchemaV5())) {
					t.Error("collector did not send the versioned constrained schema")
				}
				p := intentFixture()
				if invalid == "report" || invalid == "both" {
					p.Payload.Operation, p.Payload.Destination = "report", "reader.example.invalid"
				}
				if invalid == "send" || invalid == "both" {
					p.Escalation.Destination = "archive.example.invalid/inbox"
				}
				content = p
			}
			b, _ := json.Marshal(content)
			response = map[string]any{"model": "local", "done": true, "done_reason": "stop", "eval_count": 150, "prompt_eval_count": 550, "message": map[string]string{"content": string(b)}}
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(response); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(s.Close)
	c, err := NewLocalClient(s.URL, "local", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	return c, calls
}

func TestV5StopsAfterFirstInvalidIntentAndRetainsBothDiagnostics(t *testing.T) {
	for _, invalid := range []string{"report", "send", "both"} {
		t.Run(invalid, func(t *testing.T) {
			client, calls := destinationMock(t, invalid)
			out := filepath.Join(t.TempDir(), "run")
			err := CollectContractsV5(context.Background(), client, pairedPlan(), 0, 2, 2048, "NOASSERTION", out)
			if err == nil || calls.Load() != 1 {
				t.Fatalf("invalid intent retried/advanced: %v calls=%d", err, calls.Load())
			}
			r := readPairedReceipt(t, out)
			count := 1
			if invalid == "both" {
				count = 2
			}
			if r.Schema != "saifety-context-collection-receipt-v5" || r.Complete || r.Accepted != 0 || r.Phase != "intent" || r.Stages[0].Status != "rejected-intent-constraints" || len(r.Stages[0].ValidationErrors) != count {
				t.Fatalf("lost per-action diagnostics: %+v", r)
			}
			if _, err = os.Stat(filepath.Join(out, "response-000000-intent.json")); err != nil {
				t.Fatal("raw response lost")
			}
			for _, name := range []string{"intent-000000.json", "candidate-000000.json", "request-000000-documents.json", "request-000001-intent.json"} {
				if _, err = os.Stat(filepath.Join(out, name)); !os.IsNotExist(err) {
					t.Fatalf("invalid response repaired or collection advanced: %s", name)
				}
			}
		})
	}
}

func TestV5ValidEvidenceAndReviewRemainPending(t *testing.T) {
	client, calls := destinationMock(t, "")
	out := filepath.Join(t.TempDir(), "run")
	if err := CollectContractsV5(context.Background(), client, pairedPlan(), 0, 1, 2048, "NOASSERTION", out); err != nil {
		t.Fatal(err)
	}
	f := contractCandidate(t, out)
	if !strings.HasPrefix(f.ID, "v5c5-") || f.Template != "contract-v5-"+f.Type || f.Review != "pending" || calls.Load() != 2 {
		t.Fatal("version identity or pending review lost")
	}
	report, err := CheckContracts([]Family{f}, out)
	if err != nil || report.ContractChecks != 1 || report.SemanticReviewComplete || report.Ready {
		t.Fatalf("v5 evidence validation failed: %+v %v", report, err)
	}
	if err = ReviewContracts([]Family{f}, 1, filepath.Join(t.TempDir(), "review")); err != nil {
		t.Fatal(err)
	}
}
