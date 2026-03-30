package message

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ========== 测试辅助函数 ==========

// createTestMessage 创建测试消息
func createTestMessage(id, method string, payload []byte) Message {
	return NewMessage(id, method, payload, context.Background(), nil, nil)
}

// createTestMessageThatRecordsReply 创建能够记录回复的测试消息
func createTestMessageThatRecordsReply(id, method string, payload []byte) (Message, *map[string]any) {
	var recordedReply map[string]any
	replyFunc := func(ctx context.Context, messageID string, response any) error {
		if m, ok := response.(map[string]any); ok {
			recordedReply = m
		}
		return nil
	}
	return NewMessage(id, method, payload, context.Background(), replyFunc, nil), &recordedReply
}

// ========== MessageWorker 测试 ==========

func TestMessageWorker_New(t *testing.T) {
	worker := NewMessageWorker()
	if worker == nil {
		t.Fatal("NewMessageWorker() returned nil")
	}

	// 检查初始状态
	if worker.IsRunning() {
		t.Error("new worker should not be running")
	}
}

func TestMessageWorker_StartStop(t *testing.T) {
	worker := NewMessageWorker()

	// 启动
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if !worker.IsRunning() {
		t.Error("worker should be running after Start()")
	}

	// 重复启动应该报错
	if err := worker.Start(); err == nil {
		t.Error("Start() should fail when already running")
	}

	// 停止
	if err := worker.Stop(); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	if worker.IsRunning() {
		t.Error("worker should not be running after Stop()")
	}

	// 重复停止应该报错
	if err := worker.Stop(); err == nil {
		t.Error("Stop() should fail when not running")
	}
}

func TestMessageWorker_RegisterMethod(t *testing.T) {
	worker := NewMessageWorker()
	w := worker.(*defaultMessageWorker)

	// 注册方法
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		return "ok", nil
	})

	worker.RegisterMethod("test", handler)

	// 检查是否注册成功
	if !w.HasMethod("test") {
		t.Error("method 'test' should be registered")
	}

	methods := w.GetRegisteredMethods()
	if len(methods) != 1 || methods[0] != "test" {
		t.Errorf("GetRegisteredMethods() = %v, want ['test']", methods)
	}

	// 注销方法
	worker.UnregisterMethod("test")

	if w.HasMethod("test") {
		t.Error("method 'test' should be unregistered")
	}

	methods = w.GetRegisteredMethods()
	if len(methods) != 0 {
		t.Errorf("GetRegisteredMethods() = %v, want []", methods)
	}
}

func TestMessageWorker_Handle_AsyncQueue(t *testing.T) {
	worker := NewMessageWorker()

	// 启动 worker
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer worker.Stop()

	// 注册测试方法
	var mu sync.Mutex
	var receivedPayload []byte
	var processed bool
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		receivedPayload = msg.Payload()
		processed = true
		return map[string]any{
			"result": "success",
			"echo":   string(msg.Payload()),
		}, nil
	})
	worker.RegisterMethod("echo", handler)

	// 创建测试消息
	payload := []byte("test payload")
	msg := createTestMessage("1", "echo", payload)

	// Handle 现在是异步的，将消息加入队列
	err := worker.Handle(msg)
	if err != nil {
		t.Fatalf("Handle() failed: %v", err)
	}

	// 等待消息被处理
	time.Sleep(100 * time.Millisecond)

	// 验证收到的 payload
	mu.Lock()
	if !processed {
		t.Error("message was not processed")
	}
	if string(receivedPayload) != "test payload" {
		t.Errorf("received payload = %s, want 'test payload'", receivedPayload)
	}
	mu.Unlock()
}

