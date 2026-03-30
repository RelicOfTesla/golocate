// Package protocol provides message splitting utilities for handling TCP sticky packets.
package protocol

import (
	"reflect"
	"testing"
)

func TestSplitJSONMessages(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		// ===== Basic cases =====
		{
			name:     "empty input",
			input:    "",
			expected: nil,
		},
		{
			name:     "single JSON object",
			input:    `{"method":"search"}`,
			expected: []string{`{"method":"search"}`},
		},
		{
			name:     "single JSON object with whitespace",
			input:    `  {"method":"search"}  `,
			expected: []string{`{"method":"search"}`},
		},
		
		// ===== Two JSON objects merged =====
		{
			name:     "two JSON objects merged",
			input:    `{"method":"search"}{"method":"status"}`,
			expected: []string{`{"method":"search"}`, `{"method":"status"}`},
		},
		{
			name:     "two JSON objects with newline separator",
			input:    `{"method":"search"}` + "\n" + `{"method":"status"}`,
			expected: []string{`{"method":"search"}`, `{"method":"status"}`},
		},
		{
			name:     "two JSON objects with whitespace separator",
			input:    `{"method":"search"}  {"method":"status"}`,
			expected: []string{`{"method":"search"}`, `{"method":"status"}`},
		},
		
		// ===== Multiple JSON objects merged =====
		{
			name:     "three JSON objects merged",
			input:    `{"a":1}{"b":2}{"c":3}`,
			expected: []string{`{"a":1}`, `{"b":2}`, `{"c":3}`},
		},
		{
			name:     "four JSON objects merged",
			input:    `{"a":1}{"b":2}{"c":3}{"d":4}`,
			expected: []string{`{"a":1}`, `{"b":2}`, `{"c":3}`, `{"d":4}`},
		},
		
		// ===== Nested JSON objects =====
		{
			name:     "nested JSON objects",
			input:    `{"outer":{"inner":"value"}}`,
			expected: []string{`{"outer":{"inner":"value"}}`},
		},
		{
			name:     "nested followed by simple",
			input:    `{"outer":{"inner":"value"}}{"simple":"object"}`,
			expected: []string{`{"outer":{"inner":"value"}}`, `{"simple":"object"}`},
		},
		{
			name:     "deeply nested",
			input:    `{"l1":{"l2":{"l3":"value"}}}`,
			expected: []string{`{"l1":{"l2":{"l3":"value"}}}`},
		},
		
		// ===== JSON with strings containing braces =====
		{
			name:     "JSON with braces in string",
			input:    `{"text":"{not an object}"}`,
			expected: []string{`{"text":"{not an object}"}`},
		},
		{
			name:     "JSON with braces in string followed by another",
			input:    `{"text":"{not an object}"}{"method":"status"}`,
			expected: []string{`{"text":"{not an object}"}`, `{"method":"status"}`},
		},
		{
			name:     "JSON with escaped quotes",
			input:    `{"text":"He said \"hello\""}`,
			expected: []string{`{"text":"He said \"hello\""}`},
		},
		{
			name:     "JSON with escaped quotes followed by another",
			input:    `{"text":"He said \"hello\""}{"method":"status"}`,
			expected: []string{`{"text":"He said \"hello\""}`, `{"method":"status"}`},
		},
		
		// ===== Real-world scenarios =====
		{
			name:     "search request followed by status request",
			input:    `{"method":"search","content":"test"}{"method":"status"}`,
			expected: []string{`{"method":"search","content":"test"}`, `{"method":"status"}`},
		},
		{
			name:     "status request followed by search request",
			input:    `{"method":"status"}{"method":"search","content":"test"}`,
			expected: []string{`{"method":"status"}`, `{"method":"search","content":"test"}`},
		},
		
		// ===== Empty JSON objects =====
		{
			name:     "empty JSON object",
			input:    `{}`,
			expected: []string{`{}`},
		},
		{
			name:     "two empty JSON objects",
			input:    `{}{}`,
			expected: []string{`{}`, `{}`},
		},
		{
			name:     "empty followed by non-empty",
			input:    `{}` + `{"method":"search"}`,
			expected: []string{`{}`, `{"method":"search"}`},
		},
		
		// ===== JSON with arrays =====
		{
			name:     "JSON with array",
			input:    `{"items":[1,2,3]}`,
			expected: []string{`{"items":[1,2,3]}`},
		},
		{
			name:     "JSON with array of objects",
			input:    `{"items":[{"a":1},{"b":2}]}`,
			expected: []string{`{"items":[{"a":1},{"b":2}]}`},
		},
		
		// ===== Complex nested structures =====
		{
			name:     "complex nested with array",
			input:    `{"data":{"values":[{"x":1},{"y":2}],"meta":null}}`,
			expected: []string{`{"data":{"values":[{"x":1},{"y":2}],"meta":null}}`},
		},
		{
			name:     "complex nested followed by simple",
			input:    `{"data":{"values":[{"x":1}]}}{"simple":true}`,
			expected: []string{`{"data":{"values":[{"x":1}]}}`, `{"simple":true}`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SplitJSONMessages([]byte(tt.input))
			
			// Convert [][]byte to []string for comparison
			var resultStrings []string
			for _, msg := range result {
				resultStrings = append(resultStrings, string(msg))
			}
			
			if !reflect.DeepEqual(resultStrings, tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, resultStrings)
			}
		})
	}
}

