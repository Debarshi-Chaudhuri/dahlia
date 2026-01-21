package zk

import (
	"context"
	"fmt"
	"sync"
	"time"

	coordinator "dahlia/internal/coordinator/iface"
	"dahlia/internal/logger"

	"github.com/go-zookeeper/zk"
)

type zkCoordinator struct {
	servers        []string
	sessionTimeout time.Duration
	conn           *zk.Conn
	logger         logger.Logger
	mu             sync.RWMutex
	watches        map[string]func([]byte) // path -> handler
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewZKCoordinator creates a new ZooKeeper coordinator
func NewZKCoordinator(servers []string, sessionTimeout time.Duration, log logger.Logger) (coordinator.Coordinator, error) {
	ctx, cancel := context.WithCancel(context.Background())

	coord := &zkCoordinator{
		servers:        servers,
		sessionTimeout: sessionTimeout,
		logger:         log.With(logger.String("component", "zk_coordinator")),
		watches:        make(map[string]func([]byte)),
		ctx:            ctx,
		cancel:         cancel,
	}

	if err := coord.connect(); err != nil {
		cancel()
		return nil, err
	}

	// Start session monitoring
	go coord.monitorSession()

	return coord, nil
}

// connect establishes connection to ZooKeeper
func (c *zkCoordinator) connect() error {
	conn, events, err := zk.Connect(c.servers, c.sessionTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect to zookeeper: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()

	c.logger.Info("connected to zookeeper",
		logger.Any("servers", c.servers),
		logger.String("session_timeout", c.sessionTimeout.String()),
	)

	// Monitor connection events
	go c.monitorEvents(events)

	return nil
}

// monitorSession monitors ZK session state and handles reconnection
func (c *zkCoordinator) monitorSession() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(5 * time.Second):
			c.mu.RLock()
			if c.conn != nil {
				state := c.conn.State()
				if state != zk.StateHasSession {
					c.logger.Warn("zk session lost", logger.String("state", state.String()))
					c.mu.RUnlock()
					c.handleReconnect()
					continue
				}
			}
			c.mu.RUnlock()
		}
	}
}

// monitorEvents monitors ZK connection events
func (c *zkCoordinator) monitorEvents(events <-chan zk.Event) {
	for {
		select {
		case <-c.ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				c.logger.Warn("zk events channel closed")
				return
			}

			c.logger.Debug("zk session event",
				logger.String("type", event.Type.String()),
				logger.String("state", event.State.String()),
			)

			if event.State == zk.StateExpired || event.State == zk.StateDisconnected {
				c.logger.Warn("zk session expired or disconnected, attempting reconnect")
				go c.handleReconnect()
			}
		}
	}
}

// handleReconnect handles ZK reconnection and re-establishes watches
func (c *zkCoordinator) handleReconnect() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	// Retry connection with exponential backoff
	for attempt := 1; attempt <= 5; attempt++ {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.logger.Info("attempting zk reconnection", logger.Int("attempt", attempt))

		if err := c.connect(); err != nil {
			c.logger.Error("zk reconnection failed",
				logger.Int("attempt", attempt),
				logger.Error(err),
			)
			time.Sleep(time.Duration(attempt*attempt) * time.Second) // Exponential backoff
			continue
		}

		c.logger.Info("zk reconnection successful")

		// Re-establish watches
		c.reestablishWatches()
		return
	}

	c.logger.Error("failed to reconnect to zk after 5 attempts")
}

// reestablishWatches re-establishes all watches after reconnection
func (c *zkCoordinator) reestablishWatches() {
	for path, handler := range c.watches {
		c.logger.Info("re-establishing watch", logger.String("path", path))
		go c.startWatch(path, handler)
	}
}

func (c *zkCoordinator) UpsertNode(path string, data []byte) error {
	c.logger.Debug("creating zk node",
		logger.String("path", path),
	)

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("no zookeeper connection available")
	}

	// Create parent directories if they don't exist
	if err := c.ensureParentPath(path); err != nil {
		return err
	}

	_, err := conn.Create(path, data, 0, zk.WorldACL(zk.PermAll))
	if err != nil {
		if err == zk.ErrNodeExists {
			// Node exists, update it with new data
			return c.UpdateNode(path, data)
		}
		return fmt.Errorf("failed to create node: %w", err)
	}

	c.logger.Info("created zk node",
		logger.String("path", path),
	)

	return nil
}