func TestMessageWorker_HandleSync(t *testing.T) {
	worker := NewMessageWorker()

	// 启动 worker
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer worker.Stop()

	// 注册测试方法
	var receivedPayload []byte
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		receivedPayload = msg.Payload()
		return map[string]any{
			"result": "success",
			"echo":   string(msg.Payload()),
		}, nil
	})
	worker.RegisterMethod("echo", handler)

	// 创建测试消息
	payload := []byte("test payload")
	msg := createTestMessage("1", "echo", payload)

	// HandleSync 是同步的，会阻塞等待处理完成
	err := worker.HandleSync(msg)
	if err != nil {
		t.Fatalf("HandleSync() failed: %v", err)
	}

	// 验证收到的 payload（不需要等待）
	if string(receivedPayload) != "test payload" {
		t.Errorf("received payload = %s, want 'test payload'", receivedPayload)
	}
}

func TestMessageWorker_HandleUnknownMethod(t *testing.T) {
	worker := NewMessageWorker()

	// 启动 worker
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer worker.Stop()

	// 创建测试消息（未注册的方法）
	msg, recordedReply := createTestMessageThatRecordsReply("1", "unknown", nil)

	// 使用 HandleSync 以便同步检查结果
	err := worker.HandleSync(msg)
	// Reply() 会成功发送错误响应，所以这里不会返回错误
	if err != nil {
		t.Logf("HandleSync() returned error: %v", err)
	}

	// 验证回复内容包含错误信息
	if *recordedReply == nil {
		t.Fatal("Reply was not called")
	}
	if errMsg, ok := (*recordedReply)["error"].(string); !ok || errMsg == "" {
		t.Errorf("reply error = %v, want non-empty error message", (*recordedReply)["error"])
	}
}

func TestMessageWorker_HandleNotRunning(t *testing.T) {
	worker := NewMessageWorker()

	// 不启动 worker，直接处理消息
	worker.RegisterMethod("test", MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		return "ok", nil
	}))

	msg := createTestMessage("1", "test", nil)

	// 处理消息应该失败
	err := worker.Handle(msg)
	if err == nil {
		t.Error("Handle() should fail when worker is not running")
	}
	if err != ErrWorkerNotRunning {
		t.Errorf("Handle() error = %v, want ErrWorkerNotRunning", err)
	}

	// HandleSync 也应该失败
	err = worker.HandleSync(msg)
	if err == nil {
		t.Error("HandleSync() should fail when worker is not running")
	}
	if err != ErrWorkerNotRunning {
		t.Errorf("HandleSync() error = %v, want ErrWorkerNotRunning", err)
	}
}

func TestMessageWorker_QueueFull(t *testing.T) {
	// 创建一个小队列的 worker
	worker := NewMessageWorkerWithOptions(2, 1)

	// 启动 worker
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer worker.Stop()

	// 注册一个慢处理器
	var mu sync.Mutex
	callCount := 0
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		time.Sleep(200 * time.Millisecond) // 慢处理
		return "ok", nil
	})
	worker.RegisterMethod("slow", handler)

	// 快速发送多个消息，填满队列
	const numMessages = 10
	var accepted, rejected int

	for i := 0; i < numMessages; i++ {
		msg := createTestMessage(string(rune('a'+i)), "slow", nil)
		err := worker.Handle(msg)
		if err == nil {
			accepted++
		} else if err == ErrQueueFull {
			rejected++
		}
	}

	// 应该有一些消息被接受，一些被拒绝
	if accepted == 0 {
		t.Error("at least some messages should be accepted")
	}
	if rejected == 0 {
		t.Error("at least some messages should be rejected (queue full)")
	}

	t.Logf("accepted: %d, rejected: %d", accepted, rejected)

	// 等待所有接受的消息处理完成
	time.Sleep(500 * time.Millisecond)

	mu.Lock()
	if callCount != accepted {
		t.Errorf("callCount = %d, want %d", callCount, accepted)
	}
	mu.Unlock()
}

