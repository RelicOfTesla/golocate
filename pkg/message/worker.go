// Package message provides message worker implementation.
package message

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// 错误定义
var (
	ErrWorkerNotRunning = errors.New("worker is not running")
	ErrQueueFull        = errors.New("message queue is full")
	ErrWorkerRunning    = errors.New("worker is already running")
)

// MessageWorker 消息处理器接口
//
// 核心职责：
// - 处理消息的业务逻辑
// - 默认使用异步队列处理消息
// - 支持同步处理（可选）
// - 支持延迟处理
// - 不关心底层协议
//
// 设计变更（2026-03-30）：
// - Handle() 改为异步队列调用（默认行为）
// - 使用内部队列和后台 worker 实现异步处理
// - 支持优雅关闭：处理完队列中所有消息后再退出
type MessageWorker interface {
	// ========== 消息处理 ==========

	// Handle 将消息加入异步队列，立即返回
	//
	// 参数：
	//   - msg: 消息对象
	//
	// 返回：
	//   - error: 加入队列失败时返回错误（如队列满、worker 未启动）
	//
	// 工作流程：
	//   1. 将消息加入内部队列
	//   2. 后台 worker 从队列中取出消息并处理
	//   3. 根据 msg.Method() 分发到具体的处理方法
	//   4. 执行业务逻辑
	//   5. 调用 msg.Reply() 回复
	//
	// 注意：
	//   - 此方法是异步的，不会阻塞等待处理完成
	//   - 如果队列满，会返回 ErrQueueFull
	Handle(msg Message) error

	// HandleSync 同步处理消息，阻塞等待处理完成
	//
	// 参数：
	//   - msg: 消息对象
	//
	// 返回：
	//   - error: 处理失败时返回错误
	//
	// 使用场景：
	//   - 需要立即得到处理结果
	//   - 测试场景
	//   - 特殊的同步处理需求
	//

	// ========== 方法注册 ==========

	// RegisterMethod 注册方法处理器
	//
	// 参数：
	//   - method: 方法名
	//   - handler: 方法处理器
	//
	// 使用场景：
	//   - 动态添加新的方法处理器
	//   - 插件式扩展功能
	RegisterMethod(method string, handler MethodHandler)

	// UnregisterMethod 注销方法处理器
	//
	// 参数：
	//   - method: 方法名
	UnregisterMethod(method string)

	// ========== 生命周期 ==========

	// Start 启动 Worker
	//
	// 返回：
	//   - error: 启动失败时返回错误
	//
	// 工作流程：
	//   1. 初始化消息队列
	//   2. 启动后台 worker goroutine
	//   3. 标记为运行状态
	Start() error

	// Stop 停止 Worker
	//
	// 返回：
	//   - error: 停止失败时返回错误
	//
	// 工作流程：
	//   1. 关闭消息队列（不再接受新消息）
	//   2. 等待所有后台 worker 完成当前消息处理
	//   3. 标记为停止状态
	Stop() error

	// IsRunning 检查 Worker 是否正在运行
	IsRunning() bool
}

// MethodHandler 方法处理器接口
//
// 每个方法（search, status, get-config 等）对应一个 MethodHandler
type MethodHandler interface {
	// Handle 处理消息
	//
	// 参数：
	//   - ctx: 上下文
	//   - msg: 消息对象
	//
	// 返回：
	//   - any: 响应数据（可选，可以直接调用 msg.Reply()）
	//   - error: 处理失败时返回错误
	Handle(ctx context.Context, msg Message) (any, error)
}

// MethodHandlerFunc 方法处理器函数类型
//
// 用于简化 MethodHandler 的创建
type MethodHandlerFunc func(ctx context.Context, msg Message) (any, error)

// Handle 实现 MethodHandler 接口
func (f MethodHandlerFunc) Handle(ctx context.Context, msg Message) (any, error) {
	return f(ctx, msg)
}

// defaultMessageWorker MessageWorker 的默认实现
type defaultMessageWorker struct {
	// 方法处理器映射
	methods   map[string]MethodHandler
	methodsMu sync.RWMutex

	// 运行状态
	running   bool
	runningMu sync.Mutex

	// 消息队列
	queue     chan Message
	queueSize int

	// worker 配置
	workerCount int

	// 等待所有 worker 完成
	wg sync.WaitGroup
}

// NewMessageWorker 创建消息处理器
//
// 默认配置：
//   - 队列大小：100
//   - worker 数量：4
func NewMessageWorker() MessageWorker {
	return NewMessageWorkerWithOptions(100, 4)
}

// NewMessageWorkerWithOptions 创建消息处理器（自定义配置）
//
// 参数：
//   - queueSize: 消息队列大小
//   - workerCount: 后台 worker 数量
func NewMessageWorkerWithOptions(queueSize, workerCount int) MessageWorker {
	if queueSize <= 0 {
		queueSize = 100
	}
	if workerCount <= 0 {
		workerCount = 4
	}

	return &defaultMessageWorker{
		methods:     make(map[string]MethodHandler),
		queueSize:   queueSize,
		workerCount: workerCount,
	}
}

