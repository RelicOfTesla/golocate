// Package message provides message parser implementation.
package message

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/RelicOfTesla/golocate/pkg/message/protocol"
)

// MessageParser 消息解析器接口
//
// 核心职责：
// - 从连接读取数据
// - 检测并解析协议
// - 处理粘包（拆包+拼包）
// - 构建 Message 对象
// - 提供回复能力
type MessageParser interface {
	// ========== 消息解析 ==========

	// ParseMessage 从连接解析一个完整的消息
	//
	// 参数：
	//   - conn: 网络连接
	//   - reader: 缓冲读取器
	//
	// 返回：
	//   - Message: 解析的消息对象
	//   - []byte: 剩余数据（粘包数据）
	//   - error: 解析失败时返回错误
	//
	// 工作流程：
	//   1. 检测协议类型
	//   2. 使用对应的协议解析器解析消息
	//   3. 构建 Message 对象（包含 reply 能力）
	//   4. 返回剩余数据（如果有粘包）
	//
	// 注意：
	//   - 每次调用只解析一个完整的消息
	//   - 如果有粘包，剩余数据需要在下次调用时处理
	ParseMessage(conn net.Conn, reader *bufio.Reader) (Message, []byte, error)

	// ========== 消息构建 ==========

	// SetMessageBuilder 设置消息构建器
	//
	// 参数：
	//   - builder: 消息构建器
	//
	// 注意：
	//   - 必须在使用前设置
	//   - 用于自定义消息构建逻辑
	SetMessageBuilder(builder MessageBuilder)

	// ========== 配置 ==========

	// SetDefaultProtocol 设置默认协议类型
	//
	// 参数：
	//   - protoType: 协议类型
	//
	// 使用场景：
	//   - 当无法检测协议时使用默认协议
	SetDefaultProtocol(protoType ProtocolType)
}

// ProtocolType 协议类型
type ProtocolType string

const (
	ProtocolFast    ProtocolType = "fast"     // 快速文本协议
	ProtocolJSONRPC ProtocolType = "json-rpc" // JSON-RPC 协议
)

// MessageBuilder 消息构建器接口
//
// 用于从原始数据构建 Message 对象
type MessageBuilder interface {
	// Build 从原始数据构建消息
	//
	// 参数：
	//   - rawData: 原始字节数据
	//   - replyFunc: 回复函数（由 MessageParser 提供）
	//
	// 返回：
	//   - Message: 构建的消息对象
	//   - error: 构建失败时返回错误
	Build(rawData []byte, replyFunc ReplyFunc) (Message, error)
}

// defaultMessageParser MessageParser 的默认实现
type defaultMessageParser struct {
	builder      MessageBuilder
	defaultProto ProtocolType
	mu           sync.Mutex
}

// NewMessageParser 创建消息解析器
func NewMessageParser() MessageParser {
	return &defaultMessageParser{
		defaultProto: ProtocolFast, // 默认使用 fast 协议
	}
}


// ParseMessage 实现 MessageParser 接口
func (p *defaultMessageParser) ParseMessage(conn net.Conn, reader *bufio.Reader) (Message, []byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 使用新的 SelectDecoder 函数选择解码器
	decoder, bufReader, err := protocol.SelectDecoder(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("select decoder failed: %w", err)
	}

	// 使用 DecodeWithRemainder 方法解码请求并获取剩余数据
	req, remainder, err := decoder.DecodeWithRemainder(bufReader)
	if err != nil {
		return nil, nil, fmt.Errorf("decode request failed: %w", err)
	}

	// 创建回复函数
	// 确定响应协议：根据 decoder 类型推断请求协议类型
	var requestProto protocol.ProtocolType
	switch decoder.(type) {
	case *protocol.JSONDecoder:
		requestProto = protocol.ProtocolJSONRPC
	default:
		requestProto = protocol.ProtocolFast
	}
	responseProto := protocol.GetResponseProtocol(requestProto, req.AcceptResponseFormat)
	proto := protocol.GetProtocol(responseProto)

	replyFunc := p.createReplyFunc(conn, proto, req)

	// 序列化请求数据作为 payload
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal request failed: %w", err)
	}

	// 构建消息（如果没有 builder，使用默认构建器）
	builder := p.builder
	if builder == nil {
		builder = &defaultMessageBuilder{}
	}

	// 构建 Message 对象
	msg, err := builder.Build(payload, replyFunc)
	if err != nil {
		return nil, nil, fmt.Errorf("build message failed: %w", err)
	}

	// 设置元数据
	if m, ok := msg.(*defaultMessage); ok {
		m.metadata["protocol"] = "auto-detected"
		m.metadata["arrival_time"] = time.Now()
		if conn != nil {
			m.metadata["remote_addr"] = conn.RemoteAddr().String()
		}
	}

	// 检查 remainder 是否只包含空白字符
	// 如果是，返回 nil remainder
	if isAllWhitespace(remainder) {
		return msg, nil, nil
	}

	// 返回消息和剩余数据（用于粘包处理）
	return msg, remainder, nil
}

