package zk

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"dahlia/internal/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	zkTestServer = "localhost:2181"
	testTimeout  = 10 * time.Second
)

func setupTestCoordinator(t *testing.T) (*zkCoordinator, func()) {
	log, err := logger.NewZapLoggerForDev()
	require.NoError(t, err, "failed to create logger")

	coord, err := NewZKCoordinator([]string{zkTestServer}, testTimeout, log)
	require.NoError(t, err, "failed to connect to zookeeper")

	// Clean up any existing test nodes from previous runs
		testPaths := []string{
			"/test",
			"/workflows",
		}

		for _, path := range testPaths {
		deleteRecursive(coord.(*zkCoordinator), path)
	}

	cleanup := func() {
		// Clean up test nodes recursively
		for _, path := range testPaths {
			deleteRecursive(coord.(*zkCoordinator), path)
		}

		coord.Close()
	}

	return coord.(*zkCoordinator), cleanup
}

// deleteRecursive deletes a node and all its children
func deleteRecursive(coord *zkCoordinator, path string) {
	// Get children
	children, _, err := coord.conn.Children(path)
	if err != nil {
		return // Node doesn't exist or error
	}

	// Delete all children first
	for _, child := range children {
		childPath := path + "/" + child
		deleteRecursive(coord, childPath)
	}

	// Delete the node itself
	coord.conn.Delete(path, -1)
}

// TestZKConnection tests basic ZooKeeper connection
func TestZKConnection(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	assert.NotNil(t, coord.conn, "connection should not be nil")
}

