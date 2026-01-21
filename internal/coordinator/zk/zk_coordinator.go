package zk

import (
	"fmt"
	"time"

	coordinator "dahlia/internal/coordinator/iface"
	"dahlia/internal/logger"

	"github.com/go-zookeeper/zk"
)

type zkCoordinator struct {
	conn   *zk.Conn
	logger logger.Logger
}

// NewZKCoordinator creates a new ZooKeeper coordinator
func NewZKCoordinator(servers []string, sessionTimeout time.Duration, log logger.Logger) (coordinator.Coordinator, error) {
	conn, _, err := zk.Connect(servers, sessionTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to zookeeper: %w", err)
	}

	log.Info("connected to zookeeper",
		logger.Any("servers", servers),
	)

	return &zkCoordinator{
		conn:   conn,
		logger: log.With(logger.String("component", "zk_coordinator")),
	}, nil
}

func (c *zkCoordinator) CreateNode(path string, data []byte) error {
	c.logger.Debug("creating zk node",
		logger.String("path", path),
	)

	// Create parent directories if they don't exist
	if err := c.ensureParentPath(path); err != nil {
		return err
	}

	_, err := c.conn.Create(path, data, 0, zk.WorldACL(zk.PermAll))
	if err != nil {
		if err == zk.ErrNodeExists {
			c.logger.Warn("node already exists",
				logger.String("path", path),
			)
			return nil
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

	data, _, err := c.conn.Get(path)
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

	// Get current version
	_, stat, err := c.conn.Get(path)
	if err != nil {
		if err == zk.ErrNoNode {
			// Node doesn't exist, create it
			return c.CreateNode(path, data)
		}
		return fmt.Errorf("failed to get node: %w", err)
	}

	_, err = c.conn.Set(path, data, stat.Version)
	if err != nil {
		return fmt.Errorf("failed to update node: %w", err)
	}

	c.logger.Info("updated zk node",
		logger.String("path", path),
	)

	return nil
}

func (c *zkCoordinator) WatchNode(path string, handler func([]byte)) error {
	c.logger.Info("setting up watch on zk node",
		logger.String("path", path),
	)

	// Start goroutine to continuously watch
	go func() {
		for {
			// Establish watch
			_, _, events, err := c.conn.GetW(path)
			if err != nil {
				c.logger.Error("failed to watch node",
					logger.String("path", path),
					logger.Error(err),
				)
				return
			}

			// Wait for event
			event, ok := <-events
			if !ok {
				c.logger.Info("watch channel closed",
					logger.String("path", path),
				)
				return
			}

			c.logger.Debug("received zk event",
				logger.String("path", event.Path),
				logger.String("type", event.Type.String()),
			)

			if event.Type == zk.EventNodeDataChanged {
				// Fetch updated data
				data, _, err := c.conn.Get(event.Path)
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

				// Loop will re-establish watch automatically
			}
		}
	}()

	return nil
}

func (c *zkCoordinator) Close() error {
	c.logger.Info("closing zookeeper connection")
	c.conn.Close()
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
