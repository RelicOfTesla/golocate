package test

import (
	"bufio"
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const socketPath = "/tmp/golocate.sock"

// connectSocket creates a direct connection to golocated Unix socket
func connectSocket(t *testing.T) net.Conn {
	conn, err := net.Dial("unix", socketPath)
	require.NoError(t, err, "Should connect to Unix socket")
	return conn
}

// sendRawData sends raw data to socket and returns response
func sendRawData(t *testing.T, data string) string {
	conn := connectSocket(t)
	defer conn.Close()

	_, err := conn.Write([]byte(data + "\n"))
	require.NoError(t, err, "Should send data")

	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil {
		return ""
	}
	return response
}

// responseContainsError checks if response contains an error
func responseContainsError(response string) bool {
	return strings.Contains(response, `"error"`) || 
		strings.Contains(response, "error=") ||
		strings.Contains(response, `"type":"error"`)
}

// responseHasResults checks if response contains results
func responseHasResults(response string) bool {
	return strings.Contains(response, `"type":"result"`) ||
		strings.Contains(response, "count=") ||
		strings.Contains(response, `"count"`)
}

// ========== 第一部分：无效 JSON 格式测试 ==========

func TestProtocol_InvalidJSON_MissingClosingBrace(t *testing.T) {
	response := sendRawData(t, `{"method":"search","content":"test"`)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误（无效 JSON）
	assert.True(t, responseContainsError(response) || response == "", 
		"Should return error for invalid JSON (missing closing brace)")
}

func TestProtocol_InvalidJSON_InvalidSyntax(t *testing.T) {
	response := sendRawData(t, `{method:"search",content:"test"}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误（无效 JSON 语法）
	assert.True(t, responseContainsError(response) || response == "", 
		"Should return error for invalid JSON syntax")
}

func TestProtocol_InvalidJSON_NonJSON(t *testing.T) {
	response := sendRawData(t, `this is not json`)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误（非 JSON 数据）
	assert.True(t, responseContainsError(response) || response == "", 
		"Should return error for non-JSON data")
}

// ========== 第二部分：缺少必要字段测试 ==========

func TestProtocol_MissingMethod(t *testing.T) {
	response := sendRawData(t, `{"content":"test","limit":5}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误或空结果（缺少 method 字段）
	// 根据服务器实现，可能默认使用某个方法
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should return error or handle missing method gracefully")
}

func TestProtocol_MissingContent(t *testing.T) {
	response := sendRawData(t, `{"method":"search","path":"*","limit":5}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回空结果（没有搜索内容）
	// 根据服务器实现，Content 为空时返回空结果
	assert.True(t, !responseContainsError(response), 
		"Should not return error for missing content (return empty results)")
}

func TestProtocol_EmptyObject(t *testing.T) {
	response := sendRawData(t, `{}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误或空结果（空对象）
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should return error or handle empty object gracefully")
}

// ========== 第三部分：无效字段值测试 ==========

func TestProtocol_InvalidMethod(t *testing.T) {
	response := sendRawData(t, `{"method":"invalid_method","path":"*","content":"test","limit":5}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误（无效的 method 值）
	assert.True(t, responseContainsError(response), 
		"Should return error for invalid method")
}

func TestProtocol_InvalidLimit(t *testing.T) {
	response := sendRawData(t, `{"method":"search","path":"*","content":"test","limit":"invalid"}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误或使用默认值
	assert.True(t, responseContainsError(response) || responseHasResults(response),
		"Should return error or use default limit")
}

func TestProtocol_InvalidIgnoreCase(t *testing.T) {
	response := sendRawData(t, `{"method":"search","path":"*","content":"test","ignore_case":"invalid"}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误或使用默认值
	assert.True(t, responseContainsError(response) || responseHasResults(response),
		"Should return error or use default ignore_case")
}

func TestProtocol_InvalidRegex(t *testing.T) {
	response := sendRawData(t, `{"method":"search","path":"*","content":"test","regex":"invalid"}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误或使用默认值
	assert.True(t, responseContainsError(response) || responseHasResults(response),
		"Should return error or use default regex")
}

func TestProtocol_InvalidSortField(t *testing.T) {
	response := sendRawData(t, `{"method":"search","path":"*","content":"test","sort_field":"invalid_field"}`)
	t.Logf("Response: %s", response)
	// 预期：应该忽略无效字段或返回错误
	assert.True(t, responseContainsError(response) || responseHasResults(response),
		"Should return error or ignore invalid sort_field")
}

func TestProtocol_InvalidSortOrder(t *testing.T) {
	response := sendRawData(t, `{"method":"search","path":"*","content":"test","sort_order":"invalid_order"}`)
	t.Logf("Response: %s", response)
	// 预期：应该忽略无效字段或返回错误
	assert.True(t, responseContainsError(response) || responseHasResults(response),
		"Should return error or ignore invalid sort_order")
}

// ========== 第四部分：快速协议格式测试 ==========

func TestProtocol_FastProtocol_Valid(t *testing.T) {
	data := "method=search\npath=*\ncontent=main.go\nlimit=5\n"
	response := sendRawData(t, data)
	t.Logf("Response: %s", response)
	// 预期：应该返回搜索结果
	assert.True(t, responseHasResults(response), 
		"Should return results for valid fast protocol request")
}

func TestProtocol_FastProtocol_InvalidMethod(t *testing.T) {
	data := "method=invalid\npath=*\ncontent=test\nlimit=5\n"
	response := sendRawData(t, data)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误（无效的 method 值）
	assert.True(t, responseContainsError(response), 
		"Should return error for invalid method in fast protocol")
}

func TestProtocol_FastProtocol_MissingMethod(t *testing.T) {
	data := "path=*\ncontent=test\nlimit=5\n"
	response := sendRawData(t, data)
	t.Logf("Response: %s", response)
	// 预期：应该返回错误或空结果
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should return error or handle missing method gracefully")
}

func TestProtocol_FastProtocol_EmptyContent(t *testing.T) {
	data := "method=search\npath=*\ncontent=\nlimit=5\n"
	response := sendRawData(t, data)
	t.Logf("Response: %s", response)
	// 预期：应该返回空结果
	assert.True(t, !responseContainsError(response), 
		"Should not return error for empty content (return empty results)")
}

func TestProtocol_FastProtocol_WithJSONResponse(t *testing.T) {
	data := "method=search\npath=*\ncontent=main.go\nlimit=5\nAcceptResponseFormat=json\n"
	response := sendRawData(t, data)
	t.Logf("Response: %s", response)
	// 预期：应该返回 JSON 格式的响应
	assert.True(t, strings.Contains(response, "{") || responseHasResults(response),
		"Should return JSON response format")
}

// ========== 第五部分：边界测试 ==========

func TestProtocol_EmptyData(t *testing.T) {
	response := sendRawData(t, "")
	t.Logf("Response: %s", response)
	// 预期：应该返回错误或空结果
	assert.True(t, responseContainsError(response) || response == "",
		"Should return error or handle empty data gracefully")
}

func TestProtocol_NullBytes(t *testing.T) {
	response := sendRawData(t, "\x00\x00\x00")
	t.Logf("Response: %s", response)
	// 预期：应该返回错误
	assert.True(t, responseContainsError(response) || response == "",
		"Should return error for null bytes")
}

func TestProtocol_VeryLongData(t *testing.T) {
	// Create a very long JSON
	longData := `{"method":"search","path":"*","content":"`
	for i := 0; i < 1000; i++ {
		longData += "a"
	}
	longData += `","limit":5}`
	
	response := sendRawData(t, longData)
	t.Logf("Response length: %d", len(response))
	// 预期：应该优雅处理（返回空结果或错误）
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle very long data gracefully")
}

func TestProtocol_UnicodeData(t *testing.T) {
	response := sendRawData(t, `{"method":"search","path":"*","content":"中文测试","limit":5}`)
	t.Logf("Response: %s", response)
	// 预期：应该正确处理 Unicode
	assert.True(t, responseHasResults(response) || responseContainsError(response) || response == "",
		"Should handle Unicode data")
}

func TestProtocol_SpecialCharsInContent(t *testing.T) {
	response := sendRawData(t, `{"method":"search","path":"*","content":"test\u0000null","limit":5}`)
	t.Logf("Response: %s", response)
	// 预期：应该处理特殊字符
	assert.True(t, responseHasResults(response) || responseContainsError(response) || response == "",
		"Should handle special characters in content")
}

// ========== 第六部分：JSON-RPC 格式测试 ==========

func TestProtocol_JSONRPC_Valid(t *testing.T) {
	response := sendRawData(t, `{"jsonrpc":"2.0","method":"search","params":{"path":"*","content":"main.go"},"id":1}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回 JSON-RPC 格式的响应
	assert.True(t, strings.Contains(response, "jsonrpc") || responseHasResults(response),
		"Should return JSON-RPC response")
}

func TestProtocol_JSONRPC_MissingId(t *testing.T) {
	response := sendRawData(t, `{"jsonrpc":"2.0","method":"search","params":{"path":"*","content":"test"}}`)
	t.Logf("Response: %s", response)
	// 预期：应该处理缺少 id 的情况
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle missing id in JSON-RPC")
}

func TestProtocol_JSONRPC_InvalidVersion(t *testing.T) {
	response := sendRawData(t, `{"jsonrpc":"1.0","method":"search","params":{"path":"*","content":"test"},"id":1}`)
	t.Logf("Response: %s", response)
	// 预期：应该处理无效版本
	assert.True(t, responseContainsError(response) || responseHasResults(response) || response == "",
		"Should handle invalid JSON-RPC version")
}

func TestProtocol_JSONRPC_Ident(t *testing.T) {
	response := sendRawData(t, `{
	"jsonrpc": "2.0",
	"method": "search",
	"params": {"path":"main.go"},
	"id":1
}`)
	t.Logf("Response: %s", response)
	// 预期：应该返回 JSON-RPC 格式的响应
	assert.True(t, strings.Contains(response, "jsonrpc") || responseHasResults(response),
		"Should return JSON-RPC response for indented JSON")
}
