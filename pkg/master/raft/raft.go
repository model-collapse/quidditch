package raft

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/raft"
	raftboltdb "github.com/hashicorp/raft-boltdb/v2"
	"go.uber.org/zap"
)

const (
	retainSnapshotCount = 2
	raftTimeout         = 10 * time.Second
)

// RaftNode wraps the Hashicorp Raft library and provides cluster consensus
type RaftNode struct {
	raft         *raft.Raft
	fsm          *FSM
	transport    *raft.NetworkTransport
	logger       *zap.Logger
	config       *Config
	shutdownCh   chan struct{}
}

// Config holds Raft configuration
type Config struct {
	NodeID       string
	RaftAddr     string
	DataDir      string
	Bootstrap    bool
	Peers        []string
	Logger       *zap.Logger
}

// NewRaftNode creates a new Raft node
func NewRaftNode(cfg *Config, fsm *FSM) (*RaftNode, error) {
	if cfg.Logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	raftConfig := raft.DefaultConfig()
	raftConfig.LocalID = raft.ServerID(cfg.NodeID)
	raftConfig.SnapshotThreshold = 1024
	// Use hclog default logger for Raft
	raftConfig.Logger = hclog.Default()

	// Setup Raft communication transport
	addr, err := net.ResolveTCPAddr("tcp", cfg.RaftAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve raft addr: %w", err)
	}

	transport, err := raft.NewTCPTransport(cfg.RaftAddr, addr, 3, raftTimeout, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	// Setup Raft data directory
	raftDir := filepath.Join(cfg.DataDir, "raft")
	if err := os.MkdirAll(raftDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create raft dir: %w", err)
	}

	// Create the snapshot store
	snapshotStore, err := raft.NewFileSnapshotStore(raftDir, retainSnapshotCount, os.Stderr)
	if err != nil {
		return nil, fmt.Errorf("failed to create snapshot store: %w", err)
	}

	// Create the log store and stable store
	logStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "raft-log.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create log store: %w", err)
	}

	stableStore, err := raftboltdb.NewBoltStore(filepath.Join(raftDir, "raft-stable.db"))
	if err != nil {
		return nil, fmt.Errorf("failed to create stable store: %w", err)
	}

	// Instantiate the Raft system
	ra, err := raft.NewRaft(raftConfig, fsm, logStore, stableStore, snapshotStore, transport)
	if err != nil {
		return nil, fmt.Errorf("failed to create raft: %w", err)
	}

	node := &RaftNode{
		raft:       ra,
		fsm:        fsm,
		transport:  transport,
		logger:     cfg.Logger,
		config:     cfg,
		shutdownCh: make(chan struct{}),
	}

	// Bootstrap cluster if needed
	if cfg.Bootstrap {
		configuration := raft.Configuration{
			Servers: []raft.Server{
				{
					ID:      raft.ServerID(cfg.NodeID),
					Address: transport.LocalAddr(),
				},
			},
		}
		ra.BootstrapCluster(configuration)
		cfg.Logger.Info("Bootstrapped Raft cluster", zap.String("node_id", cfg.NodeID))
	}

	return node, nil
}

// Start starts the Raft node
func (r *RaftNode) Start(ctx context.Context) error {
	r.logger.Info("Starting Raft node",
		zap.String("node_id", r.config.NodeID),
		zap.String("addr", r.config.RaftAddr),
	)

	// If we have peers, try to join the cluster
	if len(r.config.Peers) > 0 && !r.config.Bootstrap {
		// TODO: Implement join logic
		r.logger.Info("Peers configured, will join cluster", zap.Strings("peers", r.config.Peers))
	}

	return nil
}

// Stop stops the Raft node
func (r *RaftNode) Stop(ctx context.Context) error {
	r.logger.Info("Stopping Raft node")
	close(r.shutdownCh)

	if err := r.raft.Shutdown().Error(); err != nil {
		return fmt.Errorf("failed to shutdown raft: %w", err)
	}

	return nil
}

// IsLeader returns true if this node is the Raft leader
func (r *RaftNode) IsLeader() bool {
	return r.raft.State() == raft.Leader
}

