// Package message provides message parser implementation.
package message

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	// ========== 协议检测 ==========

	// DetectProtocol 从数据中检测协议类型
	//
	// 参数：
	//   - reader: 缓冲读取器
	//
	// 返回：
	//   - ProtocolType: 检测到的协议类型
	//   - error: 检测失败时返回错误
	//
	// 注意：
	//   - 此方法只检测，不消费数据
	//   - 如果无法确定协议，返回默认协议
	DetectProtocol(reader *bufio.Reader) (ProtocolType, error)

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

	// ParseMessages 批量解析消息（处理粘包）
	//
	// 参数：
	//   - conn: 网络连接
	//   - data: 原始数据（可能包含多个消息）
	//
	// 返回：
	//   - []Message: 解析的消息列表
	//   - []byte: 剩余不完整的数据
	//   - error: 解析失败时返回错误
	//
	// 使用场景：
	//   - 一次性读取大量数据
	//   - 处理多个粘在一起的消息
	ParseMessages(conn net.Conn, data []byte) ([]Message, []byte, error)

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

// DetectProtocol 实现 MessageParser 接口
func (p *defaultMessageParser) DetectProtocol(reader *bufio.Reader) (ProtocolType, error) {
	// 使用 protocol 包的检测函数
	protoType, err := protocol.DetectProtocol(reader)
	if err != nil {
		return p.defaultProto, err
	}
	return ProtocolType(protoType), nil
}

// ParseMessage 实现 MessageParser 接口
func (p *defaultMessageParser) ParseMessage(conn net.Conn, reader *bufio.Reader) (Message, []byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 1. 检测协议
	protoType, err := p.DetectProtocol(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("detect protocol failed: %w", err)
	}

	// 2. 获取协议实现
	proto := protocol.GetProtocol(protocol.ProtocolType(protoType))

	// 3. 解析请求（包含粘包处理）
	req, remainder, err := proto.ParseRequestWithRemainder(reader)
	if err != nil {
		return nil, remainder, fmt.Errorf("parse request failed: %w", err)
	}

	// 4. 创建回复函数
	replyFunc := p.createReplyFunc(conn, proto, req)

	// 6. 序列化请求数据作为 payload
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, remainder, fmt.Errorf("marshal request failed: %w", err)
	}

	// 7. 构建消息（如果没有 builder，使用默认构建器）
	builder := p.builder
	if builder == nil {
		builder = &defaultMessageBuilder{}
	}

	// 8. 构建 Message 对象
	msg, err := builder.Build(payload, replyFunc)
	if err != nil {
		return nil, remainder, fmt.Errorf("build message failed: %w", err)
	}

	// 9. 设置元数据
	if m, ok := msg.(*defaultMessage); ok {
		m.metadata["protocol"] = string(protoType)
		m.metadata["arrival_time"] = time.Now()
		if conn != nil {
			m.metadata["remote_addr"] = conn.RemoteAddr().String()
		}
	}

	return msg, remainder, nil
}

// ParseMessages 实现 MessageParser 接口
func (p *defaultMessageParser) ParseMessages(conn net.Conn, data []byte) ([]Message, []byte, error) {
	var messages []Message
	var remainder []byte

	// 创建 reader 从 data 读取
	reader := bufio.NewReader(bytes.NewReader(data))

	for {
		// 尝试解析一个消息
		msg, rem, err := p.ParseMessage(conn, reader)
		if err != nil {
			// 如果解析失败，返回已解析的消息和剩余数据
			if len(messages) > 0 {
				return messages, data, nil
			}
			return nil, data, err
		}

		messages = append(messages, msg)

		// 如果有剩余数据，继续处理
		if len(rem) > 0 {
			// 将剩余数据放回 reader
			reader = bufio.NewReader(bytes.NewReader(rem))
			data = rem
		} else {
			// 没有剩余数据，解析完成
			break
		}
	}

	return messages, remainder, nil
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
		log.Printf("[ReplyFunc] Called with response: %+v", response)
		
		// 检查上下文是否已取消
		select {
		case <-ctx.Done():
			log.Printf("[ReplyFunc] Context cancelled: %v", ctx.Err())
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

		log.Printf("[ReplyFunc] Sending response via protocol: %+v", resp)

		// 使用通用的 ResponseWriter 发送响应
		writer := protocol.NewResponseWriter(conn, proto)
		if err := writer.WriteResponse(ctx, resp); err != nil {
			log.Printf("[ReplyFunc] WriteResponse failed: %v", err)
			return fmt.Errorf("write response failed: %w", err)
		}

		log.Printf("[ReplyFunc] Response sent successfully")
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
