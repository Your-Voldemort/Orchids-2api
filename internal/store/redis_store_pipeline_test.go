package store

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestGetAccountsByIDsPipelined 测试 Pipeline 批量获取功能
func TestGetAccountsByIDsPipelined(t *testing.T) {
	// 创建 mock redis store
	store := &redisStore{
		client: redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		}),
		prefix: "test:",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 测试 Pipeline 连接（如果 Redis 不可用，跳过测试）
	if err := store.client.Ping(ctx).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}
	defer store.Close()

	// 清理测试数据
	defer func() {
		store.client.Del(ctx, "test:accounts:id:1", "test:accounts:id:2", "test:accounts:id:3")
	}()

	// 准备测试数据
	testAccounts := []struct {
		id   int64
		data string
	}{
		{1, `{"id":1,"name":"test1","enabled":true}`},
		{2, `{"id":2,"name":"test2","enabled":true}`},
		{3, `{"id":3,"name":"test3","enabled":false}`},
	}

	// 写入测试数据
	for _, acc := range testAccounts {
		key := store.accountsKey(acc.id)
		if err := store.client.Set(ctx, key, acc.data, 0).Err(); err != nil {
			t.Fatalf("Failed to set test data: %v", err)
		}
	}

	// 测试 Pipeline 批量获取
	keys := []string{
		store.accountsKey(1),
		store.accountsKey(2),
		store.accountsKey(3),
	}

	values, err := store.getAccountsByIDsPipelined(ctx, keys)
	if err != nil {
		t.Fatalf("getAccountsByIDsPipelined failed: %v", err)
	}

	if len(values) != 3 {
		t.Errorf("Expected 3 values, got %d", len(values))
	}

	// 验证返回的数据
	for i, val := range values {
		if val == nil {
			t.Errorf("Value at index %d is nil", i)
			continue
		}
		strVal, ok := val.(string)
		if !ok {
			t.Errorf("Value at index %d is not a string", i)
			continue
		}
		if strVal == "" {
			t.Errorf("Value at index %d is empty", i)
		}
	}
}

// TestGetAccountsByIDsPipelinedFallback 测试 Pipeline 失败时的回退逻辑
func TestGetAccountsByIDsPipelinedFallback(t *testing.T) {
	// 创建一个无效的 Redis 连接来模拟失败
	store := &redisStore{
		client: redis.NewClient(&redis.Options{
			Addr: "localhost:9999", // 无效端口
		}),
		prefix: "test:",
	}
	defer store.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	keys := []string{"test:accounts:id:1"}

	// Pipeline 应该失败
	_, err := store.getAccountsByIDsPipelined(ctx, keys)
	if err == nil {
		t.Error("Expected error from invalid Redis connection, got nil")
	}
}

// TestGetAccountsByIDsEmptyKeys 测试空键列表
func TestGetAccountsByIDsEmptyKeys(t *testing.T) {
	store := &redisStore{
		client: redis.NewClient(&redis.Options{
			Addr: "localhost:6379",
		}),
		prefix: "test:",
	}
	defer store.Close()

	ctx := context.Background()

	values, err := store.getAccountsByIDsPipelined(ctx, []string{})
	if err != nil {
		t.Errorf("Expected no error for empty keys, got: %v", err)
	}
	if values != nil {
		t.Errorf("Expected nil values for empty keys, got: %v", values)
	}
}
