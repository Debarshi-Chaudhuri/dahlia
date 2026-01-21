package coordinator

// Coordinator defines ZooKeeper operations for distributed coordination
type Coordinator interface {
	CreateNode(path string, data []byte) error
	GetNode(path string) ([]byte, error)
	UpdateNode(path string, data []byte) error
	WatchNode(path string, handler func([]byte)) error
	Close() error
}
