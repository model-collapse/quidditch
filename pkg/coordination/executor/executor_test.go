package executor

import (
	"context"
	"errors"
	"testing"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// MockDataNodeClient is a mock implementation of DataNodeClient
type MockDataNodeClient struct {
	mock.Mock
	nodeID string
}

func (m *MockDataNodeClient) Search(ctx context.Context, indexName string, shardID int32, query []byte, filterExpression []byte) (*pb.SearchResponse, error) {
	args := m.Called(ctx, indexName, shardID, query, filterExpression)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.SearchResponse), args.Error(1)
}

func (m *MockDataNodeClient) Count(ctx context.Context, indexName string, shardID int32, query []byte, filterExpression []byte) (*pb.CountResponse, error) {
	args := m.Called(ctx, indexName, shardID, query, filterExpression)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pb.CountResponse), args.Error(1)
}

func (m *MockDataNodeClient) IsConnected() bool {
	args := m.Called()
	return args.Bool(0)
}

func (m *MockDataNodeClient) Connect(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *MockDataNodeClient) NodeID() string {
	return m.nodeID
}

// MockMasterClient is a mock implementation of MasterClient
type MockMasterClient struct {
	mock.Mock
}

func (m *MockMasterClient) GetShardRouting(ctx context.Context, indexName string) (map[int32]*pb.ShardRouting, error) {
	args := m.Called(ctx, indexName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[int32]*pb.ShardRouting), args.Error(1)
}

// TestQueryExecutorBasic tests basic QueryExecutor functionality with mocks
func TestQueryExecutorBasic(t *testing.T) {
	logger := zap.NewNop()
	masterClient := new(MockMasterClient)
	executor := NewQueryExecutor(masterClient, logger)

	t.Run("RegisterDataNode", func(t *testing.T) {
		client := &MockDataNodeClient{nodeID: "node1"}
		executor.RegisterDataNode(client)

		// Verify node registered
		executor.mu.RLock()
		_, exists := executor.dataClients["node1"]
		executor.mu.RUnlock()

		assert.True(t, exists, "DataNode should be registered")
	})

	t.Run("UnregisterDataNode", func(t *testing.T) {
		executor.UnregisterDataNode("node1")

		// Verify node unregistered
		executor.mu.RLock()
		_, exists := executor.dataClients["node1"]
		executor.mu.RUnlock()

		assert.False(t, exists, "DataNode should be unregistered")
	})
}

// TestQueryExecutorSearchTwoShards tests distributed search across 2 shards
func TestQueryExecutorSearchTwoShards(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	// Setup mock master client
	masterClient := new(MockMasterClient)
	masterClient.On("GetShardRouting", ctx, "test-index").Return(
		map[int32]*pb.ShardRouting{
			0: {ShardId: 0, NodeId: "node1", State: pb.ShardState_SHARD_STATE_ACTIVE},
			1: {ShardId: 1, NodeId: "node2", State: pb.ShardState_SHARD_STATE_ACTIVE},
		},
		nil,
	)

	// Setup mock data node clients
	node1 := &MockDataNodeClient{nodeID: "node1"}
	node1.On("Search", ctx, "test-index", int32(0), mock.Anything, mock.Anything).Return(
		&pb.SearchResponse{
			TookMillis: 10,
			TotalHits:  50,
			MaxScore:   0.95,
			Hits: []*pb.SearchHit{
				{Id: "doc1", Score: 0.95, Source: []byte(`{"title": "Document 1"}`)},
				{Id: "doc2", Score: 0.90, Source: []byte(`{"title": "Document 2"}`)},
			},
		},
		nil,
	)

	node2 := &MockDataNodeClient{nodeID: "node2"}
	node2.On("Search", ctx, "test-index", int32(1), mock.Anything, mock.Anything).Return(
		&pb.SearchResponse{
			TookMillis: 12,
			TotalHits:  45,
			MaxScore:   0.98,
			Hits: []*pb.SearchHit{
				{Id: "doc3", Score: 0.98, Source: []byte(`{"title": "Document 3"}`)},
				{Id: "doc4", Score: 0.85, Source: []byte(`{"title": "Document 4"}`)},
			},
		},
		nil,
	)

	// Create executor and register nodes
	executor := NewQueryExecutor(masterClient, logger)
	executor.RegisterDataNode(node1)
	executor.RegisterDataNode(node2)

	// Execute search
	query := []byte(`{"match_all": {}}`)
	result, err := executor.Search(ctx, "test-index", query, nil, 0, 10)

	// Verify results
	require.NoError(t, err)
	assert.Equal(t, int64(95), result.TotalHits) // 50 + 45
	assert.Equal(t, 0.98, result.MaxScore)       // max(0.95, 0.98)
	assert.Len(t, result.Hits, 4)

	// Verify hits are sorted by score (descending)
	assert.Equal(t, "doc3", result.Hits[0].ID)
	assert.Equal(t, 0.98, result.Hits[0].Score)
	assert.Equal(t, "doc1", result.Hits[1].ID)
	assert.Equal(t, 0.95, result.Hits[1].Score)
	assert.Equal(t, "doc2", result.Hits[2].ID)
	assert.Equal(t, 0.90, result.Hits[2].Score)
	assert.Equal(t, "doc4", result.Hits[3].ID)
	assert.Equal(t, 0.85, result.Hits[3].Score)

	// Verify mocks were called
	masterClient.AssertExpectations(t)
	node1.AssertExpectations(t)
	node2.AssertExpectations(t)
}

