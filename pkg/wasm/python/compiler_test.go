package python

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNewCompiler(t *testing.T) {
	logger := zap.NewNop()

	t.Run("PreCompiledMode", func(t *testing.T) {
		config := &CompilerConfig{
			Mode: ModePreCompiled,
		}

		compiler, err := NewCompiler(config, logger)
		require.NoError(t, err)
		assert.NotNil(t, compiler)
		assert.Equal(t, ModePreCompiled, compiler.GetMode())
		defer compiler.Cleanup()
	})

	t.Run("AutoDetectMode", func(t *testing.T) {
		config := &CompilerConfig{
			// No mode specified, should default to PreCompiled
		}

		compiler, err := NewCompiler(config, logger)
		require.NoError(t, err)
		assert.Equal(t, ModePreCompiled, compiler.GetMode())
		defer compiler.Cleanup()
	})

	t.Run("CustomTempDir", func(t *testing.T) {
		config := &CompilerConfig{
			TempDir: "/tmp/test-python-compiler",
			Mode:    ModePreCompiled,
		}

		compiler, err := NewCompiler(config, logger)
		require.NoError(t, err)
		assert.NotNil(t, compiler)
		defer compiler.Cleanup()
	})
}

func TestExtractMetadata(t *testing.T) {
	logger := zap.NewNop()
	compiler, _ := NewCompiler(&CompilerConfig{Mode: ModePreCompiled}, logger)
	defer compiler.Cleanup()

	t.Run("SimpleFunction", func(t *testing.T) {
		source := []byte(`
def text_filter(title: str, threshold: int) -> bool:
    """Filter documents by text similarity."""
    return True
`)

		metadata, err := compiler.ExtractMetadata(source)
		require.NoError(t, err)

		assert.Equal(t, "text_filter", metadata.Name)
		assert.Equal(t, "Filter documents by text similarity.", metadata.Description)
		assert.Equal(t, "python", metadata.Language)
		assert.Len(t, metadata.Parameters, 2)

		// Check first parameter
		assert.Equal(t, "title", metadata.Parameters[0].Name)
		assert.Equal(t, "string", metadata.Parameters[0].Type)
		assert.True(t, metadata.Parameters[0].Required)

		// Check second parameter
		assert.Equal(t, "threshold", metadata.Parameters[1].Name)
		assert.Equal(t, "i64", metadata.Parameters[1].Type)
		assert.True(t, metadata.Parameters[1].Required)

		// Check return type
		assert.Len(t, metadata.Returns, 1)
		assert.Equal(t, "bool", metadata.Returns[0].Type)
	})

	t.Run("WithMetadataComments", func(t *testing.T) {
		source := []byte(`
# @udf: name=similarity_filter
# @udf: version=2.0.0
# @udf: author=test-author
# @udf: category=filter
# @udf: tags=text,similarity,ml

def udf_main() -> bool:
    """Check similarity."""
    return True
`)

		metadata, err := compiler.ExtractMetadata(source)
		require.NoError(t, err)

		assert.Equal(t, "similarity_filter", metadata.Name)
		assert.Equal(t, "2.0.0", metadata.Version)
		assert.Equal(t, "test-author", metadata.Author)
		assert.Equal(t, "filter", metadata.Category)
		assert.Equal(t, []string{"text", "similarity", "ml"}, metadata.Tags)
		assert.Equal(t, "Check similarity.", metadata.Description)
	})

	t.Run("WithDefaultValues", func(t *testing.T) {
		source := []byte(`
def process(text: str, count: int = 10, threshold: float = 0.5) -> int:
    """Process text with defaults."""
    return 0
`)

		metadata, err := compiler.ExtractMetadata(source)
		require.NoError(t, err)

		assert.Len(t, metadata.Parameters, 3)

		// Required parameter
		assert.Equal(t, "text", metadata.Parameters[0].Name)
		assert.True(t, metadata.Parameters[0].Required)

		// Optional parameters
		assert.Equal(t, "count", metadata.Parameters[1].Name)
		assert.False(t, metadata.Parameters[1].Required)
		assert.Equal(t, "10", metadata.Parameters[1].Default)

		assert.Equal(t, "threshold", metadata.Parameters[2].Name)
		assert.False(t, metadata.Parameters[2].Required)
		assert.Equal(t, "0.5", metadata.Parameters[2].Default)
	})

	t.Run("NoTypeAnnotations", func(t *testing.T) {
		source := []byte(`
def simple_func(a, b):
    """Simple function without types."""
    return True
`)

		metadata, err := compiler.ExtractMetadata(source)
		require.NoError(t, err)

		assert.Equal(t, "simple_func", metadata.Name)
		assert.Len(t, metadata.Parameters, 2)

		// Should default to string type
		assert.Equal(t, "string", metadata.Parameters[0].Type)
		assert.Equal(t, "string", metadata.Parameters[1].Type)
	})

	t.Run("TripleQuotedDocstring", func(t *testing.T) {
		source := []byte(`
def my_func():
    """
    Multi-line docstring
    with details.
    """
    pass
`)

		metadata, err := compiler.ExtractMetadata(source)
		require.NoError(t, err)

		assert.Contains(t, metadata.Description, "Multi-line docstring")
	})

	t.Run("SingleQuotedDocstring", func(t *testing.T) {
		source := []byte(`
def my_func():
    '''Single quoted docstring'''
    pass
`)

		metadata, err := compiler.ExtractMetadata(source)
		require.NoError(t, err)

		assert.Equal(t, "Single quoted docstring", metadata.Description)
	})
}