// TestCreateNode tests node creation
func TestCreateNode(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	tests := []struct {
		name    string
		path    string
		data    []byte
		wantErr bool
	}{
		{
			name:    "create simple node",
			path:    "/test/node1",
			data:    []byte("test data"),
			wantErr: false,
		},
		{
			name:    "create nested node",
			path:    "/test/nested/deep/node",
			data:    []byte("nested data"),
			wantErr: false,
		},
		{
			name:    "create node with empty data",
			path:    "/test/empty",
			data:    []byte{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := coord.CreateNode(tt.path, tt.data)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGetNode tests node retrieval
func TestGetNode(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	// Setup: Create a node
	testPath := "/test/getnode"
	testData := []byte("get test data")
	err := coord.CreateNode(testPath, testData)
	require.NoError(t, err)

	// Test: Get the node
	data, err := coord.GetNode(testPath)
	assert.NoError(t, err)
	assert.Equal(t, testData, data)

	// Test: Get non-existent node
	_, err = coord.GetNode("/test/nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "node not found")
}

// TestUpdateNode tests node updates
func TestUpdateNode(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	testPath := "/test/updatenode"
	initialData := []byte("initial data")
	updatedData := []byte("updated data")

	// Create initial node
	err := coord.CreateNode(testPath, initialData)
	require.NoError(t, err)

	// Verify initial data
	data, err := coord.GetNode(testPath)
	require.NoError(t, err)
	assert.Equal(t, initialData, data)

	// Update node
	err = coord.UpdateNode(testPath, updatedData)
	assert.NoError(t, err)

	// Verify updated data
	data, err = coord.GetNode(testPath)
	assert.NoError(t, err)
	assert.Equal(t, updatedData, data)
}

// TestUpdateNonExistentNode tests updating a node that doesn't exist
func TestUpdateNonExistentNode(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	testPath := "/test/newnode"
	testData := []byte("new data")

	// Update should create the node if it doesn't exist
	err := coord.UpdateNode(testPath, testData)
	assert.NoError(t, err)

	// Verify node was created
	data, err := coord.GetNode(testPath)
	assert.NoError(t, err)
	assert.Equal(t, testData, data)
}

// TestWatchNode tests node watching functionality
func TestWatchNode(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	testPath := "/test/watchnode"
	initialData := []byte("initial")

	// Create node
	err := coord.CreateNode(testPath, initialData)
	require.NoError(t, err)

	// Setup watch
	var wg sync.WaitGroup
	wg.Add(1)

	var receivedData []byte
	handler := func(data []byte) {
		receivedData = data
		wg.Done()
	}

	err = coord.WatchNode(testPath, handler)
	require.NoError(t, err)

	// Give watch time to setup
	time.Sleep(100 * time.Millisecond)

	// Update node to trigger watch
	updatedData := []byte("updated by watch test")
	err = coord.UpdateNode(testPath, updatedData)
	require.NoError(t, err)

	// Wait for handler to be called
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, updatedData, receivedData, "handler should receive updated data")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for watch handler")
	}
}

// TestWatchNodeMultipleUpdates tests watch with multiple updates
func TestWatchNodeMultipleUpdates(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	testPath := "/test/watchmultiple"
	err := coord.CreateNode(testPath, []byte("initial"))
	require.NoError(t, err)

	// Track all updates
	var mu sync.Mutex
	var updates []string
	expectedUpdates := 3

	var wg sync.WaitGroup
	wg.Add(expectedUpdates)

	handler := func(data []byte) {
		mu.Lock()
		updates = append(updates, string(data))
		mu.Unlock()
		wg.Done()
	}

	err = coord.WatchNode(testPath, handler)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Perform multiple updates
	for i := 1; i <= expectedUpdates; i++ {
		err = coord.UpdateNode(testPath, []byte(fmt.Sprintf("update-%d", i)))
		require.NoError(t, err)
		time.Sleep(200 * time.Millisecond) // Give time for watch to trigger
	}

	// Wait for all updates
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		mu.Lock()
		assert.Equal(t, expectedUpdates, len(updates), "should receive all updates")
		mu.Unlock()
	case <-time.After(10 * time.Second):
		t.Fatal("timeout waiting for all updates")
	}
}

// TestWorkflowVersionTracking tests the workflow version use case
func TestWorkflowVersionTracking(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	workflowID := "test-workflow-123"
	versionPath := fmt.Sprintf("/workflows/%s", workflowID)

	// Create initial version
	err := coord.CreateNode(versionPath, []byte("1"))
	require.NoError(t, err)

	// Read version
	data, err := coord.GetNode(versionPath)
	assert.NoError(t, err)
	assert.Equal(t, "1", string(data))

	// Update version
	err = coord.UpdateNode(versionPath, []byte("2"))
	assert.NoError(t, err)

	// Verify updated version
	data, err = coord.GetNode(versionPath)
	assert.NoError(t, err)
	assert.Equal(t, "2", string(data))
}

// TestWorkflowRefreshTrigger tests the refresh trigger use case
func TestWorkflowRefreshTrigger(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	refreshPath := "/workflows/refresh"

	// Create refresh trigger node
	timestamp := fmt.Sprintf("%d", time.Now().Unix())
	err := coord.CreateNode(refreshPath, []byte(timestamp))
	require.NoError(t, err)

	// Setup watch for refresh
	var wg sync.WaitGroup
	wg.Add(1)

	var triggered bool
	handler := func(data []byte) {
		t.Logf("Refresh triggered with timestamp: %s", string(data))
		triggered = true
		wg.Done()
	}

	err = coord.WatchNode(refreshPath, handler)
	require.NoError(t, err)

	time.Sleep(100 * time.Millisecond)

	// Trigger refresh by updating timestamp
	newTimestamp := fmt.Sprintf("%d", time.Now().Unix())
	err = coord.UpdateNode(refreshPath, []byte(newTimestamp))
	require.NoError(t, err)

	// Wait for trigger
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		assert.True(t, triggered, "refresh should be triggered")
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for refresh trigger")
	}
}

// TestParentPathCreation tests automatic parent path creation
func TestParentPathCreation(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	deepPath := "/test/level1/level2/level3/node"
	testData := []byte("deep node data")

	// Create deep node - should auto-create parents
	err := coord.CreateNode(deepPath, testData)
	assert.NoError(t, err)

	// Verify node exists and has correct data
	data, err := coord.GetNode(deepPath)
	assert.NoError(t, err)
	assert.Equal(t, testData, data)

	// Verify parent paths exist
	parentPaths := []string{
		"/test/level1",
		"/test/level1/level2",
		"/test/level1/level2/level3",
	}

	for _, path := range parentPaths {
		exists, _, err := coord.conn.Exists(path)
		assert.NoError(t, err)
		assert.True(t, exists, "parent path %s should exist", path)
	}
}

// TestConcurrentOperations tests concurrent access
func TestConcurrentOperations(t *testing.T) {
	coord, cleanup := setupTestCoordinator(t)
	defer cleanup()

	basePath := "/test/concurrent"
	err := coord.CreateNode(basePath, []byte{})
	require.NoError(t, err)

	numGoroutines := 10
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Concurrent creates
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			path := fmt.Sprintf("%s/node-%d", basePath, id)
			data := []byte(fmt.Sprintf("data-%d", id))
			err := coord.CreateNode(path, data)
			assert.NoError(t, err)
		}(i)
	}

	wg.Wait()

	// Verify all nodes were created
	for i := 0; i < numGoroutines; i++ {
		path := fmt.Sprintf("%s/node-%d", basePath, i)
		data, err := coord.GetNode(path)
		assert.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("data-%d", i), string(data))
	}
}

// TestClose tests coordinator cleanup
func TestClose(t *testing.T) {
	log, err := logger.NewZapLoggerForDev()
	require.NoError(t, err)

	coord, err := NewZKCoordinator([]string{zkTestServer}, testTimeout, log)
	require.NoError(t, err)

	// Close should not error
	err = coord.Close()
	assert.NoError(t, err)
}
