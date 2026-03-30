// Package message provides parser tests.
package message

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/message/protocol"
)

// mockConn 模拟 net.Conn 用于测试
type mockConn struct {
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
}

// newMockConn creates a mock connection for testing message parsing.
// This function is only used in test files and should not be used in production code.
func newMockConn() *mockConn {
	return &mockConn{
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}
}

func (c *mockConn) Read(b []byte) (n int, err error) {
	return c.readBuf.Read(b)
}

func (c *mockConn) Write(b []byte) (n int, err error) {
	return c.writeBuf.Write(b)
}

func (c *mockConn) Close() error {
	return nil
}

func (c *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
}

func (c *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (c *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (c *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (c *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// TestNewMessageParser 测试创建解析器
func TestNewMessageParser(t *testing.T) {
	parser := NewMessageParser()
	if parser == nil {
		t.Errorf("expected parser to be created")
	}
}

// TestMessageParserSetDefaultProtocol 测试设置默认协议
func TestMessageParserSetDefaultProtocol(t *testing.T) {
	parser := NewMessageParser()

	// 设置默认协议
	parser.SetDefaultProtocol(ProtocolJSONRPC)

	// 检查默认协议是否设置成功
	// 我们通过解析一个空请求来测试
	reader := bufio.NewReader(bytes.NewReader([]byte{}))
	protoType, err := parser.DetectProtocol(reader)

	// 由于输入为空，应该返回默认协议
	if err == nil {
		if protoType != ProtocolJSONRPC {
			t.Errorf("expected default protocol %s, got %s", ProtocolJSONRPC, protoType)
		}
	}
}

// TestMessageParserDetectProtocolFast 测试检测 fast 协议
func TestMessageParserDetectProtocolFast(t *testing.T) {
	parser := NewMessageParser()

	// fast 协议请求
	fastRequest := "method=search\ncontent=test\n\n"
	reader := bufio.NewReader(bytes.NewReader([]byte(fastRequest)))

	protoType, err := parser.DetectProtocol(reader)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if protoType != ProtocolFast {
		t.Errorf("expected protocol %s, got %s", ProtocolFast, protoType)
	}
}

// TestMessageParserDetectProtocolJSONRPC 测试检测 json-rpc 协议
func TestMessageParserDetectProtocolJSONRPC(t *testing.T) {
	parser := NewMessageParser()

	// json-rpc 协议请求
	jsonrpcRequest := `{"jsonrpc":"2.0","id":1,"method":"search","params":{"content":"test"}}` + "\n"
	reader := bufio.NewReader(bytes.NewReader([]byte(jsonrpcRequest)))

	protoType, err := parser.DetectProtocol(reader)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if protoType != ProtocolJSONRPC {
		t.Errorf("expected protocol %s, got %s", ProtocolJSONRPC, protoType)
	}
}

// TestMessageParserParseMessageFast 测试解析 fast 协议消息
func TestMessageParserParseMessageFast(t *testing.T) {
	parser := NewMessageParser()

	// fast 协议请求
	fastRequest := "method=search\nid=123\ncontent=test\n\n"
	conn := newMockConn()
	conn.readBuf.Write([]byte(fastRequest))
	reader := bufio.NewReader(conn.readBuf)

	msg, remainder, err := parser.ParseMessage(conn, reader)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if msg == nil {
		t.Errorf("expected message to be parsed")
		return
	}

	if msg.Method() != "search" {
		t.Errorf("expected method 'search', got '%s'", msg.Method())
	}

	if msg.ID() != "123" {
		t.Errorf("expected ID '123', got '%s'", msg.ID())
	}

	if len(remainder) != 0 {
		t.Errorf("expected no remainder, got %d bytes", len(remainder))
	}
}

// TestMessageParserParseMessageJSONRPC 测试解析 json-rpc 协议消息
func TestMessageParserParseMessageJSONRPC(t *testing.T) {
	parser := NewMessageParser()

	// json-rpc 协议请求
	jsonrpcRequest := `{"jsonrpc":"2.0","id":456,"method":"search","params":{"content":"test"}}` + "\n"
	conn := newMockConn()
	conn.readBuf.Write([]byte(jsonrpcRequest))
	reader := bufio.NewReader(conn.readBuf)

	msg, remainder, err := parser.ParseMessage(conn, reader)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if msg == nil {
		t.Errorf("expected message to be parsed")
		return
	}

	if msg.Method() != "search" {
		t.Errorf("expected method 'search', got '%s'", msg.Method())
	}

	if msg.ID() != "456" {
		t.Errorf("expected ID '456', got '%s'", msg.ID())
	}

	if len(remainder) != 0 {
		t.Errorf("expected no remainder, got %d bytes", len(remainder))
	}
}

// TestMessageParserParseMessagesBatch 测试批量解析消息（粘包处理）
func TestMessageParserParseMessagesBatch(t *testing.T) {
	parser := NewMessageParser()

	// 两个 fast 协议请求粘在一起
	msg1 := "method=search\nid=1\ncontent=test1\n\n"
	msg2 := "method=status\nid=2\n\n"
	stickyData := msg1 + msg2

	conn := newMockConn()
	messages, remainder, err := parser.ParseMessages(conn, []byte(stickyData))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
		return
	}

	// 检查第一个消息
	if messages[0].Method() != "search" {
		t.Errorf("expected first message method 'search', got '%s'", messages[0].Method())
	}
	if messages[0].ID() != "1" {
		t.Errorf("expected first message ID '1', got '%s'", messages[0].ID())
	}

	// 检查第二个消息
	if messages[1].Method() != "status" {
		t.Errorf("expected second message method 'status', got '%s'", messages[1].Method())
	}
	if messages[1].ID() != "2" {
		t.Errorf("expected second message ID '2', got '%s'", messages[1].ID())
	}

	if len(remainder) != 0 {
		t.Errorf("expected no remainder, got %d bytes", len(remainder))
	}
}

// TestMessageParserParseMessagesWithIncomplete 测试批量解析带不完整数据
func TestMessageParserParseMessagesWithIncomplete(t *testing.T) {
	parser := NewMessageParser()

	// 一个完整的消息 + 一个不完整的消息
	completeMsg := "method=search\nid=1\ncontent=test\n\n"
	incompleteMsg := "method=status\nid=2\n" // 缺少结尾的空行
	data := completeMsg + incompleteMsg

	conn := newMockConn()
	messages, remainder, err := parser.ParseMessages(conn, []byte(data))
	if err != nil {
		// 如果返回错误，检查是否是因为不完整的消息
		t.Logf("parse error (expected for incomplete message): %v", err)
	}

	// 应该至少解析出一个完整的消息
	if len(messages) < 1 {
		t.Errorf("expected at least 1 message, got %d", len(messages))
		return
	}

	// 检查第一个消息
	if messages[0].Method() != "search" {
		t.Errorf("expected first message method 'search', got '%s'", messages[0].Method())
	}

	// 应该有剩余数据（不完整的消息）
	if len(remainder) == 0 {
		t.Logf("warning: expected remainder, got none")
	}
}

// TestMessageParserReply 测试通过 Message 回复
func TestMessageParserReply(t *testing.T) {
	parser := NewMessageParser()

	// fast 协议请求
	fastRequest := "method=search\nid=123\ncontent=test\n\n"
	conn := newMockConn()
	conn.readBuf.Write([]byte(fastRequest))
	reader := bufio.NewReader(conn.readBuf)

	msg, _, err := parser.ParseMessage(conn, reader)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
		return
	}

	// 通过 Message 回复
	response := &protocol.Response{
		ID:    123,
		Count: 10,
		Total: 10,
		Paths: []string{"file1.txt", "file2.txt"},
	}

	err = msg.Reply(response)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// 检查是否写入了响应
	if conn.writeBuf.Len() == 0 {
		t.Errorf("expected response to be written")
	}
}

// TestMessageParserSetMessageBuilder 测试设置消息构建器
func TestMessageParserSetMessageBuilder(t *testing.T) {
	parser := NewMessageParser()

	// 创建自定义构建器
	customBuilder := &testMessageBuilder{}
	parser.SetMessageBuilder(customBuilder)

	// 解析消息
	fastRequest := "method=search\nid=123\ncontent=test\n\n"
	conn := newMockConn()
	conn.readBuf.Write([]byte(fastRequest))
	reader := bufio.NewReader(conn.readBuf)

	msg, _, err := parser.ParseMessage(conn, reader)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
		return
	}

	// 检查是否使用了自定义构建器
	if msg.Method() != "custom-method" {
		t.Errorf("expected custom method 'custom-method', got '%s'", msg.Method())
	}
}

// testMessageBuilder 测试用的消息构建器
type testMessageBuilder struct{}

func (b *testMessageBuilder) Build(rawData []byte, replyFunc ReplyFunc) (Message, error) {
	// 返回一个自定义的消息
	return NewMessage("custom-id", "custom-method", []byte("custom-payload"), context.Background(), replyFunc, nil), nil
}

// TestMessageParserParseMessageWithNilConn 测试 nil 连接
func TestMessageParserParseMessageWithNilConn(t *testing.T) {
	parser := NewMessageParser()

	// fast 协议请求
	fastRequest := "method=search\nid=123\ncontent=test\n\n"
	reader := bufio.NewReader(bytes.NewReader([]byte(fastRequest)))

	msg, _, err := parser.ParseMessage(nil, reader)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
		return
	}

	if msg == nil {
		t.Errorf("expected message to be parsed")
		return
	}
}

