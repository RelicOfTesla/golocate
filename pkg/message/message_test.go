// Package message provides message tests.
package message

import (
	"context"
	"errors"
	"sync"
	"testing"
)

// TestNewMessage 测试创建消息
func TestNewMessage(t *testing.T) {
	id := "test-123"
	method := "search"
	payload := []byte(`{"content":"test"}`)
	ctx := context.Background()
	replyFunc := func(ctx context.Context, messageID string, response any) error {
		return nil
	}
	metadata := map[string]any{
		"remote_addr": "127.0.0.1:8080",
	}

	msg := NewMessage(id, method, payload, ctx, replyFunc, metadata)

	if msg.ID() != id {
		t.Errorf("expected ID %s, got %s", id, msg.ID())
	}

	if msg.Method() != method {
		t.Errorf("expected Method %s, got %s", method, msg.Method())
	}

	if string(msg.Payload()) != string(payload) {
		t.Errorf("expected Payload %s, got %s", string(payload), string(msg.Payload()))
	}

	if msg.Context() != ctx {
		t.Errorf("expected Context to be the same")
	}
}

// TestMessageReply 测试同步回复
func TestMessageReply(t *testing.T) {
	var replyCalled bool
	var replyMu sync.Mutex
	replyFunc := func(ctx context.Context, messageID string, response any) error {
		replyMu.Lock()
		defer replyMu.Unlock()
		replyCalled = true
		return nil
	}

	msg := NewMessage("test-123", "search", []byte{}, context.Background(), replyFunc, nil)

	// 第一次回复应该成功
	err := msg.Reply(map[string]any{"status": "ok"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	replyMu.Lock()
	if !replyCalled {
		t.Errorf("expected reply function to be called")
	}
	replyMu.Unlock()

	// 第二次回复应该失败
	err = msg.Reply(map[string]any{"status": "ok"})
	if err == nil {
		t.Errorf("expected error when replying twice")
	}
}

// TestMessageReplyError 测试回复失败
func TestMessageReplyError(t *testing.T) {
	expectedErr := errors.New("reply failed")
	replyFunc := func(ctx context.Context, messageID string, response any) error {
		return expectedErr
	}

	msg := NewMessage("test-123", "search", []byte{}, context.Background(), replyFunc, nil)

	// 回复应该失败
	err := msg.Reply(map[string]any{"status": "ok"})
	if err == nil {
		t.Errorf("expected error, got nil")
	}

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
}

// TestMessageMultipleReplies 测试多次回复
func TestMessageMultipleReplies(t *testing.T) {
	var replyCallCount int
	var replyMu sync.Mutex
	replyFunc := func(ctx context.Context, messageID string, response any) error {
		replyMu.Lock()
		defer replyMu.Unlock()
		replyCallCount++
		return nil
	}

	msg := NewMessage("test-123", "search", []byte{}, context.Background(), replyFunc, nil)

	// 第一次回复成功
	err := msg.Reply(map[string]any{"status": "first"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// 第二次回复失败
	err = msg.Reply(map[string]any{"status": "second"})
	if err == nil {
		t.Errorf("expected error when replying twice")
	}

	// 第三次回复失败
	err = msg.Reply(map[string]any{"status": "third"})
	if err == nil {
		t.Errorf("expected error when replying three times")
	}

	// 检查 reply 只被调用一次
	replyMu.Lock()
	if replyCallCount != 1 {
		t.Errorf("expected reply function to be called once, got %d", replyCallCount)
	}
	replyMu.Unlock()
}

// TestMessageSetOnComplete 测试完成回调
func TestMessageSetOnComplete(t *testing.T) {
	var callbackCalled bool
	var callbackMu sync.Mutex
	replyFunc := func(ctx context.Context, messageID string, response any) error {
		return nil
	}

	msg := NewMessage("test-123", "search", []byte{}, context.Background(), replyFunc, nil)

	// 设置完成回调
	msg.SetOnComplete(func() {
		callbackMu.Lock()
		defer callbackMu.Unlock()
		callbackCalled = true
	})

	// 回复
	err := msg.Reply(map[string]any{"status": "ok"})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// 检查回调是否被调用
	callbackMu.Lock()
	if !callbackCalled {
		t.Errorf("expected callback to be called")
	}
	callbackMu.Unlock()
}
