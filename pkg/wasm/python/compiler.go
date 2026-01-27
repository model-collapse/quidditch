package python

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Compiler handles Python to WASM compilation
type Compiler struct {
	// Compilation toolchain paths
	micropythonPath string // Path to MicroPython WASM compiler
	pyodinePath     string // Path to Pyodide compiler
	tempDir         string
	logger          *zap.Logger

	// Compilation mode
	mode CompilationMode
}

// CompilationMode defines how Python is compiled to WASM
type CompilationMode string

const (
	// ModePreCompiled expects pre-compiled WASM binaries
	ModePreCompiled CompilationMode = "precompiled"

	// ModeMicroPython uses MicroPython WASM compiler
	ModeMicroPython CompilationMode = "micropython"

	// ModePyodide uses Pyodide for full Python support
	ModePyodide CompilationMode = "pyodide"
)

// UDFMetadata contains metadata extracted from Python source
type UDFMetadata struct {
	Name        string          `json:"name"`
	Version     string          `json:"version"`
	Description string          `json:"description"`
	Author      string          `json:"author"`
	Category    string          `json:"category"` // "filter", "scorer", "aggregator"
	Parameters  []ParameterDef  `json:"parameters"`
	Returns     []ReturnDef     `json:"returns"`
	Tags        []string        `json:"tags"`
	Language    string          `json:"language"` // "python"
	Created     time.Time       `json:"created"`
}

// ParameterDef defines a UDF parameter
type ParameterDef struct {
	Name        string      `json:"name"`
	Type        string      `json:"type"`        // "string", "i64", "f64", "bool"
	Required    bool        `json:"required"`
	Default     interface{} `json:"default,omitempty"`
	Description string      `json:"description,omitempty"`
}

// ReturnDef defines a UDF return value
type ReturnDef struct {
	Type        string `json:"type"` // "string", "i64", "f64", "bool"
	Description string `json:"description,omitempty"`
}

// CompilerConfig configures the Python compiler
type CompilerConfig struct {
	MicroPythonPath string
	PyodinePath     string
	TempDir         string
	Mode            CompilationMode
}

// NewCompiler creates a new Python to WASM compiler
func NewCompiler(config *CompilerConfig, logger *zap.Logger) (*Compiler, error) {
	// Use provided temp dir or create one
	tempDir := config.TempDir
	if tempDir == "" {
		var err error
		tempDir, err = os.MkdirTemp("", "quidditch-python-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir: %w", err)
		}
	}

	// Determine compilation mode
	mode := config.Mode
	if mode == "" {
		// Auto-detect based on available tools
		if config.MicroPythonPath != "" {
			if _, err := os.Stat(config.MicroPythonPath); err == nil {
				mode = ModeMicroPython
			}
		} else if config.PyodinePath != "" {
			if _, err := os.Stat(config.PyodinePath); err == nil {
				mode = ModePyodide
			}
		} else {
			// Default to pre-compiled mode
			mode = ModePreCompiled
		}
	}

	logger.Info("Python compiler initialized",
		zap.String("mode", string(mode)),
		zap.String("temp_dir", tempDir))

	return &Compiler{
		micropythonPath: config.MicroPythonPath,
		pyodinePath:     config.PyodinePath,
		tempDir:         tempDir,
		mode:            mode,
		logger:          logger,
	}, nil
}

// Compile compiles Python source to WASM
func (c *Compiler) Compile(ctx context.Context, source []byte, metadata *UDFMetadata) ([]byte, error) {
	switch c.mode {
	case ModePreCompiled:
		return nil, fmt.Errorf("pre-compiled mode requires WASM binary, not Python source")

	case ModeMicroPython:
		return c.compileMicroPython(ctx, source, metadata)

	case ModePyodide:
		return c.compilePyodide(ctx, source, metadata)

	default:
		return nil, fmt.Errorf("unknown compilation mode: %s", c.mode)
	}
}

