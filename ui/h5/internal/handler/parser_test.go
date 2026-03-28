package handler

import (
	"fmt"
	
)

func main() {
	tests := []struct {
		input    string
		expected SearchParams
	}{
		{
			input: "test123",
			expected: SearchParams{Content: "test123", Limit: 100},
		},
		{
			input: "test123 -content:xxxx",
			expected: SearchParams{Content: "xxxx", Limit: 100},
		},
		{
			input: "test123 -path:/home/user",
			expected: SearchParams{Content: "test123", Path: "/home/user", Limit: 100},
		},
		{
			input: "test123 -ignore-case -limit:50",
			expected: SearchParams{Content: "test123", IgnoreCase: true, Limit: 50},
		},
		{
			input: "test123 -i -l:50",
			expected: SearchParams{Content: "test123", IgnoreCase: true, Limit: 50},
		},
		{
			input: "test123 -content:\"xxx yyy\"",
			expected: SearchParams{Content: "xxx yyy", Limit: 100},
		},
	}

	fmt.Println("=== 参数解析测试 ===")
	fmt.Println()

	for i, test := range tests {
		result := ParseSearchQuery(test.input)
		passed := result.Content == test.expected.Content &&
			result.Path == test.expected.Path &&
			result.IgnoreCase == test.expected.IgnoreCase &&
			result.Limit == test.expected.Limit

		status := "✅ PASS"
		if !passed {
			status = "❌ FAIL"
		}

		fmt.Printf("测试 %d: %s\n", i+1, status)
		fmt.Printf("  输入: %s\n", test.input)
		fmt.Printf("  期望: Content=%q, Path=%q, IgnoreCase=%v, Limit=%d\n",
			test.expected.Content, test.expected.Path, test.expected.IgnoreCase, test.expected.Limit)
		fmt.Printf("  实际: Content=%q, Path=%q, IgnoreCase=%v, Limit=%d\n",
			result.Content, result.Path, result.IgnoreCase, result.Limit)
		fmt.Println()
	}
}
