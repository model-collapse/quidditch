package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/quidditch/quidditch/pkg/common/config"
	"github.com/quidditch/quidditch/pkg/coordination"
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
	Use:   "quidditch-coordination",
	Short: "Quidditch Coordination Node",
	Long: `Quidditch Coordination Node handles query parsing, planning with Apache Calcite,
Python pipeline execution, and result aggregation.`,
	RunE: run,
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is /etc/quidditch/coordination.yaml)")
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
	cfg, err := config.LoadCoordinationConfig(cfgFile)
	if err != nil {
		logger.Fatal("Failed to load configuration", zap.Error(err))
	}

	logger.Info("Starting Quidditch Coordination Node",
		zap.String("node_id", cfg.NodeID),
		zap.String("bind_addr", cfg.BindAddr),
		zap.Int("rest_port", cfg.RESTPort),
		zap.Int("grpc_port", cfg.GRPCPort),
		zap.String("master_addr", cfg.MasterAddr),
	)

	// Create coordination node
	coordNode, err := coordination.NewCoordinationNode(cfg, logger)
	if err != nil {
		logger.Fatal("Failed to create coordination node", zap.Error(err))
	}

	// Start coordination node
	if err := coordNode.Start(ctx); err != nil {
		logger.Fatal("Failed to start coordination node", zap.Error(err))
	}

	// Setup signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Coordination node started successfully",
		zap.String("rest_endpoint", fmt.Sprintf("http://%s:%d", cfg.BindAddr, cfg.RESTPort)),
	)

	// Wait for shutdown signal
	<-sigCh
	logger.Info("Received shutdown signal, stopping coordination node...")

	// Graceful shutdown
	if err := coordNode.Stop(ctx); err != nil {
		logger.Error("Error during shutdown", zap.Error(err))
		return err
	}

	logger.Info("Coordination node stopped successfully")
	return nil
}
