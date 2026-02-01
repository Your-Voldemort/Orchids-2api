package handler

import (
	"context"
	"sync"
	"time"
)

// SessionManager 管理会话状态
type SessionManager struct {
	sessions sync.Map // map[string]*Session
	ttl      time.Duration
}

// Session 会话信息
type Session struct {
	ID        string
	Workdir   string
	CreatedAt time.Time
	UpdatedAt time.Time
	Data      map[string]interface{}
	mu        sync.RWMutex
}

// NewSessionManager 创建会话管理器
func NewSessionManager(ttl time.Duration) *SessionManager {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	sm := &SessionManager{ttl: ttl}

	// 启动清理协程
	go sm.cleanupLoop()

	return sm
}

// GetOrCreate 获取或创建会话
func (sm *SessionManager) GetOrCreate(sessionID string) *Session {
	if session, ok := sm.sessions.Load(sessionID); ok {
		s := session.(*Session)
		s.mu.Lock()
		s.UpdatedAt = time.Now()
		s.mu.Unlock()
		return s
	}

	now := time.Now()
	session := &Session{
		ID:        sessionID,
		CreatedAt: now,
		UpdatedAt: now,
		Data:      make(map[string]interface{}),
	}

	// 使用 LoadOrStore 避免竞态条件
	actual, _ := sm.sessions.LoadOrStore(sessionID, session)
	return actual.(*Session)
}

// Get 获取会话
func (sm *SessionManager) Get(sessionID string) (*Session, bool) {
	if session, ok := sm.sessions.Load(sessionID); ok {
		return session.(*Session), true
	}
	return nil, false
}

// Delete 删除会话
func (sm *SessionManager) Delete(sessionID string) {
	sm.sessions.Delete(sessionID)
}

// SetWorkdir 设置会话工作目录
func (s *Session) SetWorkdir(workdir string) {
	s.mu.Lock()
	s.Workdir = workdir
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
}

// GetWorkdir 获取会话工作目录
func (s *Session) GetWorkdir() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Workdir
}

// Set 设置会话数据
func (s *Session) Set(key string, value interface{}) {
	s.mu.Lock()
	s.Data[key] = value
	s.UpdatedAt = time.Now()
	s.mu.Unlock()
}

// Get 获取会话数据
func (s *Session) Get(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.Data[key]
	return v, ok
}

// GetString 获取字符串类型的会话数据
func (s *Session) GetString(key string) string {
	if v, ok := s.Get(key); ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

// cleanupLoop 定期清理过期会话
func (sm *SessionManager) cleanupLoop() {
	ticker := time.NewTicker(sm.ttl / 2)
	defer ticker.Stop()

	for range ticker.C {
		sm.cleanup()
	}
}

// cleanup 清理过期会话
func (sm *SessionManager) cleanup() {
	now := time.Now()
	sm.sessions.Range(func(key, value interface{}) bool {
		session := value.(*Session)
		session.mu.RLock()
		expired := now.Sub(session.UpdatedAt) > sm.ttl
		session.mu.RUnlock()

		if expired {
			sm.sessions.Delete(key)
		}
		return true
	})
}

// RequestContext 请求上下文，包含请求相关的所有信息
type RequestContext struct {
	TraceID       string
	SessionID     string
	Workdir       string
	Channel       string
	Model         string
	IsStream      bool
	StartTime     time.Time
	AccountID     int64
	AccountName   string
	InputTokens   int
	OutputTokens  int
}

// requestContextKey 是 context 中存储 RequestContext 的 key
type requestContextKey struct{}

// WithRequestContext 创建带有 RequestContext 的新 context
func WithRequestContext(ctx context.Context, rc *RequestContext) context.Context {
	return context.WithValue(ctx, requestContextKey{}, rc)
}

// GetRequestContext 从 context 获取 RequestContext
func GetRequestContext(ctx context.Context) *RequestContext {
	if ctx == nil {
		return nil
	}
	if rc, ok := ctx.Value(requestContextKey{}).(*RequestContext); ok {
		return rc
	}
	return nil
}

// NewRequestContext 创建新的请求上下文
func NewRequestContext() *RequestContext {
	return &RequestContext{
		StartTime: time.Now(),
	}
}

// Duration 返回请求持续时间
func (rc *RequestContext) Duration() time.Duration {
	return time.Since(rc.StartTime)
}

// TotalTokens 返回总 token 数
func (rc *RequestContext) TotalTokens() int {
	return rc.InputTokens + rc.OutputTokens
}
