package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/quidditch/quidditch/pkg/master"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	cfgFile string
	logger  *zap.Logger
)

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "quidditch-master",
	Short: "Quidditch Master Node",
	Long: `Quidditch Master Node manages cluster state, shard allocation,
and index metadata using Raft consensus.`,
	RunE: run,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is /etc/quidditch/master.yaml)")
}

func initConfig() {
	var err error
	logger, err = zap.NewProduction()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()
}

func run(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration
	cfg, err := config.LoadMasterConfig(cfgFile)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logger.Info("Starting Quidditch Master Node",
		zap.String("node_id", cfg.NodeID),
		zap.String("bind_addr", cfg.BindAddr),
		zap.Int("raft_port", cfg.RaftPort),
		zap.Int("grpc_port", cfg.GRPCPort),
	)

	// Create master node
	masterNode, err := master.NewMasterNode(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create master node", zap.Error(err))
	}

	// Start master node
	if err := masterNode.Start(ctx); err != nil {
		logger.Fatal("Failed to start master node", zap.Error(err))
	}

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Master node started successfully")

	// Wait for shutdown signal
	<-sigCh
	logger.Info("Received shutdown signal, stopping master node...")

	// Graceful shutdown
	if err := masterNode.Stop(ctx); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
		return err
	}

	logger.Info("Master node stopped successfully")
	return nil
}