// Handle 实现 MessageWorker 接口
//
// 将消息加入异步队列，立即返回
func (w *defaultMessageWorker) Handle(msg Message) error {
	// 检查 Worker 是否正在运行
	if !w.IsRunning() {
		return ErrWorkerNotRunning
	}

	// 将消息加入队列
	select {
	case w.queue <- msg:
		return nil
	default:
		return ErrQueueFull
	}
}

// processMessage 处理单个消息
//
// 内部方法，用于实际处理消息
func (w *defaultMessageWorker) processMessage(msg Message) error {
	// 查找方法处理器
	w.methodsMu.RLock()
	handler, ok := w.methods[msg.Method()]
	w.methodsMu.RUnlock()

	if !ok {
		// 方法未注册，返回错误响应
		return msg.Reply(map[string]any{
			"error": fmt.Sprintf("unknown method: %s", msg.Method()),
		})
	}

	// 执行方法处理器
	response, err := handler.Handle(msg.Context(), msg)
	if err != nil {
		// 处理失败，返回错误响应
		return msg.Reply(map[string]any{
			"error": err.Error(),
		})
	}

	// 如果处理器返回了响应，发送它
	if response != nil {
		return msg.Reply(response)
	}

	// 如果处理器没有返回响应，说明它可能已经异步回复了
	return nil
}

// RegisterMethod 实现 MessageWorker 接口
func (w *defaultMessageWorker) RegisterMethod(method string, handler MethodHandler) {
	w.methodsMu.Lock()
	defer w.methodsMu.Unlock()
	w.methods[method] = handler
}

// UnregisterMethod 实现 MessageWorker 接口
func (w *defaultMessageWorker) UnregisterMethod(method string) {
	w.methodsMu.Lock()
	defer w.methodsMu.Unlock()
	delete(w.methods, method)
}

// Start 实现 MessageWorker 接口
func (w *defaultMessageWorker) Start() error {
	w.runningMu.Lock()
	defer w.runningMu.Unlock()

	if w.running {
		return ErrWorkerRunning
	}

	// 初始化队列
	w.queue = make(chan Message, w.queueSize)

	// 启动后台 worker
	for i := 0; i < w.workerCount; i++ {
		w.wg.Add(1)
		go w.worker(i)
	}

	// 标记为运行中
	w.running = true

	return nil
}

// Stop 实现 MessageWorker 接口
func (w *defaultMessageWorker) Stop() error {
	w.runningMu.Lock()
	defer w.runningMu.Unlock()

	if !w.running {
		return ErrWorkerNotRunning
	}

	// 关闭队列（不再接受新消息）
	// worker 会继续处理队列中的消息，直到队列空了才退出
	close(w.queue)

	// 等待所有 worker 完成
	w.wg.Wait()

	// 标记为已停止
	w.running = false

	return nil
}

// IsRunning 实现 MessageWorker 接口
func (w *defaultMessageWorker) IsRunning() bool {
	w.runningMu.Lock()
	defer w.runningMu.Unlock()
	return w.running
}

// worker 后台 worker goroutine
//
// 从队列中取出消息并处理
// 当队列关闭时，worker 会处理完所有剩余消息后退出
func (w *defaultMessageWorker) worker(id int) {
	defer w.wg.Done()

	// 使用 for range 从队列中读取消息
	// 当队列关闭且为空时，for range 会自动退出
	for msg := range w.queue {
		// 处理消息
		// 注意：这里忽略错误，因为错误已经通过 Reply() 发送
		_ = w.processMessage(msg)
	}
}

// ========== 辅助方法 ==========

// GetRegisteredMethods 获取已注册的方法列表（用于调试）
func (w *defaultMessageWorker) GetRegisteredMethods() []string {
	w.methodsMu.RLock()
	defer w.methodsMu.RUnlock()

	methods := make([]string, 0, len(w.methods))
	for method := range w.methods {
		methods = append(methods, method)
	}
	return methods
}

// HasMethod 检查方法是否已注册
func (w *defaultMessageWorker) HasMethod(method string) bool {
	w.methodsMu.RLock()
	defer w.methodsMu.RUnlock()
	_, ok := w.methods[method]
	return ok
}

// GetQueueLength 获取队列当前长度（用于监控）
func (w *defaultMessageWorker) GetQueueLength() int {
	if w.queue == nil {
		return 0
	}
	return len(w.queue)
}

// GetQueueCapacity 获取队列容量
func (w *defaultMessageWorker) GetQueueCapacity() int {
	if w.queue == nil {
		return 0
	}
	return cap(w.queue)
}
