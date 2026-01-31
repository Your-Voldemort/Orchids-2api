package pool

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSPool manages a pool of WebSocket connections
type WSPool struct {
	connections chan *websocket.Conn
	factory     func() (*websocket.Conn, error)
	minIdle     int
	maxSize     int
	mu          sync.RWMutex
	closed      bool
}

// NewWSPool creates a new WebSocket connection pool
func NewWSPool(factory func() (*websocket.Conn, error), minIdle, maxSize int) *WSPool {
	pool := &WSPool{
		connections: make(chan *websocket.Conn, maxSize),
		factory:     factory,
		minIdle:     minIdle,
		maxSize:     maxSize,
	}

	// Pre-warm connections
	go pool.warmUp()
	go pool.maintainMinIdle()

	return pool
}

// Get retrieves a connection from the pool or creates a new one
func (p *WSPool) Get(ctx context.Context) (*websocket.Conn, error) {
	p.mu.RLock()
	if p.closed {
		p.mu.RUnlock()
		return nil, ErrPoolClosed
	}
	p.mu.RUnlock()

	select {
	case conn := <-p.connections:
		if p.isHealthy(conn) {
			return conn, nil
		}
		conn.Close()
		// Fall through to create new
	case <-time.After(100 * time.Millisecond):
		// No idle connection available, create new
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return p.factory()
}

// Put returns a connection to the pool
func (p *WSPool) Put(conn *websocket.Conn) {
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()

	if closed || conn == nil || !p.isHealthy(conn) {
		if conn != nil {
			conn.Close()
		}
		return
	}

	select {
	case p.connections <- conn:
		// Successfully returned to pool
	default:
		// Pool is full, close the connection
		conn.Close()
	}
}

// warmUp pre-creates minimum idle connections
func (p *WSPool) warmUp() {
	for i := 0; i < p.minIdle; i++ {
		conn, err := p.factory()
		if err != nil {
			continue
		}
		select {
		case p.connections <- conn:
		default:
			conn.Close()
		}
	}
}

// maintainMinIdle ensures minimum number of idle connections
func (p *WSPool) maintainMinIdle() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		p.mu.RLock()
		if p.closed {
			p.mu.RUnlock()
			return
		}
		p.mu.RUnlock()

		idle := len(p.connections)
		if idle < p.minIdle {
			needed := p.minIdle - idle
			for i := 0; i < needed; i++ {
				conn, err := p.factory()
				if err != nil {
					continue
				}
				select {
				case p.connections <- conn:
				default:
					conn.Close()
				}
			}
		}
	}
}

// isHealthy checks if a connection is still alive
func (p *WSPool) isHealthy(conn *websocket.Conn) bool {
	if conn == nil {
		return false
	}
	err := conn.WriteControl(
		websocket.PingMessage,
		[]byte{},
		time.Now().Add(1*time.Second),
	)
	return err == nil
}

// Close closes the pool and all connections
func (p *WSPool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	p.mu.Unlock()

	close(p.connections)
	for conn := range p.connections {
		conn.Close()
	}
}

// Stats returns pool statistics
func (p *WSPool) Stats() PoolStats {
	return PoolStats{
		Idle:    len(p.connections),
		MinIdle: p.minIdle,
		MaxSize: p.maxSize,
	}
}

// PoolStats contains pool statistics
type PoolStats struct {
	Idle    int
	MinIdle int
	MaxSize int
}

var ErrPoolClosed = fmt.Errorf("connection pool is closed")