// compileMicroPython compiles Python to WASM using MicroPython
func (c *Compiler) compileMicroPython(ctx context.Context, source []byte, metadata *UDFMetadata) ([]byte, error) {
	c.logger.Info("Compiling Python with MicroPython",
		zap.String("name", metadata.Name),
		zap.Int("source_bytes", len(source)))

	// Write Python source to temp file
	sourceFile := filepath.Join(c.tempDir, "udf.py")
	if err := os.WriteFile(sourceFile, source, 0644); err != nil {
		return nil, fmt.Errorf("failed to write source file: %w", err)
	}

	// Compile with MicroPython
	wasmFile := filepath.Join(c.tempDir, "udf.wasm")
	cmd := exec.CommandContext(ctx,
		c.micropythonPath,
		"-m", "mpy-cross",
		"--target", "wasm32",
		"-o", wasmFile,
		sourceFile,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		c.logger.Error("MicroPython compilation failed",
			zap.Error(err),
			zap.String("output", string(output)))
		return nil, fmt.Errorf("compilation failed: %w\nOutput: %s", err, output)
	}

	// Read compiled WASM
	wasmBytes, err := os.ReadFile(wasmFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read WASM file: %w", err)
	}

	c.logger.Info("Python compiled successfully",
		zap.Int("wasm_bytes", len(wasmBytes)))

	return wasmBytes, nil
}

// compilePyodide compiles Python to WASM using Pyodide
func (c *Compiler) compilePyodide(ctx context.Context, source []byte, metadata *UDFMetadata) ([]byte, error) {
	// Pyodide compilation is more complex and typically done via JavaScript
	// This is a placeholder for future implementation
	return nil, fmt.Errorf("Pyodide compilation not yet implemented")
}

// ExtractMetadata extracts UDF metadata from Python source
func (c *Compiler) ExtractMetadata(source []byte) (*UDFMetadata, error) {
	sourceStr := string(source)

	metadata := &UDFMetadata{
		Language: "python",
		Created:  time.Now(),
		Category: "filter", // Default category
	}

	// Extract function name (look for def udf_main or main entry point)
	funcNameRe := regexp.MustCompile(`def\s+(\w+)\s*\(`)
	if matches := funcNameRe.FindStringSubmatch(sourceStr); len(matches) > 1 {
		metadata.Name = matches[1]
	}

	// Extract docstring (first triple-quoted string)
	// Use DOTALL flag to match across newlines
	docstringRe := regexp.MustCompile(`(?s)"""(.*?)"""|'''(.*?)'''`)
	if matches := docstringRe.FindStringSubmatch(sourceStr); len(matches) > 1 {
		docstring := matches[1]
		if docstring == "" {
			docstring = matches[2]
		}
		metadata.Description = strings.TrimSpace(docstring)
	}

	// Extract parameters from function signature
	// Look for: def func(param1: type1, param2: type2) -> return_type:
	sigRe := regexp.MustCompile(`def\s+\w+\s*\((.*?)\)\s*(?:->\s*(\w+))?`)
	if matches := sigRe.FindStringSubmatch(sourceStr); len(matches) > 1 {
		// Parse parameters
		paramsStr := matches[1]
		if paramsStr != "" {
			params := c.parseParameters(paramsStr)
			metadata.Parameters = params
		}

		// Parse return type
		if len(matches) > 2 && matches[2] != "" {
			returnType := c.mapPythonType(matches[2])
			metadata.Returns = []ReturnDef{
				{Type: returnType},
			}
		}
	}

	// Extract metadata from comments
	// Look for: # @udf: key=value
	metaRe := regexp.MustCompile(`#\s*@udf:\s*(\w+)=(.+)`)
	for _, match := range metaRe.FindAllStringSubmatch(sourceStr, -1) {
		if len(match) > 2 {
			key := strings.TrimSpace(match[1])
			value := strings.TrimSpace(match[2])

			switch key {
			case "name":
				metadata.Name = value
			case "version":
				metadata.Version = value
			case "author":
				metadata.Author = value
			case "category":
				metadata.Category = value
			case "tags":
				metadata.Tags = strings.Split(value, ",")
				for i, tag := range metadata.Tags {
					metadata.Tags[i] = strings.TrimSpace(tag)
				}
			}
		}
	}

	// Set defaults if not provided
	if metadata.Name == "" {
		metadata.Name = "python_udf"
	}
	if metadata.Version == "" {
		metadata.Version = "1.0.0"
	}

	return metadata, nil
}

