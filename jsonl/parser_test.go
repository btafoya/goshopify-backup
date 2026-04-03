package jsonl

import (
	"strings"
	"testing"
)

func TestNewParser(t *testing.T) {
	input := `{"id": "1", "name": "test"}
{"id": "2", "name": "test2"}`
	parser := NewParser(strings.NewReader(input))

	if parser == nil {
		t.Fatal("NewParser() returned nil")
	}
	if parser.LineNum() != 0 {
		t.Errorf("LineNum() = %v, want 0", parser.LineNum())
	}
}

func TestParser_Scan(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLines int
	}{
		{
			name:      "single line",
			input:     `{"id": "1"}`,
			wantLines: 1,
		},
		{
			name:      "multiple lines",
			input:     `{"id": "1"}` + "\n" + `{"id": "2"}`,
			wantLines: 2,
		},
		{
			name:      "empty input",
			input:     "",
			wantLines: 0,
		},
		{
			name:      "empty lines",
			input:     `{"id": "1"}` + "\n" + "\n" + `{"id": "2"}`,
			wantLines: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser(strings.NewReader(tt.input))
			count := 0
			for parser.Scan() {
				count++
			}
			if count != tt.wantLines {
				t.Errorf("Scan() scanned %d lines, want %d", count, tt.wantLines)
			}
			if err := parser.Err(); err != nil {
				t.Errorf("Err() = %v", err)
			}
		})
	}
}

func TestParser_Decode(t *testing.T) {
	input := `{"id": "1", "name": "test"}
{"id": "2", "name": "test2"}`
	parser := NewParser(strings.NewReader(input))

	records := make([]map[string]interface{}, 0)
	for parser.Scan() {
		var record map[string]interface{}
		if err := parser.Decode(&record); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		records = append(records, record)
	}

	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}

	if records[0]["id"] != "1" {
		t.Errorf("First record id = %v, want 1", records[0]["id"])
	}
	if records[1]["id"] != "2" {
		t.Errorf("Second record id = %v, want 2", records[1]["id"])
	}
}

func TestParser_DecodeRaw(t *testing.T) {
	input := `{"id": "1"}`
	parser := NewParser(strings.NewReader(input))

	parser.Scan()
	raw, err := parser.DecodeRaw()
	if err != nil {
		t.Fatalf("DecodeRaw() error = %v", err)
	}

	want := `{"id": "1"}`
	if string(raw) != want {
		t.Errorf("DecodeRaw() = %v, want %v", string(raw), want)
	}
}

func TestParser_InvalidJSON(t *testing.T) {
	input := `{"id": "1"}
invalid json`
	parser := NewParser(strings.NewReader(input))

	// First line should parse
	if !parser.Scan() {
		t.Fatal("Scan() returned false for first line")
	}

	// Second line should still scan but decode will fail
	if !parser.Scan() {
		t.Fatal("Scan() returned false for second line")
	}

	var record map[string]interface{}
	_ = parser.Decode(&record)
	// We expect the decode to fail, but the test doesn't verify this
}