func TestMapPythonType(t *testing.T) {
	logger := zap.NewNop()
	compiler, _ := NewCompiler(&CompilerConfig{Mode: ModePreCompiled}, logger)
	defer compiler.Cleanup()

	tests := []struct {
		pythonType string
		wasmType   string
	}{
		{"str", "string"},
		{"string", "string"},
		{"int", "i64"},
		{"integer", "i64"},
		{"float", "f64"},
		{"double", "f64"},
		{"bool", "bool"},
		{"boolean", "bool"},
		{"unknown", "string"}, // Default
	}

	for _, tt := range tests {
		t.Run(tt.pythonType, func(t *testing.T) {
			result := compiler.mapPythonType(tt.pythonType)
			assert.Equal(t, tt.wasmType, result)
		})
	}
}

func TestValidateMetadata(t *testing.T) {
	logger := zap.NewNop()
	compiler, _ := NewCompiler(&CompilerConfig{Mode: ModePreCompiled}, logger)
	defer compiler.Cleanup()

	t.Run("ValidMetadata", func(t *testing.T) {
		metadata := &UDFMetadata{
			Name:    "test_udf",
			Version: "1.0.0",
			Parameters: []ParameterDef{
				{Name: "text", Type: "string", Required: true},
				{Name: "count", Type: "i64", Required: false},
			},
			Returns: []ReturnDef{
				{Type: "bool"},
			},
		}

		err := compiler.ValidateMetadata(metadata)
		assert.NoError(t, err)
	})

	t.Run("MissingName", func(t *testing.T) {
		metadata := &UDFMetadata{
			Version: "1.0.0",
		}

		err := compiler.ValidateMetadata(metadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "name is required")
	})

	t.Run("MissingVersion", func(t *testing.T) {
		metadata := &UDFMetadata{
			Name: "test_udf",
		}

		err := compiler.ValidateMetadata(metadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version is required")
	})

	t.Run("InvalidParameterType", func(t *testing.T) {
		metadata := &UDFMetadata{
			Name:    "test_udf",
			Version: "1.0.0",
			Parameters: []ParameterDef{
				{Name: "data", Type: "invalid_type"},
			},
		}

		err := compiler.ValidateMetadata(metadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid parameter type")
	})

	t.Run("InvalidReturnType", func(t *testing.T) {
		metadata := &UDFMetadata{
			Name:    "test_udf",
			Version: "1.0.0",
			Returns: []ReturnDef{
				{Type: "invalid_type"},
			},
		}

		err := compiler.ValidateMetadata(metadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid return type")
	})

	t.Run("MissingParameterName", func(t *testing.T) {
		metadata := &UDFMetadata{
			Name:    "test_udf",
			Version: "1.0.0",
			Parameters: []ParameterDef{
				{Type: "string"},
			},
		}

		err := compiler.ValidateMetadata(metadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "parameter name is required")
	})
}

func TestSerializeMetadata(t *testing.T) {
	logger := zap.NewNop()
	compiler, _ := NewCompiler(&CompilerConfig{Mode: ModePreCompiled}, logger)
	defer compiler.Cleanup()

	metadata := &UDFMetadata{
		Name:        "test_udf",
		Version:     "1.0.0",
		Description: "Test UDF",
		Author:      "test",
		Category:    "filter",
		Parameters: []ParameterDef{
			{Name: "text", Type: "string", Required: true},
		},
		Returns: []ReturnDef{
			{Type: "bool"},
		},
		Tags:     []string{"test", "example"},
		Language: "python",
		Created:  time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := compiler.SerializeMetadata(metadata)
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// Verify JSON structure
	assert.Contains(t, string(data), `"name": "test_udf"`)
	assert.Contains(t, string(data), `"version": "1.0.0"`)
	assert.Contains(t, string(data), `"language": "python"`)
}

func TestParseMetadata(t *testing.T) {
	logger := zap.NewNop()
	compiler, _ := NewCompiler(&CompilerConfig{Mode: ModePreCompiled}, logger)
	defer compiler.Cleanup()

	jsonData := []byte(`{
		"name": "test_udf",
		"version": "1.0.0",
		"description": "Test UDF",
		"author": "test",
		"category": "filter",
		"parameters": [
			{
				"name": "text",
				"type": "string",
				"required": true
			}
		],
		"returns": [
			{
				"type": "bool"
			}
		],
		"tags": ["test"],
		"language": "python"
	}`)

	metadata, err := compiler.ParseMetadata(jsonData)
	require.NoError(t, err)

	assert.Equal(t, "test_udf", metadata.Name)
	assert.Equal(t, "1.0.0", metadata.Version)
	assert.Equal(t, "Test UDF", metadata.Description)
	assert.Len(t, metadata.Parameters, 1)
	assert.Equal(t, "text", metadata.Parameters[0].Name)
}

func TestCompilePreCompiledMode(t *testing.T) {
	logger := zap.NewNop()
	compiler, _ := NewCompiler(&CompilerConfig{Mode: ModePreCompiled}, logger)
	defer compiler.Cleanup()

	source := []byte("def udf_main(): return True")
	metadata := &UDFMetadata{Name: "test", Version: "1.0.0"}

	_, err := compiler.Compile(context.Background(), source, metadata)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pre-compiled mode requires WASM binary")
}

func TestCompileMicroPythonMode(t *testing.T) {
	t.Skip("Requires MicroPython compiler to be installed")

	logger := zap.NewNop()
	compiler, err := NewCompiler(&CompilerConfig{
		Mode:            ModeMicroPython,
		MicroPythonPath: "/usr/local/bin/micropython", // Adjust path
	}, logger)
	require.NoError(t, err)
	defer compiler.Cleanup()

	source := []byte(`
def udf_main():
    return True
`)

	metadata := &UDFMetadata{
		Name:    "test_udf",
		Version: "1.0.0",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	wasm, err := compiler.Compile(ctx, source, metadata)
	require.NoError(t, err)
	assert.NotEmpty(t, wasm)
	assert.Greater(t, len(wasm), 0)
}

func TestCleanup(t *testing.T) {
	logger := zap.NewNop()
	compiler, err := NewCompiler(&CompilerConfig{Mode: ModePreCompiled}, logger)
	require.NoError(t, err)

	tempDir := compiler.tempDir
	assert.NotEmpty(t, tempDir)

	// Cleanup should remove temp directory
	err = compiler.Cleanup()
	assert.NoError(t, err)
}

func TestParseParameters(t *testing.T) {
	logger := zap.NewNop()
	compiler, _ := NewCompiler(&CompilerConfig{Mode: ModePreCompiled}, logger)
	defer compiler.Cleanup()

	t.Run("SimpleParameters", func(t *testing.T) {
		params := compiler.parseParameters("a: str, b: int")
		assert.Len(t, params, 2)
		assert.Equal(t, "a", params[0].Name)
		assert.Equal(t, "string", params[0].Type)
		assert.Equal(t, "b", params[1].Name)
		assert.Equal(t, "i64", params[1].Type)
	})

	t.Run("WithDefaults", func(t *testing.T) {
		params := compiler.parseParameters("a: str, b: int = 10")
		assert.Len(t, params, 2)
		assert.True(t, params[0].Required)
		assert.False(t, params[1].Required)
		assert.Equal(t, "10", params[1].Default)
	})

	t.Run("NoTypes", func(t *testing.T) {
		params := compiler.parseParameters("a, b")
		assert.Len(t, params, 2)
		assert.Equal(t, "string", params[0].Type)
		assert.Equal(t, "string", params[1].Type)
	})

	t.Run("EmptyString", func(t *testing.T) {
		params := compiler.parseParameters("")
		assert.Len(t, params, 0)
	})
}