func TestMessageWorker_GracefulShutdown(t *testing.T) {
	worker := NewMessageWorkerWithOptions(10, 2)

	// 启动 worker
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// 注册一个处理器
	var mu sync.Mutex
	callCount := 0
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		time.Sleep(50 * time.Millisecond) // 模拟处理时间
		return "ok", nil
	})
	worker.RegisterMethod("test", handler)

	// 发送一些消息
	for i := 0; i < 5; i++ {
		msg := createTestMessage(string(rune('a'+i)), "test", nil)
		worker.Handle(msg)
	}

	// 立即停止（应该等待正在处理的消息完成）
	start := time.Now()
	err := worker.Stop()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	t.Logf("Stop() took %v", elapsed)

	// 验证所有消息都被处理了
	mu.Lock()
	if callCount != 5 {
		t.Errorf("callCount = %d, want 5", callCount)
	}
	mu.Unlock()

	// 验证停止后不能再发送消息
	msg := createTestMessage("after-stop", "test", nil)
	err = worker.Handle(msg)
	if err != ErrWorkerNotRunning {
		t.Errorf("Handle() after stop should return ErrWorkerNotRunning, got %v", err)
	}
}

func TestMessageWorker_ConcurrentHandle(t *testing.T) {
	worker := NewMessageWorkerWithOptions(100, 4)

	// 启动 worker
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer worker.Stop()

	// 注册测试方法
	var mu sync.Mutex
	callCount := 0
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		mu.Lock()
		callCount++
		mu.Unlock()
		time.Sleep(10 * time.Millisecond) // 模拟耗时操作
		return "ok", nil
	})
	worker.RegisterMethod("concurrent", handler)

	// 并发发送多个消息
	const numMessages = 50
	var wg sync.WaitGroup
	var accepted int
	var mu2 sync.Mutex

	for i := 0; i < numMessages; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			msg := createTestMessage(string(rune(idx)), "concurrent", nil)
			err := worker.Handle(msg)
			if err == nil {
				mu2.Lock()
				accepted++
				mu2.Unlock()
			}
		}(i)
	}

	wg.Wait()

	// 等待所有消息处理完成
	time.Sleep(200 * time.Millisecond)

	// 验证调用次数
	mu.Lock()
	if callCount != accepted {
		t.Errorf("callCount = %d, want %d", callCount, accepted)
	}
	mu.Unlock()

	t.Logf("sent %d messages, accepted %d, processed %d", numMessages, accepted, callCount)
}

func TestMessageWorker_HandlerReturnsError(t *testing.T) {
	worker := NewMessageWorker()

	// 启动 worker
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer worker.Stop()

	// 注册会返回错误的处理器
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		return nil, errors.New("handler error")
	})
	worker.RegisterMethod("error", handler)

	// 创建测试消息
	msg, recordedReply := createTestMessageThatRecordsReply("1", "error", nil)

	// 使用 HandleSync 同步处理
	err := worker.HandleSync(msg)
	// Reply() 会成功发送错误响应，所以这里不会返回错误
	if err != nil {
		t.Logf("HandleSync() returned error: %v", err)
	}

	// 验证回复内容包含错误信息
	if *recordedReply == nil {
		t.Fatal("Reply was not called")
	}
	if errMsg, ok := (*recordedReply)["error"].(string); !ok || errMsg != "handler error" {
		t.Errorf("reply error = %v, want 'handler error'", (*recordedReply)["error"])
	}
}

func TestMessageWorker_HandlerReturnsNil(t *testing.T) {
	worker := NewMessageWorker()

	// 启动 worker
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer worker.Stop()

	// 注册返回 nil 的处理器（表示已手动回复）
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		// 手动回复
		msg.Reply(map[string]any{"manual": true})
		return nil, nil // 返回 nil 表示已手动回复
	})
	worker.RegisterMethod("async-reply", handler)

	// 创建测试消息
	msg := createTestMessage("1", "async-reply", nil)

	// 处理消息（应该成功，因为返回 nil）
	err := worker.HandleSync(msg)
	if err != nil {
		t.Fatalf("HandleSync() failed: %v", err)
	}
}

