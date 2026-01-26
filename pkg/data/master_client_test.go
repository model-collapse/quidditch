package data

import (
	"context"
	"net"
	"testing"
	"time"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// mockMasterServer implements a mock MasterServiceServer for testing
type mockMasterServer struct {
	pb.UnimplementedMasterServiceServer
	registerFunc    func(*pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error)
	heartbeatFunc   func(*pb.NodeHeartbeatRequest) (*pb.NodeHeartbeatResponse, error)
	unregisterFunc  func(*pb.UnregisterNodeRequest) (*pb.UnregisterNodeResponse, error)
	getStateFunc    func(*pb.GetClusterStateRequest) (*pb.ClusterStateResponse, error)
	getIndexFunc    func(*pb.GetIndexMetadataRequest) (*pb.IndexMetadataResponse, error)
}

func (m *mockMasterServer) RegisterNode(ctx context.Context, req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
	if m.registerFunc != nil {
		return m.registerFunc(req)
	}
	return &pb.RegisterNodeResponse{
		Acknowledged:   true,
		ClusterVersion: 1,
	}, nil
}

func (m *mockMasterServer) NodeHeartbeat(ctx context.Context, req *pb.NodeHeartbeatRequest) (*pb.NodeHeartbeatResponse, error) {
	if m.heartbeatFunc != nil {
		return m.heartbeatFunc(req)
	}
	return &pb.NodeHeartbeatResponse{
		Acknowledged:   true,
		ClusterVersion: 1,
	}, nil
}

func (m *mockMasterServer) UnregisterNode(ctx context.Context, req *pb.UnregisterNodeRequest) (*pb.UnregisterNodeResponse, error) {
	if m.unregisterFunc != nil {
		return m.unregisterFunc(req)
	}
	return &pb.UnregisterNodeResponse{
		Acknowledged: true,
	}, nil
}

func (m *mockMasterServer) GetClusterState(ctx context.Context, req *pb.GetClusterStateRequest) (*pb.ClusterStateResponse, error) {
	if m.getStateFunc != nil {
		return m.getStateFunc(req)
	}
	return &pb.ClusterStateResponse{
		Version:     1,
		ClusterName: "test-cluster",
		ClusterUuid: "test-uuid",
		Status:      pb.ClusterStatus_CLUSTER_STATUS_GREEN,
	}, nil
}

func (m *mockMasterServer) GetIndexMetadata(ctx context.Context, req *pb.GetIndexMetadataRequest) (*pb.IndexMetadataResponse, error) {
	if m.getIndexFunc != nil {
		return m.getIndexFunc(req)
	}
	return &pb.IndexMetadataResponse{
		Metadata: &pb.IndexMetadata{
			IndexName: req.IndexName,
			IndexUuid: "test-index-uuid",
			Version:   1,
		},
	}, nil
}

// setupMockMasterServer creates a mock master server for testing
func setupMockMasterServer(t *testing.T, mock *mockMasterServer) (*grpc.Server, *bufconn.Listener) {
	buffer := 1024 * 1024
	listener := bufconn.Listen(buffer)

	server := grpc.NewServer()
	pb.RegisterMasterServiceServer(server, mock)

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Logf("Server exited with error: %v", err)
		}
	}()

	return server, listener
}

// bufDialer creates a dialer for bufconn
func bufDialer(listener *bufconn.Listener) func(context.Context, string) (net.Conn, error) {
	return func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.Dial()
	}
}

func TestNewMasterClient(t *testing.T) {
	logger := zap.NewNop()
	client := NewMasterClient("node-1", "localhost:9090", logger)

	assert.NotNil(t, client)
	assert.Equal(t, "node-1", client.nodeID)
	assert.Equal(t, "localhost:9090", client.masterAddr)
	assert.False(t, client.connected)
}

func TestMasterClient_Connect(t *testing.T) {
	mock := &mockMasterServer{}
	server, listener := setupMockMasterServer(t, mock)
	defer server.Stop()

	logger := zap.NewNop()
	client := NewMasterClient("node-1", "bufconn", logger)

	// Override dial function for testing
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(listener)),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second))
	require.NoError(t, err)

	client.conn = conn
	client.client = pb.NewMasterServiceClient(conn)
	client.connected = true

	assert.True(t, client.IsConnected())
}