// parseParameters parses Python function parameters
func (c *Compiler) parseParameters(paramsStr string) []ParameterDef {
	var params []ParameterDef

	// Split by comma (simple parsing, doesn't handle complex default values)
	paramList := strings.Split(paramsStr, ",")

	for _, param := range paramList {
		param = strings.TrimSpace(param)
		if param == "" {
			continue
		}

		paramDef := ParameterDef{
			Required: true,
		}

		// Check for default value
		var nameType string
		if strings.Contains(param, "=") {
			parts := strings.SplitN(param, "=", 2)
			nameType = strings.TrimSpace(parts[0])
			defaultStr := strings.TrimSpace(parts[1])
			paramDef.Required = false

			// Try to parse default value
			defaultStr = strings.Trim(defaultStr, "\"'")
			paramDef.Default = defaultStr
		} else {
			nameType = param
		}

		// Parse name and type annotation
		if strings.Contains(nameType, ":") {
			parts := strings.SplitN(nameType, ":", 2)
			paramDef.Name = strings.TrimSpace(parts[0])
			typeStr := strings.TrimSpace(parts[1])
			paramDef.Type = c.mapPythonType(typeStr)
		} else {
			paramDef.Name = nameType
			paramDef.Type = "string" // Default type
		}

		params = append(params, paramDef)
	}

	return params
}

// mapPythonType maps Python type annotations to WASM types
func (c *Compiler) mapPythonType(pythonType string) string {
	pythonType = strings.ToLower(pythonType)

	switch pythonType {
	case "str", "string":
		return "string"
	case "int", "integer":
		return "i64"
	case "float", "double":
		return "f64"
	case "bool", "boolean":
		return "bool"
	default:
		return "string" // Default to string
	}
}

// ValidateMetadata validates UDF metadata
func (c *Compiler) ValidateMetadata(metadata *UDFMetadata) error {
	if metadata.Name == "" {
		return fmt.Errorf("UDF name is required")
	}
	if metadata.Version == "" {
		return fmt.Errorf("UDF version is required")
	}

	// Validate parameter types
	validTypes := map[string]bool{
		"string": true,
		"i64":    true,
		"f64":    true,
		"bool":   true,
	}

	for _, param := range metadata.Parameters {
		if param.Name == "" {
			return fmt.Errorf("parameter name is required")
		}
		if !validTypes[param.Type] {
			return fmt.Errorf("invalid parameter type: %s (must be string, i64, f64, or bool)", param.Type)
		}
	}

	// Validate return types
	for _, ret := range metadata.Returns {
		if !validTypes[ret.Type] {
			return fmt.Errorf("invalid return type: %s (must be string, i64, f64, or bool)", ret.Type)
		}
	}

	return nil
}

// SerializeMetadata serializes metadata to JSON
func (c *Compiler) SerializeMetadata(metadata *UDFMetadata) ([]byte, error) {
	return json.MarshalIndent(metadata, "", "  ")
}

// ParseMetadata parses metadata from JSON
func (c *Compiler) ParseMetadata(data []byte) (*UDFMetadata, error) {
	var metadata UDFMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}
	return &metadata, nil
}

// Cleanup removes temporary files
func (c *Compiler) Cleanup() error {
	if c.tempDir != "" {
		return os.RemoveAll(c.tempDir)
	}
	return nil
}

// GetMode returns the current compilation mode
func (c *Compiler) GetMode() CompilationMode {
	return c.mode
}
