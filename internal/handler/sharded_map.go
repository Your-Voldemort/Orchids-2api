package handler

import (
	"sync"
)

// ShardCount 定义分片数量
const ShardCount = 16

// ShardedMap 是一个分片的并发安全 Map
// 通过将数据分散到多个分片来减少锁竞争
type ShardedMap[V any] struct {
	shards [ShardCount]struct {
		mu   sync.RWMutex
		data map[string]V
	}
}

// NewShardedMap 创建新的分片 Map
func NewShardedMap[V any]() *ShardedMap[V] {
	m := &ShardedMap[V]{}
	for i := 0; i < ShardCount; i++ {
		m.shards[i].data = make(map[string]V)
	}
	return m
}

// fnv1aHash 使用 FNV-1a 哈希算法计算字符串的哈希值
func fnv1aHash(key string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h ^= uint32(key[i])
		h *= 16777619
	}
	return h
}

// getShard 根据 key 获取对应分片索引
func (m *ShardedMap[V]) getShard(key string) int {
	return int(fnv1aHash(key) % ShardCount)
}

// Get 获取值
func (m *ShardedMap[V]) Get(key string) (V, bool) {
	idx := m.getShard(key)
	m.shards[idx].mu.RLock()
	val, ok := m.shards[idx].data[key]
	m.shards[idx].mu.RUnlock()
	return val, ok
}

// Set 设置值
func (m *ShardedMap[V]) Set(key string, value V) {
	idx := m.getShard(key)
	m.shards[idx].mu.Lock()
	m.shards[idx].data[key] = value
	m.shards[idx].mu.Unlock()
}

// Delete 删除值
func (m *ShardedMap[V]) Delete(key string) {
	idx := m.getShard(key)
	m.shards[idx].mu.Lock()
	delete(m.shards[idx].data, key)
	m.shards[idx].mu.Unlock()
}

// Range 遍历所有分片
// 如果 fn 返回 false，则停止遍历
func (m *ShardedMap[V]) Range(fn func(key string, value V) bool) {
	for i := 0; i < ShardCount; i++ {
		m.shards[i].mu.RLock()
		for k, v := range m.shards[i].data {
			if !fn(k, v) {
				m.shards[i].mu.RUnlock()
				return
			}
		}
		m.shards[i].mu.RUnlock()
	}
}
