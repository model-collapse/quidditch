package data

import (
	"testing"
)

func TestDefaultAnalyzerSettings(t *testing.T) {
	settings := DefaultAnalyzerSettings()

	if settings.DefaultAnalyzer != "standard" {
		t.Errorf("Expected default analyzer 'standard', got '%s'", settings.DefaultAnalyzer)
	}

	if settings.FieldAnalyzers == nil {
		t.Error("Expected FieldAnalyzers map to be initialized")
	}
}

func TestGetAnalyzerForField(t *testing.T) {
	settings := DefaultAnalyzerSettings()

	// Test default analyzer
	analyzer := settings.GetAnalyzerForField("unknown_field")
	if analyzer != "standard" {
		t.Errorf("Expected default analyzer 'standard', got '%s'", analyzer)
	}

	// Set field-specific analyzer
	settings.SetFieldAnalyzer("title", "english")

	// Test field-specific analyzer
	analyzer = settings.GetAnalyzerForField("title")
	if analyzer != "english" {
		t.Errorf("Expected 'english' analyzer, got '%s'", analyzer)
	}

	// Test other fields still use default
	analyzer = settings.GetAnalyzerForField("description")
	if analyzer != "standard" {
		t.Errorf("Expected default analyzer 'standard', got '%s'", analyzer)
	}
}

func TestSetFieldAnalyzer(t *testing.T) {
	settings := DefaultAnalyzerSettings()

	settings.SetFieldAnalyzer("title", "english")
	settings.SetFieldAnalyzer("description", "simple")
	settings.SetFieldAnalyzer("tags", "keyword")

	tests := []struct {
		field    string
		expected string
	}{
		{"title", "english"},
		{"description", "simple"},
		{"tags", "keyword"},
		{"other", "standard"}, // default
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			analyzer := settings.GetAnalyzerForField(tt.field)
			if analyzer != tt.expected {
				t.Errorf("Field %s: expected '%s', got '%s'", tt.field, tt.expected, analyzer)
			}
		})
	}
}

func TestValidateAnalyzerSettings(t *testing.T) {
	tests := []struct {
		name        string
		settings    *AnalyzerSettings
		shouldError bool
	}{
		{
			name:        "valid default settings",
			settings:    DefaultAnalyzerSettings(),
			shouldError: false,
		},
		{
			name: "valid custom settings",
			settings: &AnalyzerSettings{
				DefaultAnalyzer: "english",
				FieldAnalyzers: map[string]string{
					"title":       "standard",
					"description": "simple",
				},
			},
			shouldError: false,
		},
		{
			name: "invalid default analyzer",
			settings: &AnalyzerSettings{
				DefaultAnalyzer: "invalid_analyzer",
				FieldAnalyzers:  map[string]string{},
			},
			shouldError: true,
		},
		{
			name: "invalid field analyzer",
			settings: &AnalyzerSettings{
				DefaultAnalyzer: "standard",
				FieldAnalyzers: map[string]string{
					"title": "invalid_analyzer",
				},
			},
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.settings.Validate()
			if tt.shouldError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

func TestAnalyzerCache(t *testing.T) {
	cache := NewAnalyzerCache()
	defer cache.Close()

	// Get analyzer first time (should create)
	analyzer1, err := cache.GetOrCreate("standard")
	if err != nil {
		t.Fatalf("Failed to get analyzer: %v", err)
	}

	// Get analyzer second time (should return cached)
	analyzer2, err := cache.GetOrCreate("standard")
	if err != nil {
		t.Fatalf("Failed to get cached analyzer: %v", err)
	}

	// Should be same instance
	if analyzer1 != analyzer2 {
		t.Error("Expected cached analyzer to be same instance")
	}

	// Get different analyzer
	analyzer3, err := cache.GetOrCreate("simple")
	if err != nil {
		t.Fatalf("Failed to get simple analyzer: %v", err)
	}

	// Should be different instance
	if analyzer1 == analyzer3 {
		t.Error("Expected different analyzers to be different instances")
	}
}

func TestAnalyzeField(t *testing.T) {
	cache := NewAnalyzerCache()
	defer cache.Close()

	settings := DefaultAnalyzerSettings()
	settings.SetFieldAnalyzer("title", "simple")

	tests := []struct {
		field     string
		value     string
		minTokens int
	}{
		{"title", "Hello World", 2},           // simple analyzer
		{"description", "The quick fox", 2},   // standard analyzer (removes "the")
		{"tags", "one-tag", 1},                // standard analyzer (default)
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			tokens, err := AnalyzeField(cache, settings, tt.field, tt.value)
			if err != nil {
				t.Fatalf("Failed to analyze field: %v", err)
			}

			if len(tokens) < tt.minTokens {
				t.Errorf("Expected at least %d tokens, got %d: %v", tt.minTokens, len(tokens), tokens)
			}

			t.Logf("Field %s: %s -> %v", tt.field, tt.value, tokens)
		})
	}
}

func TestAnalyzeFieldWithDifferentAnalyzers(t *testing.T) {
	cache := NewAnalyzerCache()
	defer cache.Close()

	settings := DefaultAnalyzerSettings()

	// Test with different analyzers
	tests := []struct {
		analyzer string
		value    string
		contains string
	}{
		{"simple", "Hello World", "hello"},        // lowercased
		{"whitespace", "Hello World", "Hello"},    // not lowercased
		{"keyword", "one two three", "one two three"}, // not split
		{"english", "café", "cafe"},               // ASCII folded
	}

	for _, tt := range tests {
		t.Run(tt.analyzer, func(t *testing.T) {
			settings.DefaultAnalyzer = tt.analyzer
			tokens, err := AnalyzeField(cache, settings, "test", tt.value)
			if err != nil {
				t.Fatalf("Failed to analyze: %v", err)
			}

			found := false
			for _, token := range tokens {
				if token == tt.contains {
					found = true
					break
				}
			}

			if !found {
				t.Errorf("Expected tokens to contain '%s', got: %v", tt.contains, tokens)
			}
		})
	}
}