func TestMasterClient_Register(t *testing.T) {
	tests := []struct {
		name          string
		registerFunc  func(*pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error)
		expectError   bool
		expectedCalls int
	}{
		{
			name: "successful registration",
			registerFunc: func(req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
				assert.Equal(t, "node-1", req.NodeId)
				assert.Equal(t, pb.NodeType_NODE_TYPE_DATA, req.NodeType)
				return &pb.RegisterNodeResponse{
					Acknowledged:   true,
					ClusterVersion: 1,
				}, nil
			},
			expectError:   false,
			expectedCalls: 1,
		},
		{
			name: "registration with leader redirect",
			registerFunc: func() func(*pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
				calls := 0
				return func(req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
					calls++
					if calls < 3 {
						return nil, status.Error(codes.FailedPrecondition, "not the leader")
					}
					return &pb.RegisterNodeResponse{
						Acknowledged:   true,
						ClusterVersion: 1,
					}, nil
				}
			}(),
			expectError:   false,
			expectedCalls: 3,
		},
		{
			name: "registration failure",
			registerFunc: func(req *pb.RegisterNodeRequest) (*pb.RegisterNodeResponse, error) {
				return nil, status.Error(codes.Internal, "registration failed")
			},
			expectError:   true,
			expectedCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockMasterServer{
				registerFunc: tt.registerFunc,
			}
			server, listener := setupMockMasterServer(t, mock)
			defer server.Stop()

			logger := zap.NewNop()
			client := NewMasterClient("node-1", "bufconn", logger)

			// Setup connection
			ctx := context.Background()
			conn, err := grpc.DialContext(ctx, "bufnet",
				grpc.WithContextDialer(bufDialer(listener)),
				grpc.WithInsecure(),
				grpc.WithBlock(),
				grpc.WithTimeout(5*time.Second))
			require.NoError(t, err)
			defer conn.Close()

			client.conn = conn
			client.client = pb.NewMasterServiceClient(conn)
			client.connected = true

			// Test registration
			attributes := &pb.NodeAttributes{
				StorageTier: "hot",
				MaxShards:   10,
				SimdEnabled: true,
				Version:     "1.0.0",
			}

			err = client.Register(ctx, "localhost", 9090, attributes)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestMasterClient_Heartbeat(t *testing.T) {
	heartbeatCount := 0
	mock := &mockMasterServer{
		heartbeatFunc: func(req *pb.NodeHeartbeatRequest) (*pb.NodeHeartbeatResponse, error) {
			heartbeatCount++
			assert.Equal(t, "node-1", req.NodeId)
			return &pb.NodeHeartbeatResponse{
				Acknowledged:   true,
				ClusterVersion: 1,
			}, nil
		},
	}

	server, listener := setupMockMasterServer(t, mock)
	defer server.Stop()

	logger := zap.NewNop()
	client := NewMasterClient("node-1", "bufconn", logger)

	// Setup connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(listener)),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second))
	require.NoError(t, err)
	defer conn.Close()

	client.conn = conn
	client.client = pb.NewMasterServiceClient(conn)
	client.connected = true

	// Send single heartbeat
	err = client.sendHeartbeat(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 1, heartbeatCount)
}

func TestMasterClient_StartStopHeartbeat(t *testing.T) {
	heartbeatCount := 0
	mock := &mockMasterServer{
		heartbeatFunc: func(req *pb.NodeHeartbeatRequest) (*pb.NodeHeartbeatResponse, error) {
			heartbeatCount++
			return &pb.NodeHeartbeatResponse{
				Acknowledged:   true,
				ClusterVersion: 1,
			}, nil
		},
	}

	server, listener := setupMockMasterServer(t, mock)
	defer server.Stop()

	logger := zap.NewNop()
	client := NewMasterClient("node-1", "bufconn", logger)

	// Setup connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(listener)),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second))
	require.NoError(t, err)
	defer conn.Close()

	client.conn = conn
	client.client = pb.NewMasterServiceClient(conn)
	client.connected = true

	// Start heartbeat with short interval
	client.StartHeartbeat(ctx, 100*time.Millisecond)

	// Wait for a few heartbeats
	time.Sleep(350 * time.Millisecond)

	// Stop heartbeat
	client.StopHeartbeat()

	// Verify heartbeats were sent
	assert.GreaterOrEqual(t, heartbeatCount, 2, "Expected at least 2 heartbeats")
}

func TestMasterClient_GetClusterState(t *testing.T) {
	mock := &mockMasterServer{
		getStateFunc: func(req *pb.GetClusterStateRequest) (*pb.ClusterStateResponse, error) {
			return &pb.ClusterStateResponse{
				Version:     1,
				ClusterName: "test-cluster",
				ClusterUuid: "test-uuid",
				Status:      pb.ClusterStatus_CLUSTER_STATUS_GREEN,
			}, nil
		},
	}

	server, listener := setupMockMasterServer(t, mock)
	defer server.Stop()

	logger := zap.NewNop()
	client := NewMasterClient("node-1", "bufconn", logger)

	// Setup connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(listener)),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second))
	require.NoError(t, err)
	defer conn.Close()

	client.conn = conn
	client.client = pb.NewMasterServiceClient(conn)
	client.connected = true

	// Test GetClusterState
	resp, err := client.GetClusterState(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "test-cluster", resp.ClusterName)
	assert.Equal(t, pb.ClusterStatus_CLUSTER_STATUS_GREEN, resp.Status)
}

func TestMasterClient_GetIndexMetadata(t *testing.T) {
	mock := &mockMasterServer{
		getIndexFunc: func(req *pb.GetIndexMetadataRequest) (*pb.IndexMetadataResponse, error) {
			assert.Equal(t, "test-index", req.IndexName)
			return &pb.IndexMetadataResponse{
				Metadata: &pb.IndexMetadata{
					IndexName: "test-index",
					IndexUuid: "test-index-uuid",
					Version:   1,
				},
			}, nil
		},
	}

	server, listener := setupMockMasterServer(t, mock)
	defer server.Stop()

	logger := zap.NewNop()
	client := NewMasterClient("node-1", "bufconn", logger)

	// Setup connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(listener)),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second))
	require.NoError(t, err)
	defer conn.Close()

	client.conn = conn
	client.client = pb.NewMasterServiceClient(conn)
	client.connected = true

	// Test GetIndexMetadata
	resp, err := client.GetIndexMetadata(ctx, "test-index")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "test-index", resp.Metadata.IndexName)
}

func TestMasterClient_Unregister(t *testing.T) {
	unregisterCalled := false
	mock := &mockMasterServer{
		unregisterFunc: func(req *pb.UnregisterNodeRequest) (*pb.UnregisterNodeResponse, error) {
			unregisterCalled = true
			assert.Equal(t, "node-1", req.NodeId)
			return &pb.UnregisterNodeResponse{
				Acknowledged: true,
			}, nil
		},
	}

	server, listener := setupMockMasterServer(t, mock)
	defer server.Stop()

	logger := zap.NewNop()
	client := NewMasterClient("node-1", "bufconn", logger)

	// Setup connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(listener)),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second))
	require.NoError(t, err)
	defer conn.Close()

	client.conn = conn
	client.client = pb.NewMasterServiceClient(conn)
	client.connected = true

	// Test Unregister
	err = client.Unregister(ctx)
	assert.NoError(t, err)
	assert.True(t, unregisterCalled)
}

func TestMasterClient_NotConnected(t *testing.T) {
	logger := zap.NewNop()
	client := NewMasterClient("node-1", "localhost:9090", logger)

	ctx := context.Background()

	// Test operations when not connected
	err := client.Register(ctx, "localhost", 9090, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	_, err = client.GetClusterState(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	_, err = client.GetIndexMetadata(ctx, "test-index")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")

	err = client.Unregister(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not connected")
}

func TestMasterClient_Disconnect(t *testing.T) {
	mock := &mockMasterServer{}
	server, listener := setupMockMasterServer(t, mock)
	defer server.Stop()

	logger := zap.NewNop()
	client := NewMasterClient("node-1", "bufconn", logger)

	// Setup connection
	ctx := context.Background()
	conn, err := grpc.DialContext(ctx, "bufnet",
		grpc.WithContextDialer(bufDialer(listener)),
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(5*time.Second))
	require.NoError(t, err)

	client.conn = conn
	client.client = pb.NewMasterServiceClient(conn)
	client.connected = true

	assert.True(t, client.IsConnected())

	// Disconnect
	err = client.Disconnect()
	assert.NoError(t, err)
	assert.False(t, client.IsConnected())

	// Disconnect again (should be no-op)
	err = client.Disconnect()
	assert.NoError(t, err)
}
