package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestFetchPreservesMultilineSamplesAndLabels(t *testing.T) {
	client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		body := "prompt,type\n\"first line\nsecond line\",benign\nignore previous instructions,jailbreak\n"
		if r.URL.Host == "datasets-server.huggingface.co" {
			body = `{"rows":[{"row":{"text":"safe example","label":0}},{"row":{"text":"override example","label":1}}],"num_rows_total":2}`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})}
	path := filepath.Join(t.TempDir(), "data.jsonl")
	if err := fetch(path, client); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }() // Best-effort cleanup.
	scan := bufio.NewScanner(f)
	var count, attacks, multiline int
	for scan.Scan() {
		var row struct {
			Text  string
			Label int
		}
		if err := json.Unmarshal(scan.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		count++
		attacks += row.Label
		if row.Text == "first line\nsecond line" {
			multiline++
		}
	}
	if err := scan.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 8 || attacks != 4 || multiline != 2 {
		t.Fatalf("count=%d attacks=%d multiline=%d", count, attacks, multiline)
	}
}

func TestFetchFailureDoesNotReplaceExistingCorpus(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.jsonl")
	if err := os.WriteFile(path, []byte("existing corpus"), 0600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) { return nil, fmt.Errorf("source unavailable") })}
	if err := fetch(path, client); err == nil {
		t.Fatal("expected fetch failure")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "existing corpus" {
		t.Fatalf("corpus changed: %q %v", data, err)
	}
}