// isAllWhitespace 检查字节数组是否只包含空白字符
func isAllWhitespace(data []byte) bool {
	for _, b := range data {
		if b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
	}
	return true
}

// SetMessageBuilder 实现 MessageParser 接口
func (p *defaultMessageParser) SetMessageBuilder(builder MessageBuilder) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.builder = builder
}

// SetDefaultProtocol 实现 MessageParser 接口
func (p *defaultMessageParser) SetDefaultProtocol(protoType ProtocolType) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.defaultProto = protoType
}

// createReplyFunc 创建回复函数
func (p *defaultMessageParser) createReplyFunc(conn net.Conn, proto protocol.Protocol, req *protocol.Request) ReplyFunc {
	return func(ctx context.Context, messageID string, response any) error {
		slog.Debug("called with response", "response", response)
		
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			slog.Debug("context cancelled", "error", ctx.Err())
			return ctx.Err()
		default:
		}

		// 转换响应为 protocol.Response
		resp := &protocol.Response{}
		switch r := response.(type) {
		case *protocol.Response:
			resp = r
		case *protocol.SearchResults:
			// 从 SearchResults 转换
			resp.Count = r.Count
			resp.Total = r.Total
			resp.Paths = make([]string, len(r.Results))
			for i, result := range r.Results {
				resp.Paths[i] = result.Path
			}
		case map[string]any:
			// 从 map 转换 - 支持任意字段
			// 先尝试已知字段
			if count, ok := r["count"].(int); ok {
				resp.Count = count
			}
			if total, ok := r["total"].(int); ok {
				resp.Total = total
			}
			if paths, ok := r["paths"].([]string); ok {
				resp.Paths = paths
			}
			if errMsg, ok := r["error"].(string); ok {
				resp.Error = errMsg
			}
			
			// 将完整的 map 存储到 Result 字段（用于 status、get-config 等命令）
			// 这样可以保留所有字段
			resp.Result = r
		default:
			// 其他类型，尝试作为 Result 字段
			resp.Result = response
		}

		// 设置 ID
		if req.ID != nil {
			resp.ID = req.ID
		}

		slog.Debug("sending response via protocol", "response", resp)

		// 使用通用的 ResponseWriter 发送响应
		writer := protocol.NewResponseWriter(conn, proto)
		if err := writer.WriteResponse(ctx, resp); err != nil {
			slog.Error("write response failed", "error", err)
			return fmt.Errorf("write response failed: %w", err)
		}

		slog.Debug("response sent successfully")
		return nil
	}
}

// defaultMessageBuilder MessageBuilder 的默认实现
type defaultMessageBuilder struct{}

// Build 实现 MessageBuilder 接口
func (b *defaultMessageBuilder) Build(rawData []byte, replyFunc ReplyFunc) (Message, error) {
	// 解析 raw data 为 protocol.Request
	var req protocol.Request
	if err := json.Unmarshal(rawData, &req); err != nil {
		return nil, fmt.Errorf("unmarshal request failed: %w", err)
	}

	// 提取 ID
	id := ""
	if req.ID != nil {
		switch v := req.ID.(type) {
		case string:
			id = v
		case int:
			id = fmt.Sprintf("%d", v)
		case int64:
			id = fmt.Sprintf("%d", v)
		case float64:
			id = fmt.Sprintf("%.0f", v)
		}
	}

	// 创建上下文
	ctx := context.Background()

	// 创建 Message 对象
	msg := NewMessage(id, req.Method, rawData, ctx, replyFunc, nil)

	return msg, nil
}