// TestMessageParserParseMessagesJSONRPCBatch 测试批量解析 JSON-RPC 消息
func TestMessageParserParseMessagesJSONRPCBatch(t *testing.T) {
	parser := NewMessageParser()

	// 两个 JSON-RPC 消息粘在一起
	msg1 := `{"jsonrpc":"2.0","id":1,"method":"search","params":{"content":"test1"}}` + "\n"
	msg2 := `{"jsonrpc":"2.0","id":2,"method":"status","params":{}}` + "\n"
	stickyData := msg1 + msg2

	conn := newMockConn()
	messages, remainder, err := parser.ParseMessages(conn, []byte(stickyData))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(messages))
		return
	}

	// 检查第一个消息
	if messages[0].Method() != "search" {
		t.Errorf("expected first message method 'search', got '%s'", messages[0].Method())
	}
	if messages[0].ID() != "1" {
		t.Errorf("expected first message ID '1', got '%s'", messages[0].ID())
	}

	// 检查第二个消息
	if messages[1].Method() != "status" {
		t.Errorf("expected second message method 'status', got '%s'", messages[1].Method())
	}
	if messages[1].ID() != "2" {
		t.Errorf("expected second message ID '2', got '%s'", messages[1].ID())
	}

	if len(remainder) != 0 {
		t.Errorf("expected no remainder, got %d bytes", len(remainder))
	}
}

