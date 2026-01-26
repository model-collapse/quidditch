package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// MasterConfig holds configuration for master nodes
type MasterConfig struct {
	NodeID      string
	BindAddr    string
	RaftPort    int
	GRPCPort    int
	DataDir     string
	Peers       []string
	LogLevel    string
	MetricsPort int
}

// CoordinationConfig holds configuration for coordination nodes
type CoordinationConfig struct {
	NodeID         string
	BindAddr       string
	RESTPort       int
	GRPCPort       int
	MasterAddr     string
	CalciteAddr    string
	PythonEnabled  bool
	PythonPath     string
	LogLevel       string
	MetricsPort    int
	MaxConcurrent  int
	RequestTimeout time.Duration
}

// DataNodeConfig holds configuration for data nodes (Diagon)
type DataNodeConfig struct {
	NodeID       string
	BindAddr     string
	GRPCPort     int
	DataDir      string
	MasterAddr   string
	StorageTier  string // hot, warm, cold, frozen
	MaxShards    int
	LogLevel     string
	MetricsPort  int
	SIMDEnabled  bool
}

// LoadMasterConfig loads master node configuration from file
func LoadMasterConfig(cfgFile string) (*MasterConfig, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("node_id", getHostname())
	v.SetDefault("bind_addr", "0.0.0.0")
	v.SetDefault("raft_port", 9300)
	v.SetDefault("grpc_port", 9301)
	v.SetDefault("data_dir", "/var/lib/quidditch/master")
	v.SetDefault("log_level", "info")
	v.SetDefault("metrics_port", 9400)

	// Load config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("master")
		v.SetConfigType("yaml")
		v.AddConfigPath("/etc/quidditch/")
		v.AddConfigPath("$HOME/.quidditch/")
		v.AddConfigPath(".")
	}

	// Read environment variables
	v.SetEnvPrefix("QUIDDITCH")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	cfg := &MasterConfig{
		NodeID:      v.GetString("node_id"),
		BindAddr:    v.GetString("bind_addr"),
		RaftPort:    v.GetInt("raft_port"),
		GRPCPort:    v.GetInt("grpc_port"),
		DataDir:     v.GetString("data_dir"),
		Peers:       v.GetStringSlice("peers"),
		LogLevel:    v.GetString("log_level"),
		MetricsPort: v.GetInt("metrics_port"),
	}

	return cfg, nil
}

// LoadCoordinationConfig loads coordination node configuration from file
func LoadCoordinationConfig(cfgFile string) (*CoordinationConfig, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("node_id", getHostname())
	v.SetDefault("bind_addr", "0.0.0.0")
	v.SetDefault("rest_port", 9200)
	v.SetDefault("grpc_port", 9302)
	v.SetDefault("master_addr", "localhost:9301")
	v.SetDefault("calcite_addr", "localhost:50051")
	v.SetDefault("python_enabled", true)
	v.SetDefault("python_path", "/usr/lib/python3.11")
	v.SetDefault("log_level", "info")
	v.SetDefault("metrics_port", 9401)
	v.SetDefault("max_concurrent", 1000)
	v.SetDefault("request_timeout", "30s")

	// Load config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("coordination")
		v.SetConfigType("yaml")
		v.AddConfigPath("/etc/quidditch/")
		v.AddConfigPath("$HOME/.quidditch/")
		v.AddConfigPath(".")
	}

	// Read environment variables
	v.SetEnvPrefix("QUIDDITCH")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	cfg := &CoordinationConfig{
		NodeID:         v.GetString("node_id"),
		BindAddr:       v.GetString("bind_addr"),
		RESTPort:       v.GetInt("rest_port"),
		GRPCPort:       v.GetInt("grpc_port"),
		MasterAddr:     v.GetString("master_addr"),
		CalciteAddr:    v.GetString("calcite_addr"),
		PythonEnabled:  v.GetBool("python_enabled"),
		PythonPath:     v.GetString("python_path"),
		LogLevel:       v.GetString("log_level"),
		MetricsPort:    v.GetInt("metrics_port"),
		MaxConcurrent:  v.GetInt("max_concurrent"),
		RequestTimeout: v.GetDuration("request_timeout"),
	}

	return cfg, nil
}

// LoadDataNodeConfig loads data node configuration from file
func LoadDataNodeConfig(cfgFile string) (*DataNodeConfig, error) {
	v := viper.New()

	// Set defaults
	v.SetDefault("node_id", getHostname())
	v.SetDefault("bind_addr", "0.0.0.0")
	v.SetDefault("grpc_port", 9303)
	v.SetDefault("data_dir", "/var/lib/quidditch/data")
	v.SetDefault("master_addr", "localhost:9301")
	v.SetDefault("storage_tier", "hot")
	v.SetDefault("max_shards", 100)
	v.SetDefault("log_level", "info")
	v.SetDefault("metrics_port", 9402)
	v.SetDefault("simd_enabled", true)

	// Load config file
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		v.SetConfigName("data")
		v.SetConfigType("yaml")
		v.AddConfigPath("/etc/quidditch/")
		v.AddConfigPath("$HOME/.quidditch/")
		v.AddConfigPath(".")
	}

	// Read environment variables
	v.SetEnvPrefix("QUIDDITCH")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	cfg := &DataNodeConfig{
		NodeID:      v.GetString("node_id"),
		BindAddr:    v.GetString("bind_addr"),
		GRPCPort:    v.GetInt("grpc_port"),
		DataDir:     v.GetString("data_dir"),
		MasterAddr:  v.GetString("master_addr"),
		StorageTier: v.GetString("storage_tier"),
		MaxShards:   v.GetInt("max_shards"),
		LogLevel:    v.GetString("log_level"),
		MetricsPort: v.GetInt("metrics_port"),
		SIMDEnabled: v.GetBool("simd_enabled"),
	}

	return cfg, nil
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}
