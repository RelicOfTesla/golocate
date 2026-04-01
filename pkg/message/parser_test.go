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