// TestQueryExecutorSearchWithPagination tests global pagination
func TestQueryExecutorSearchWithPagination(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	// Setup mock master client
	masterClient := new(MockMasterClient)
	masterClient.On("GetShardRouting", ctx, "test-index").Return(
		map[int32]*pb.ShardRouting{
			0: {ShardId: 0, NodeId: "node1", State: pb.ShardState_SHARD_STATE_ACTIVE},
		},
		nil,
	)

	// Setup mock data node with 100 documents
	node1 := &MockDataNodeClient{nodeID: "node1"}
	hits := make([]*pb.SearchHit, 100)
	for i := 0; i < 100; i++ {
		hits[i] = &pb.SearchHit{
			Id:     string(rune('A' + i)),
			Score:  float64(100 - i), // Descending scores
			Source: []byte(`{}`),
		}
	}

	node1.On("Search", ctx, "test-index", int32(0), mock.Anything, mock.Anything).Return(
		&pb.SearchResponse{
			TookMillis: 5,
			TotalHits:  100,
			MaxScore:   100.0,
			Hits:       hits,
		},
		nil,
	)

	// Create executor and register node
	executor := NewQueryExecutor(masterClient, logger)
	executor.RegisterDataNode(node1)

	// Test pagination: from=10, size=5
	result, err := executor.Search(ctx, "test-index", []byte(`{"match_all": {}}`), nil, 10, 5)

	// Verify results
	require.NoError(t, err)
	assert.Equal(t, int64(100), result.TotalHits)
	assert.Len(t, result.Hits, 5) // Should return 5 documents

	// Verify correct slice (documents 10-14)
	for i := 0; i < 5; i++ {
		expectedID := string(rune('A' + 10 + i))
		assert.Equal(t, expectedID, result.Hits[i].ID)
	}

	masterClient.AssertExpectations(t)
	node1.AssertExpectations(t)
}

