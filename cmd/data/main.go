package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/quidditch/quidditch/pkg/data"
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
	Use:   "quidditch-data",
	Short: "Quidditch Data Node",
	Long: `Quidditch Data Node executes queries using the Diagon search engine core.
It manages shards, executes search queries, and handles document operations.`,
	RunE: run,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is /etc/quidditch/data.yaml)")
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
	cfg, err := config.LoadDataNodeConfig(cfgFile)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logger.Info("Starting Quidditch Data Node",
		zap.String("node_id", cfg.NodeID),
		zap.String("bind_addr", cfg.BindAddr),
		zap.Int("grpc_port", cfg.GRPCPort),
		zap.String("data_dir", cfg.DataDir),
		zap.String("master_addr", cfg.MasterAddr),
		zap.String("storage_tier", cfg.StorageTier),
		zap.Bool("simd_enabled", cfg.SIMDEnabled),
	)

	// Create data node
	dataNode, err := data.NewDataNode(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create data node", zap.Error(err))
	}

	// Start data node
	if err := dataNode.Start(ctx); err != nil {
		logger.Fatal("Failed to start data node", zap.Error(err))
	}

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Data node started successfully",
		zap.String("grpc_endpoint", fmt.Sprintf("%s:%d", cfg.BindAddr, cfg.GRPCPort)),
	)

	// Wait for shutdown signal
	<-sigCh
	logger.Info("Received shutdown signal, stopping data node...")

	// Graceful shutdown
	if err := dataNode.Stop(ctx); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
		return err
	}

	logger.Info("Data node stopped successfully")
	return nil
}
