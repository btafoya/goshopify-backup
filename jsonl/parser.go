package jsonl

import (
	"bufio"
	"encoding/json"
	"io"
)

// Parser streaming-parses JSONL using bufio.Scanner
// Avoids loading entire file into memory
type Parser struct {
	scanner *bufio.Scanner
	lineNum int
}

// NewParser creates a new JSONL parser from an io.Reader
func NewParser(r io.Reader) *Parser {
	return &Parser{
		scanner: bufio.NewScanner(r),
		lineNum: 0,
	}
}

// Scan advances to the next JSON line
// Returns false when scanning is done or error
// Skips empty lines
func (p *Parser) Scan() bool {
	for p.scanner.Scan() {
		line := p.scanner.Bytes()
		if len(line) > 0 {
			p.lineNum++
			return true
		}
	}
	return false
}

// Decode decodes the current JSON line into dst
func (p *Parser) Decode(dst interface{}) error {
	return json.Unmarshal(p.scanner.Bytes(), dst)
}

// DecodeRaw decodes the current JSON line and returns raw bytes
func (p *Parser) DecodeRaw() ([]byte, error) {
	return p.scanner.Bytes(), nil
}

// Err returns any error encountered during scanning
func (p *Parser) Err() error {
	return p.scanner.Err()
}

// LineNum returns the current line number (1-indexed)
func (p *Parser) LineNum() int {
	return p.lineNum
}

// Bytes returns the bytes of the current line
func (p *Parser) Bytes() []byte {
	return p.scanner.Bytes()
}