func TestSplitJSONMessagesWithRemainder(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		expectedMessages  []string
		expectedRemainder string
	}{
		// ===== Complete messages =====
		{
			name:              "complete message",
			input:             `{"method":"search"}`,
			expectedMessages:  []string{`{"method":"search"}`},
			expectedRemainder: "",
		},
		{
			name:              "two complete messages",
			input:             `{"a":1}{"b":2}`,
			expectedMessages:  []string{`{"a":1}`, `{"b":2}`},
			expectedRemainder: "",
		},
		
		// ===== Incomplete messages =====
		{
			name:              "incomplete JSON at end",
			input:             `{"a":1}{"b":2}{"c":`,
			expectedMessages:  []string{`{"a":1}`, `{"b":2}`},
			expectedRemainder: `{"c":`,
		},
		{
			name:              "only incomplete JSON",
			input:             `{"method":"search"`,
			expectedMessages:  nil,
			expectedRemainder: `{"method":"search"`,
		},
		{
			name:              "incomplete JSON with partial field",
			input:             `{"method":"sea`,
			expectedMessages:  nil,
			expectedRemainder: `{"method":"sea`,
		},
		
		// ===== Empty cases =====
		{
			name:              "empty input",
			input:             "",
			expectedMessages:  nil,
			expectedRemainder: "",
		},
		{
			name:              "only whitespace",
			input:             "   \t\n  ",
			expectedMessages:  nil,
			expectedRemainder: "",
		},
		
		// ===== Mixed complete and incomplete =====
		{
			name:              "complete followed by incomplete",
			input:             `{"method":"status"}{"method":"search","content":"test`,
			expectedMessages:  []string{`{"method":"status"}`},
			expectedRemainder: `{"method":"search","content":"test`,
		},
		{
			name:              "multiple complete followed by incomplete",
			input:             `{"a":1}{"b":2}{"c":3}{"d":`,
			expectedMessages:  []string{`{"a":1}`, `{"b":2}`, `{"c":3}`},
			expectedRemainder: `{"d":`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, remainder := SplitJSONMessagesWithRemainder([]byte(tt.input))
			
			// Convert [][]byte to []string for comparison
			var messageStrings []string
			for _, msg := range messages {
				messageStrings = append(messageStrings, string(msg))
			}
			
			if !reflect.DeepEqual(messageStrings, tt.expectedMessages) {
				t.Errorf("expected messages %v, got %v", tt.expectedMessages, messageStrings)
			}
			
			remainderStr := string(remainder)
			if remainderStr != tt.expectedRemainder {
				t.Errorf("expected remainder %q, got %q", tt.expectedRemainder, remainderStr)
			}
		})
	}
}

// BenchmarkSplitJSONMessages benchmarks the SplitJSONMessages function
func BenchmarkSplitJSONMessages(b *testing.B) {
	benchmarks := []struct {
		name  string
		input string
	}{
		{"single_small", `{"method":"search"}`},
		{"single_large", `{"data":"` + string(make([]byte, 1000)) + `"}`},
		{"two_small", `{"a":1}{"b":2}`},
		{"two_large", `{"data":"` + string(make([]byte, 500)) + `"}{"data":"` + string(make([]byte, 500)) + `"}`},
		{"ten_small", `{"a":1}{"b":2}{"c":3}{"d":4}{"e":5}{"f":6}{"g":7}{"h":8}{"i":9}{"j":10}`},
		{"nested", `{"l1":{"l2":{"l3":{"l4":{"l5":"value"}}}}}`},
		{"with_escaped_quotes", `{"text":"He said \"hello\" and she said \"world\""}{"method":"status"}`},
		{"with_braces_in_string", `{"text":"{not an object} and {also not}"}{"method":"status"}`},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			data := []byte(bm.input)
			for i := 0; i < b.N; i++ {
				SplitJSONMessages(data)
			}
		})
	}
}