// TestQueryExecutorPartialShardFailure tests graceful degradation
func TestQueryExecutorPartialShardFailure(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	// Setup mock master client
	masterClient := new(MockMasterClient)
	masterClient.On("GetShardRouting", ctx, "test-index").Return(
		map[int32]*pb.ShardRouting{
			0: {ShardId: 0, NodeId: "node1", State: pb.ShardState_SHARD_STATE_ACTIVE},
			1: {ShardId: 1, NodeId: "node2", State: pb.ShardState_SHARD_STATE_ACTIVE},
			2: {ShardId: 2, NodeId: "node3", State: pb.ShardState_SHARD_STATE_ACTIVE},
		},
		nil,
	)

	// Setup mock data nodes
	node1 := &MockDataNodeClient{nodeID: "node1"}
	node1.On("Search", ctx, "test-index", int32(0), mock.Anything, mock.Anything).Return(
		&pb.SearchResponse{TotalHits: 30, Hits: []*pb.SearchHit{}},
		nil,
	)

	node2 := &MockDataNodeClient{nodeID: "node2"}
	// Node2 fails
	node2.On("Search", ctx, "test-index", int32(1), mock.Anything, mock.Anything).Return(
		(*pb.SearchResponse)(nil),
		errors.New("connection timeout"),
	)

	node3 := &MockDataNodeClient{nodeID: "node3"}
	node3.On("Search", ctx, "test-index", int32(2), mock.Anything, mock.Anything).Return(
		&pb.SearchResponse{TotalHits: 35, Hits: []*pb.SearchHit{}},
		nil,
	)

	// Create executor and register nodes
	executor := NewQueryExecutor(masterClient, logger)
	executor.RegisterDataNode(node1)
	executor.RegisterDataNode(node2)
	executor.RegisterDataNode(node3)

	// Execute search (should succeed with partial results)
	result, err := executor.Search(ctx, "test-index", []byte(`{"match_all": {}}`), nil, 0, 10)

	// Verify graceful degradation
	require.NoError(t, err, "Search should succeed despite partial shard failure")
	assert.Equal(t, int64(65), result.TotalHits) // 30 + 35 (node2 excluded)

	masterClient.AssertExpectations(t)
	node1.AssertExpectations(t)
	node2.AssertExpectations(t)
	node3.AssertExpectations(t)
}

// TestQueryExecutorNoDataNodes tests behavior with no data nodes
func TestQueryExecutorNoDataNodes(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	// Setup mock master client
	masterClient := new(MockMasterClient)
	masterClient.On("GetShardRouting", ctx, "test-index").Return(
		map[int32]*pb.ShardRouting{
			0: {ShardId: 0, NodeId: "node1", State: pb.ShardState_SHARD_STATE_ACTIVE},
		},
		nil,
	)

	// Create executor with no data nodes
	executor := NewQueryExecutor(masterClient, logger)

	// Execute search (should fail)
	_, err := executor.Search(ctx, "test-index", []byte(`{"match_all": {}}`), nil, 0, 10)

	// Verify error
	assert.Error(t, err, "Search should fail with no data nodes")
	assert.Contains(t, err.Error(), "no data node found", "Error should mention missing data node")

	masterClient.AssertExpectations(t)
}

// TestQueryExecutorMasterClientError tests master client failure
func TestQueryExecutorMasterClientError(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	// Setup mock master client that fails
	masterClient := new(MockMasterClient)
	masterClient.On("GetShardRouting", ctx, "test-index").Return(
		(map[int32]*pb.ShardRouting)(nil),
		errors.New("master unavailable"),
	)

	// Create executor
	executor := NewQueryExecutor(masterClient, logger)

	// Execute search (should fail)
	_, err := executor.Search(ctx, "test-index", []byte(`{"match_all": {}}`), nil, 0, 10)

	// Verify error
	assert.Error(t, err, "Search should fail when master is unavailable")
	assert.Contains(t, err.Error(), "failed to get shard routing", "Error should mention routing failure")

	masterClient.AssertExpectations(t)
}

// TestQueryExecutorHasDataNodeClient tests HasDataNodeClient method
func TestQueryExecutorHasDataNodeClient(t *testing.T) {
	logger := zap.NewNop()
	masterClient := new(MockMasterClient)
	executor := NewQueryExecutor(masterClient, logger)

	// Initially no nodes
	assert.False(t, executor.HasDataNodeClient("node1"))

	// Register node
	client := &MockDataNodeClient{nodeID: "node1"}
	executor.RegisterDataNode(client)

	// Should exist now
	assert.True(t, executor.HasDataNodeClient("node1"))

	// Unregister node
	executor.UnregisterDataNode("node1")

	// Should not exist
	assert.False(t, executor.HasDataNodeClient("node1"))
}
