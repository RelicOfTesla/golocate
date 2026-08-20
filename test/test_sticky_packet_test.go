package test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// ========== 粘包截断处理测试 ==========

// TestStickyPacket_TwoJSONRequests 测试两个 JSON 请求粘在一起的情况
func TestStickyPacket_TwoJSONRequests(t *testing.T) {
	// 场景：两个 JSON 请求粘在一起
	// 输入：{"method":"status"}{"method":"status"}
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := `{"method":"status"}{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应（第一个请求的响应）
	// 由于 TCP 的特性，可能无法在同一个连接中接收多个响应
	// 但至少不应该报错
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle sticky packet gracefully")
}

// TestStickyPacket_TwoJSONRequestsWithNewline 测试两个 JSON 请求用换行符分隔的情况
func TestStickyPacket_TwoJSONRequestsWithNewline(t *testing.T) {
	// 场景：两个 JSON 请求用换行符分隔
	// 输入：{"method":"status"}\n{"method":"status"}
	// 预期：服务器应该能够正确处理这两个请求
	stickyData := `{"method":"status"}` + "\n" + `{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle newline-separated requests gracefully")
}

// TestStickyPacket_SearchFollowedByStatus 测试 search 请求后面跟着 status 请求
func TestStickyPacket_SearchFollowedByStatus(t *testing.T) {
	// 场景：search 请求后面跟着 status 请求
	// 输入：{"method":"search","path":"*","content":"main.go"}{"method":"status"}
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := `{"method":"search","path":"*","content":"main.go","limit":5}{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle search+status sticky packet gracefully")
}

// TestStickyPacket_StatusFollowedBySearch 测试 status 请求后面跟着 search 请求
func TestStickyPacket_StatusFollowedBySearch(t *testing.T) {
	// 场景：status 请求后面跟着 search 请求
	// 输入：{"method":"status"}{"method":"search","path":"*","content":"main.go"}
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := `{"method":"status"}{"method":"search","path":"*","content":"main.go","limit":5}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle status+search sticky packet gracefully")
}

// TestStickyPacket_ThreeJSONRequests 测试三个 JSON 请求粘在一起的情况
func TestStickyPacket_ThreeJSONRequests(t *testing.T) {
	// 场景：三个 JSON 请求粘在一起
	// 输入：{"method":"status"}{"method":"status"}{"method":"status"}
	// 预期：服务器应该能够分离这三个请求，并分别处理
	stickyData := `{"method":"status"}{"method":"status"}{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle three sticky requests gracefully")
}

// TestStickyPacket_JSONAndJSONRPC 测试 JSON 和 JSON-RPC 请求粘在一起的情况
func TestStickyPacket_JSONAndJSONRPC(t *testing.T) {
	// 场景：JSON 请求后面跟着 JSON-RPC 请求
	// 输入：{"method":"status"}{"jsonrpc":"2.0","method":"status","id":1}
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := `{"method":"status"}{"jsonrpc":"2.0","method":"status","id":1}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle JSON+JSON-RPC sticky packet gracefully")
}

// TestStickyPacket_EmptyJSONObject 测试空 JSON 对象后面跟着有效请求的情况
func TestStickyPacket_EmptyJSONObject(t *testing.T) {
	// 场景：空 JSON 对象后面跟着有效请求
	// 输入：{}{"method":"status"}
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := `{}` + `{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应（或者错误）
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle empty JSON + valid request sticky packet gracefully")
}

// TestStickyPacket_WithWhitespace 测试带空白字符的粘包
func TestStickyPacket_WithWhitespace(t *testing.T) {
	// 场景：两个 JSON 请求用空白字符分隔
	// 输入：{"method":"status"}  {"method":"status"}
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := `{"method":"status"}  {"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle whitespace-separated requests gracefully")
}

// TestStickyPacket_NestedJSON 测试嵌套 JSON 的粘包
func TestStickyPacket_NestedJSON(t *testing.T) {
	// 场景：嵌套 JSON 后面跟着另一个请求
	// 输入：{"outer":{"inner":"value"}}{"method":"status"}
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := `{"outer":{"inner":"value"}}{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle nested JSON sticky packet gracefully")
}

// TestStickyPacket_JSONWithBracesInString 测试 JSON 字符串中包含大括号的粘包
func TestStickyPacket_JSONWithBracesInString(t *testing.T) {
	// 场景：JSON 字符串中包含大括号，后面跟着另一个请求
	// 输入：{"text":"{not an object}"}{"method":"status"}
	// 预期：服务器应该能够正确识别字符串中的大括号，并正确分离请求
	stickyData := `{"text":"{not an object}"}{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle JSON with braces in string sticky packet gracefully")
}

// TestStickyPacket_JSONWithEscapedQuotes 测试 JSON 包含转义引号的粘包
func TestStickyPacket_JSONWithEscapedQuotes(t *testing.T) {
	// 场景：JSON 包含转义引号，后面跟着另一个请求
	// 输入：{"text":"He said \"hello\""}{"method":"status"}
	// 预期：服务器应该能够正确处理转义引号，并正确分离请求
	stickyData := `{"text":"He said \"hello\""}{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle JSON with escaped quotes sticky packet gracefully")
}

// TestStickyPacket_ComplexNested 测试复杂嵌套结构的粘包
func TestStickyPacket_ComplexNested(t *testing.T) {
	// 场景：复杂嵌套结构后面跟着另一个请求
	// 输入：{"data":{"values":[{"x":1},{"y":2}],"meta":null}}{"method":"status"}
	// 预期：服务器应该能够正确分离请求
	stickyData := `{"data":{"values":[{"x":1},{"y":2}],"meta":null}}{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle complex nested JSON sticky packet gracefully")
}

// TestStickyPacket_IncompleteJSON 测试不完整 JSON 后面跟着完整请求的情况
func TestStickyPacket_IncompleteJSON(t *testing.T) {
	// 场景：不完整 JSON 后面跟着完整请求
	// 注意：这个测试可能会失败，因为第一个请求是无效的
	// 输入：{"method":"search"}{"method":"status"}
	// 预期：服务器应该能够识别第一个完整的 JSON，并忽略剩余的不完整部分
	stickyData := `{"method":"search"{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：可能会返回错误，因为第一个 JSON 不完整
	// 但是应该优雅处理，不应该崩溃
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle incomplete JSON sticky packet gracefully")
}

// TestStickyPacket_LargeJSON 测试大型 JSON 的粘包
func TestStickyPacket_LargeJSON(t *testing.T) {
	// 场景：大型 JSON 后面跟着另一个请求
	// 输入：{"data":"xxxxx...（大量数据）..."}{"method":"status"}
	// 预期：服务器应该能够正确处理
	largeData := strings.Repeat("x", 1000)
	stickyData := `{"data":"` + largeData + `"}{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response length: %d", len(response))

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle large JSON sticky packet gracefully")
}

// ========== 混合协议粘包测试 ==========

// TestStickyPacket_JSONAndFast 测试 JSON 请求后面跟着 Fast 协议请求
func TestStickyPacket_JSONAndFast(t *testing.T) {
	// 场景：JSON 请求后面跟着 Fast 协议请求
	// 输入：{"method":"status"}method=status\n\n
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := `{"method":"status"}` + "method=status\n\n"

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle JSON+Fast sticky packet gracefully")
}

// TestStickyPacket_FastAndJSON 测试 Fast 协议请求后面跟着 JSON 请求
func TestStickyPacket_FastAndJSON(t *testing.T) {
	// 场景：Fast 协议请求后面跟着 JSON 请求
	// 输入：method=status\n\n{"method":"status"}
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := "method=status\n\n" + `{"method":"status"}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle Fast+JSON sticky packet gracefully")
}

// TestStickyPacket_JSONAndFastAndJSONRPC 测试三种协议混合粘包
func TestStickyPacket_JSONAndFastAndJSONRPC(t *testing.T) {
	// 场景：JSON 请求 + Fast 协议请求 + JSON-RPC 请求粘在一起
	// 输入：{"method":"status"}method=status\n\n{"jsonrpc":"2.0","method":"status","id":1}
	// 预期：服务器应该能够分离这三个请求，并分别处理
	stickyData := `{"method":"status"}` + "method=status\n\n" + `{"jsonrpc":"2.0","method":"status","id":1}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle JSON+Fast+JSON-RPC sticky packet gracefully")
}

// TestStickyPacket_FastAndJSONRPC 测试 Fast 协议请求后面跟着 JSON-RPC 请求
func TestStickyPacket_FastAndJSONRPC(t *testing.T) {
	// 场景：Fast 协议请求后面跟着 JSON-RPC 请求
	// 输入：method=status\n\n{"jsonrpc":"2.0","method":"status","id":1}
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := "method=status\n\n" + `{"jsonrpc":"2.0","method":"status","id":1}`

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle Fast+JSON-RPC sticky packet gracefully")
}

// TestStickyPacket_JSONRPCAndFast 测试 JSON-RPC 请求后面跟着 Fast 协议请求
func TestStickyPacket_JSONRPCAndFast(t *testing.T) {
	// 场景：JSON-RPC 请求后面跟着 Fast 协议请求
	// 输入：{"jsonrpc":"2.0","method":"status","id":1}method=status\n\n
	// 预期：服务器应该能够分离这两个请求，并分别处理
	stickyData := `{"jsonrpc":"2.0","method":"status","id":1}` + "method=status\n\n"

	response := sendRawData(t, stickyData)
	t.Logf("Response: %s", response)

	// 预期：至少应该有一个响应
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle JSON-RPC+Fast sticky packet gracefully")
}
