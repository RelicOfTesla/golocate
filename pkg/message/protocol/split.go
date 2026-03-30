// Package protocol provides message splitting utilities for handling TCP sticky packets.
package protocol

import (
	"bytes"
	"fmt"
)

// SplitJSONMessages splits concatenated JSON messages (handles TCP sticky packets).
// This function identifies JSON object boundaries by matching braces.
//
// Input examples:
//   - `{"method":"search"}{"method":"status"}` (two JSON objects merged)
//   - `{"method":"search"}\n{"method":"status"}` (with newline separator)
//   - `{"method":"search"}  {"method":"status"}` (with whitespace separator)
//
// Output: slice of individual JSON strings
//   - `["{"method":"search"}", "{"method":"status"}"]`
//
// This function handles:
//   - Nested JSON objects
//   - Strings containing braces (escaped quotes)
//   - Various whitespace separators
//   - Mixed valid/invalid JSON (returns valid ones first)
func SplitJSONMessages(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}

	var messages [][]byte
	decoder := newJSONDecoder(data)

	for {
		msg, err := decoder.decodeNext()
		if err != nil {
			// No more complete JSON objects
			break
		}
		messages = append(messages, msg)
	}

	return messages
}

// jsonDecoder is a simple JSON decoder that can extract individual JSON objects
// from concatenated data without fully parsing them.
type jsonDecoder struct {
	data   []byte
	offset int
}

func newJSONDecoder(data []byte) *jsonDecoder {
	return &jsonDecoder{
		data:   data,
		offset: 0,
	}
}

// decodeNext extracts the next complete JSON object from the data.
// Returns the raw bytes of the JSON object (including surrounding whitespace trimmed).
// If the JSON object is incomplete, the offset is NOT modified (caller can access remainder).
func (d *jsonDecoder) decodeNext() ([]byte, error) {
	// Save current offset in case we need to restore it
	savedOffset := d.offset

	// Skip leading whitespace
	d.skipWhitespace()

	if d.offset >= len(d.data) {
		// No more data, restore offset
		d.offset = savedOffset
		return nil, fmt.Errorf("no more data")
	}

	// Check if we're at the start of a JSON object
	if d.data[d.offset] != '{' {
		// Not at start of JSON object, restore offset
		d.offset = savedOffset
		return nil, fmt.Errorf("not at start of JSON object")
	}

	// Find the matching closing brace
	start := d.offset
	depth := 0
	inString := false
	escapeNext := false

	for d.offset < len(d.data) {
		ch := d.data[d.offset]

		if escapeNext {
			// Previous character was backslash, skip this character
			escapeNext = false
			d.offset++
			continue
		}

		switch ch {
		case '\\':
			if inString {
				escapeNext = true
			}
		case '"':
			inString = !inString
		case '{':
			if !inString {
				depth++
			}
		case '}':
			if !inString {
				depth--
				if depth == 0 {
					// Found matching closing brace
					d.offset++
					result := d.data[start:d.offset]
					// Trim trailing whitespace
					result = bytes.TrimRight(result, " \t\n\r")
					return result, nil
				}
			}
		}

		d.offset++
	}

	// No complete JSON object found, restore offset so caller can get remainder
	d.offset = savedOffset
	return nil, fmt.Errorf("incomplete JSON object")
}

// skipWhitespace skips whitespace characters
func (d *jsonDecoder) skipWhitespace() {
	for d.offset < len(d.data) {
		ch := d.data[d.offset]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			d.offset++
		} else {
			break
		}
	}
}

// SplitJSONMessagesWithRemainder splits concatenated JSON messages and returns
// the messages and any remaining incomplete data.
// This is useful when the input data contains partial JSON at the end.
//
// Example:
//   - Input: `{"a":1}{"b":2}{"c":`
//   - Output: messages=[`{"a":1}`, `{"b":2}`], remainder=`{"c":`
func SplitJSONMessagesWithRemainder(data []byte) (messages [][]byte, remainder []byte) {
	if len(data) == 0 {
		return nil, nil
	}

	offset := 0

	for offset < len(data) {
		// Skip leading whitespace
		for offset < len(data) {
			ch := data[offset]
			if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
				offset++
			} else {
				break
			}
		}

		if offset >= len(data) {
			break
		}

		// Check if we're at the start of a JSON object
		if data[offset] != '{' {
			break
		}

		// Find the matching closing brace
		start := offset
		depth := 0
		inString := false
		escapeNext := false
		found := false

		for offset < len(data) {
			ch := data[offset]

			if escapeNext {
				escapeNext = false
				offset++
				continue
			}

			switch ch {
			case '\\':
				if inString {
					escapeNext = true
				}
			case '"':
				inString = !inString
			case '{':
				if !inString {
					depth++
				}
			case '}':
				if !inString {
					depth--
					if depth == 0 {
						// Found matching closing brace
						offset++
						found = true
						break
					}
				}
			}

			if found {
				break
			}
			offset++
		}

		if found {
			// Extract the complete JSON object
			msg := data[start:offset]
			// Trim trailing whitespace
			msg = bytes.TrimRight(msg, " \t\n\r")
			messages = append(messages, msg)
		} else {
			// Incomplete JSON object, restore offset to start position
			offset = start
			break
		}
	}

	// Get remaining data
	if offset < len(data) {
		remainder = data[offset:]
	}

	return messages, remainder
}