func (c *zkCoordinator) GetNode(path string) ([]byte, error) {
	c.logger.Debug("getting zk node",
		logger.String("path", path),
	)

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("no zookeeper connection available")
	}

	data, _, err := conn.Get(path)
	if err != nil {
		if err == zk.ErrNoNode {
			return nil, fmt.Errorf("node not found: %s", path)
		}
		return nil, fmt.Errorf("failed to get node: %w", err)
	}

	return data, nil
}

func (c *zkCoordinator) UpdateNode(path string, data []byte) error {
	c.logger.Debug("updating zk node",
		logger.String("path", path),
	)

	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return fmt.Errorf("no zookeeper connection available")
	}

	// Get current version
	_, stat, err := conn.Get(path)
	if err != nil {
		if err == zk.ErrNoNode {
			// Node doesn't exist, create it
			return c.UpsertNode(path, data)
		}
		return fmt.Errorf("failed to get node: %w", err)
	}

	_, err = conn.Set(path, data, stat.Version)
	if err != nil {
		return fmt.Errorf("failed to update node: %w", err)
	}

	c.logger.Info("updated zk node",
		logger.String("path", path),
	)

	return nil
}

func (c *zkCoordinator) WatchNode(path string, handler func([]byte)) error {
	c.mu.Lock()
	c.watches[path] = handler
	c.mu.Unlock()

	c.logger.Info("setting up watch on zk node",
		logger.String("path", path),
	)

	go c.startWatch(path, handler)
	return nil
}

// startWatch starts watching a specific path
func (c *zkCoordinator) startWatch(path string, handler func([]byte)) {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			c.logger.Debug("no zk connection, waiting for reconnect", logger.String("path", path))
			time.Sleep(1 * time.Second)
			continue
		}

		// Establish watch
		_, _, events, err := conn.GetW(path)
		if err != nil {
			if err == zk.ErrNoNode {
				c.logger.Debug("watched node does not exist yet", logger.String("path", path))
				time.Sleep(5 * time.Second)
				continue
			}
			c.logger.Error("failed to watch node",
				logger.String("path", path),
				logger.Error(err),
			)
			time.Sleep(1 * time.Second)
			continue
		}

		// Wait for event
		select {
		case <-c.ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				c.logger.Debug("watch channel closed, will retry",
					logger.String("path", path),
				)
				time.Sleep(1 * time.Second)
				continue
			}

			c.logger.Debug("received zk event",
				logger.String("path", event.Path),
				logger.String("type", event.Type.String()),
			)

			if event.Type == zk.EventNodeDataChanged {
				// Fetch updated data
				data, _, err := conn.Get(event.Path)
				if err != nil {
					c.logger.Error("failed to get updated node data",
						logger.String("path", event.Path),
						logger.Error(err),
					)
					continue
				}

				c.logger.Info("node data changed, triggering handler",
					logger.String("path", event.Path),
				)

				// Call handler with new data
				handler(data)
			}
		}
	}
}

func (c *zkCoordinator) Close() error {
	c.logger.Info("closing zookeeper connection")

	// Cancel context to stop all goroutines
	c.cancel()

	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	return nil
}

// ensureParentPath creates parent directories if they don't exist
func (c *zkCoordinator) ensureParentPath(path string) error {
	if path == "/" {
		return nil
	}

	// Get parent path
	parentPath := path[:len(path)-len(path[findLastSlash(path):])]
	if parentPath == "" {
		parentPath = "/"
	}

	// Check if parent exists
	exists, _, err := c.conn.Exists(parentPath)
	if err != nil {
		return fmt.Errorf("failed to check parent path: %w", err)
	}

	if !exists {
		// Recursively create parent
		if err := c.ensureParentPath(parentPath); err != nil {
			return err
		}

		_, err := c.conn.Create(parentPath, []byte{}, 0, zk.WorldACL(zk.PermAll))
		if err != nil && err != zk.ErrNodeExists {
			return fmt.Errorf("failed to create parent path: %w", err)
		}
	}

	return nil
}

func findLastSlash(path string) int {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			return i
		}
	}
	return -1
}
