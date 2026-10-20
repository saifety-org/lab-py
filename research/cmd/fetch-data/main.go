// Command fetch-data normalizes public classifier datasets into JSONL.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func get(client *http.Client, address string) ([]byte, error) {
	resp, err := client.Get(address)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }() // Best-effort cleanup.
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", address, resp.StatusCode)
	}
	const limit = 32 << 20
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if len(b) > limit {
		return nil, fmt.Errorf("response exceeds %d bytes", limit)
	}
	return b, nil
}

func fetch(out string, client *http.Client) error {
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(out), ".external-*.jsonl")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }() // Best-effort cleanup.
	defer func() { _ = f.Close() }()           // Best-effort cleanup.
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	counts := [2]int{}
	write := func(text string, label int, source string) error {
		text = strings.TrimSpace(text)
		if label < 0 || label > 1 {
			return fmt.Errorf("invalid label %d", label)
		}
		if len(text) < 3 || len(text) > 6000 {
			return nil
		}
		if err := enc.Encode(struct {
			Text   string `json:"text"`
			Label  int    `json:"label"`
			Source string `json:"src"`
		}{text, label, source}); err != nil {
			return err
		}
		counts[label]++
		return nil
	}
	for _, split := range []string{"train", "test"} {
		address := "https://huggingface.co/datasets/jackhhao/jailbreak-classification/resolve/main/balanced/jailbreak_dataset_" + split + "_balanced.csv"
		b, err := get(client, address)
		if err != nil {
			return err
		}
		r := csv.NewReader(strings.NewReader(string(b)))
		headers, err := r.Read()
		if err != nil {
			return err
		}
		prompt, kind := -1, -1
		for i, name := range headers {
			if name == "prompt" {
				prompt = i
			}
			if name == "type" {
				kind = i
			}
		}
		if prompt < 0 || kind < 0 {
			return fmt.Errorf("missing prompt/type CSV columns")
		}
		for {
			row, err := r.Read()
			if err == io.EOF {
				break
			}
			if err != nil {
				return err
			}
			label := 0
			if strings.EqualFold(row[kind], "jailbreak") {
				label = 1
			}
			if err := write(row[prompt], label, "jackhhao"); err != nil {
				return err
			}
		}
	}
	for _, split := range []string{"train", "test"} {
		for offset := 0; ; {
			q := url.Values{"dataset": {"deepset/prompt-injections"}, "config": {"default"}, "split": {split}, "offset": {strconv.Itoa(offset)}, "length": {"100"}}
			b, err := get(client, "https://datasets-server.huggingface.co/rows?"+q.Encode())
			if err != nil {
				return err
			}
			var page struct {
				Rows []struct {
					Row struct {
						Text  string `json:"text"`
						Label *int   `json:"label"`
					} `json:"row"`
				} `json:"rows"`
				Total int `json:"num_rows_total"`
			}
			if err := json.Unmarshal(b, &page); err != nil {
				return err
			}
			if len(page.Rows) == 0 || page.Total <= 0 {
				return fmt.Errorf("empty or malformed dataset page at %s offset=%d", split, offset)
			}
			for _, item := range page.Rows {
				if item.Row.Label == nil {
					return fmt.Errorf("missing label in dataset page")
				}
				if err := write(item.Row.Text, *item.Row.Label, "deepset"); err != nil {
					return err
				}
			}
			offset += len(page.Rows)
			if offset >= page.Total {
				break
			}
		}
	}
	if counts[0] == 0 || counts[1] == 0 {
		return fmt.Errorf("dataset must contain both labels")
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), out); err != nil {
		return err
	}
	fmt.Printf("%s: attack=%d benign=%d\n", out, counts[1], counts[0])
	return nil
}

func main() {
	out := flag.String("out", "datasets/training/external.jsonl", "output JSONL path")
	flag.Parse()
	if err := fetch(*out, &http.Client{Timeout: 60 * time.Second}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
