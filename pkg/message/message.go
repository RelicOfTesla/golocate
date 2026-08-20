// Package message provides message abstraction for golocate.
// This package defines the Message interface that hides protocol details from the business layer.
//
// Core Design:
// - Message is the only abstraction the business layer sees
// - Message encapsulates request information and reply capabilities
// - Protocol details are hidden inside Message implementation
package message

import (
	"context"
	"errors"
	"sync"
)

// Message 消息接口 - 业务层唯一看到的抽象
//
// 核心职责：
// - 提供请求信息的访问
// - 提供回复能力
// - 隐藏底层协议细节
type Message interface {
	// ========== 请求信息 ==========

	// ID 返回消息的唯一标识符（用于匹配请求和响应）
	ID() string

	// Method 返回请求的方法名
	Method() string

	// Payload 返回请求的负载（业务数据）
	Payload() []byte

	// Context 返回消息的上下文（用于取消、超时等）
	Context() context.Context

	// ========== 回复能力 ==========

	// Reply 回复消息
	//
	// 参数：
	//   - response: 响应数据
	//
	// 返回：
	//   - error: 发送失败时返回错误
	//
	// 注意：
	//   - 每个消息只能回复一次
	//   - 回复后消息被视为已完成
	Reply(response any) error

	// ========== 生命周期 ==========

	// SetOnComplete 设置消息处理完成时的回调
	//
	// 参数：
	//   - callback: 回调函数，在消息处理完成时调用
	//
	// 使用场景：
	//   - 连接管理：跟踪每个连接的消息处理状态
	//   - 资源清理：在消息处理完成时释放资源
	//
	// 注意：
	//   - 回调在消息处理完成后调用（Reply 成功后）
	//   - 回调只会在消息首次完成时调用一次
	SetOnComplete(callback func())
}

// ReplyFunc 回复函数类型
//
// 由 MessageParser 提供，MessageBuilder 用于构建消息时使用
type ReplyFunc func(ctx context.Context, messageID string, response any) error

// defaultMessage Message 的默认实现
type defaultMessage struct {
	// 请求信息
	id      string
	method  string
	payload []byte
	ctx     context.Context

	// 回复能力
	replyFunc ReplyFunc
	replied   bool
	replyMu   sync.Mutex

	// 元数据（内部使用）
	metadata map[string]any

	// 生命周期
	done       chan struct{}
	doneOnce   sync.Once
	onComplete func()     // 消息处理完成时的回调
	completeMu sync.Mutex // 保护 onComplete
}

// NewMessage 创建新的消息
func NewMessage(id, method string, payload []byte, ctx context.Context, replyFunc ReplyFunc, metadata map[string]any) Message {
	if metadata == nil {
		metadata = make(map[string]any)
	}

	return &defaultMessage{
		id:        id,
		method:    method,
		payload:   payload,
		ctx:       ctx,
		replyFunc: replyFunc,
		metadata:  metadata,
		done:      make(chan struct{}),
	}
}

// ID 实现 Message 接口
func (m *defaultMessage) ID() string {
	return m.id
}

// Method 实现 Message 接口
func (m *defaultMessage) Method() string {
	return m.method
}

// Payload 实现 Message 接口
func (m *defaultMessage) Payload() []byte {
	return m.payload
}

// Context 实现 Message 接口
func (m *defaultMessage) Context() context.Context {
	return m.ctx
}

// Reply 实现 Message 接口
func (m *defaultMessage) Reply(response any) error {
	m.replyMu.Lock()
	defer m.replyMu.Unlock()

	// 检查是否已回复
	if m.replied {
		return errors.New("message already replied")
	}

	// 调用回复函数
	if m.replyFunc != nil {
		if err := m.replyFunc(m.ctx, m.id, response); err != nil {
			return err
		}
	}

	// 标记为已回复
	m.replied = true

	// 标记为已完成
	m.markDone()

	return nil
}

// SetOnComplete 实现 Message 接口
func (m *defaultMessage) SetOnComplete(callback func()) {
	m.completeMu.Lock()
	defer m.completeMu.Unlock()
	m.onComplete = callback
}

// markDone 标记消息为已完成
func (m *defaultMessage) markDone() {
	m.doneOnce.Do(func() {
		// 标记为已完成
		close(m.done)

		// 调用完成回调（如果有）
		m.completeMu.Lock()
		callback := m.onComplete
		m.completeMu.Unlock()

		if callback != nil {
			callback()
		}
	})
}