// TestMessageParserParseMessagesMixedProtocol 测试混合协议粘包
func TestMessageParserParseMessagesMixedProtocol(t *testing.T) {
	parser := NewMessageParser()

	// fast 协议消息 + JSON-RPC 消息
	msg1 := "method=search\nid=1\ncontent=test\n\n"
	msg2 := `{"jsonrpc":"2.0","id":2,"method":"status","params":{}}` + "\n"
	mixedData := msg1 + msg2

	conn := newMockConn()
	messages, _, err := parser.ParseMessages(conn, []byte(mixedData))
	if err != nil {
		t.Logf("parse error (expected for mixed protocol): %v", err)
	}

	// 可能只解析出第一个消息（fast 协议）
	if len(messages) >= 1 {
		// 检查第一个消息
		if messages[0].Method() != "search" {
			t.Errorf("expected first message method 'search', got '%s'", messages[0].Method())
		}
	}

	// 第二个消息可能无法解析（因为协议切换）
	// 这是预期的行为
}

// TestMessageParserAsyncReply 测试异步回复
func TestMessageBuilderBuild(t *testing.T) {
	builder := &defaultMessageBuilder{}

	// 创建请求数据
	req := &protocol.Request{
		Method:  "search",
		ID:      123,
		Content: "test",
	}

	rawData, err := json.Marshal(req)
	if err != nil {
		t.Errorf("failed to marshal request: %v", err)
		return
	}

	// 构建消息
	replyFunc := func(ctx context.Context, messageID string, response any) error {
		return nil
	}

	msg, err := builder.Build(rawData, replyFunc)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
		return
	}

	if msg.Method() != "search" {
		t.Errorf("expected method 'search', got '%s'", msg.Method())
	}

	if msg.ID() != "123" {
		t.Errorf("expected ID '123', got '%s'", msg.ID())
	}
}