// Leader returns the current leader address
func (r *RaftNode) Leader() string {
	addr, _ := r.raft.LeaderWithID()
	return string(addr)
}

// Apply applies a command to the Raft log
func (r *RaftNode) Apply(cmd Command, timeout time.Duration) error {
	if !r.IsLeader() {
		return fmt.Errorf("not the leader")
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	future := r.raft.Apply(data, timeout)
	if err := future.Error(); err != nil {
		return fmt.Errorf("failed to apply command: %w", err)
	}

	return nil
}

// AddVoter adds a new voting member to the cluster
func (r *RaftNode) AddVoter(id, addr string, timeout time.Duration) error {
	if !r.IsLeader() {
		return fmt.Errorf("not the leader")
	}

	future := r.raft.AddVoter(raft.ServerID(id), raft.ServerAddress(addr), 0, timeout)
	return future.Error()
}

// RemoveServer removes a server from the cluster
func (r *RaftNode) RemoveServer(id string, timeout time.Duration) error {
	if !r.IsLeader() {
		return fmt.Errorf("not the leader")
	}

	future := r.raft.RemoveServer(raft.ServerID(id), 0, timeout)
	return future.Error()
}

// GetState returns the current FSM state
func (r *RaftNode) GetState() interface{} {
	return r.fsm.GetState()
}

// WaitForLeader blocks until a leader is elected
func (r *RaftNode) WaitForLeader(timeout time.Duration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-ticker.C:
			if r.Leader() != "" {
				return nil
			}
		case <-timer.C:
			return fmt.Errorf("timeout waiting for leader")
		}
	}
}

// zapRaftLogger wraps zap.Logger to implement hclog.Logger interface for Raft
type zapRaftLogger struct {
	logger *zap.Logger
}

func (z *zapRaftLogger) Log(level interface{}, msg string, args ...interface{}) {
	// Convert args to zap fields
	fields := make([]zap.Field, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		if i+1 < len(args) {
			key := fmt.Sprintf("%v", args[i])
			value := args[i+1]
			fields = append(fields, zap.Any(key, value))
		}
	}

	switch level {
	case "ERROR":
		z.logger.Error(msg, fields...)
	case "WARN":
		z.logger.Warn(msg, fields...)
	case "INFO":
		z.logger.Info(msg, fields...)
	default:
		z.logger.Debug(msg, fields...)
	}
}

func (z *zapRaftLogger) Trace(msg string, args ...interface{}) { z.Log("TRACE", msg, args...) }
func (z *zapRaftLogger) Debug(msg string, args ...interface{}) { z.Log("DEBUG", msg, args...) }
func (z *zapRaftLogger) Info(msg string, args ...interface{})  { z.Log("INFO", msg, args...) }
func (z *zapRaftLogger) Warn(msg string, args ...interface{})  { z.Log("WARN", msg, args...) }
func (z *zapRaftLogger) Error(msg string, args ...interface{}) { z.Log("ERROR", msg, args...) }
func (z *zapRaftLogger) IsTrace() bool                         { return false }
func (z *zapRaftLogger) IsDebug() bool                         { return false }
func (z *zapRaftLogger) IsInfo() bool                          { return true }
func (z *zapRaftLogger) IsWarn() bool                          { return true }
func (z *zapRaftLogger) IsError() bool                         { return true }
func (z *zapRaftLogger) With(args ...interface{}) interface{}  { return z }
func (z *zapRaftLogger) Name() string                          { return "raft" }
func (z *zapRaftLogger) Named(name string) interface{}         { return z }
func (z *zapRaftLogger) ResetNamed(name string) interface{}    { return z }
func (z *zapRaftLogger) SetLevel(level interface{})            {}
func (z *zapRaftLogger) GetLevel() hclog.Level                 { return hclog.Info }
func (z *zapRaftLogger) ImpliedArgs() []interface{}            { return nil }
func (z *zapRaftLogger) StandardLogger(opts ...interface{}) interface{} {
	return nil
}
func (z *zapRaftLogger) StandardWriter(opts ...interface{}) io.Writer {
	return os.Stderr
}