func TestMessageWorker_ConcurrentRegister(t *testing.T) {
	worker := NewMessageWorker()
	w := worker.(*defaultMessageWorker)

	// 并发注册方法
	const numMethods = 100
	var wg sync.WaitGroup

	for i := 0; i < numMethods; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			method := string(rune('a' + idx%26))
			handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
				return "ok", nil
			})
			worker.RegisterMethod(method, handler)
		}(i)
	}

	wg.Wait()

	// 验证方法数量
	methods := w.GetRegisteredMethods()
	if len(methods) == 0 {
		t.Error("at least some methods should be registered")
	}
}

func TestMessageWorker_MethodHandlerFunc(t *testing.T) {
	// 测试 MethodHandlerFunc 类型
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		return "test result", nil
	})

	// 验证它实现了 MethodHandler 接口
	var _ MethodHandler = handler

	// 调用它
	msg := createTestMessage("1", "test", nil)
	result, err := handler.Handle(context.Background(), msg)
	if err != nil {
		t.Fatalf("Handle() failed: %v", err)
	}

	if result != "test result" {
		t.Errorf("result = %v, want 'test result'", result)
	}
}

func TestMessageWorker_QueueMetrics(t *testing.T) {
	worker := NewMessageWorkerWithOptions(10, 1)
	w := worker.(*defaultMessageWorker)

	// 启动前队列长度应该是 0
	if w.GetQueueLength() != 0 {
		t.Errorf("queue length before start = %d, want 0", w.GetQueueLength())
	}

	// 启动
	if err := worker.Start(); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer worker.Stop()

	// 检查队列容量
	if w.GetQueueCapacity() != 10 {
		t.Errorf("queue capacity = %d, want 10", w.GetQueueCapacity())
	}

	// 注册一个慢处理器
	handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		time.Sleep(100 * time.Millisecond)
		return "ok", nil
	})
	worker.RegisterMethod("slow", handler)

	// 发送多个消息
	for i := 0; i < 5; i++ {
		msg := createTestMessage(string(rune('a'+i)), "slow", nil)
		worker.Handle(msg)
	}

	// 队列应该有一些消息
	time.Sleep(10 * time.Millisecond) // 给一点时间让消息入队
	length := w.GetQueueLength()
	if length == 0 {
		t.Log("queue length is 0, messages may have been processed quickly")
	}
	t.Logf("queue length: %d", length)
}

// ========== Benchmark 测试 ==========

func BenchmarkMessageWorker_Handle(b *testing.B) {
	worker := NewMessageWorkerWithOptions(1000, 4)
	worker.Start()
	defer worker.Stop()

	worker.RegisterMethod("bench", MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		return "ok", nil
	}))

	msg := createTestMessage("1", "bench", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		worker.Handle(msg)
	}
}

func BenchmarkMessageWorker_HandleSync(b *testing.B) {
	worker := NewMessageWorker()
	worker.Start()
	defer worker.Stop()

	worker.RegisterMethod("bench", MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		return "ok", nil
	}))

	msg := createTestMessage("1", "bench", nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		worker.HandleSync(msg)
	}
}

func BenchmarkMessageWorker_RegisterMethod(b *testing.B) {
	worker := NewMessageWorker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler := MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
			return "ok", nil
		})
		worker.RegisterMethod(string(rune(i)), handler)
	}
}

func BenchmarkMessageWorker_ConcurrentHandle(b *testing.B) {
	worker := NewMessageWorkerWithOptions(10000, 8)
	worker.Start()
	defer worker.Stop()

	worker.RegisterMethod("bench", MethodHandlerFunc(func(ctx context.Context, msg Message) (any, error) {
		return "ok", nil
	}))

	b.RunParallel(func(pb *testing.PB) {
		msg := createTestMessage("1", "bench", nil)
		for pb.Next() {
			worker.Handle(msg)
		}
	})
}
