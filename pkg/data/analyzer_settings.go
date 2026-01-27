package data

import (
	"fmt"

	"github.com/quidditch/quidditch/pkg/data/diagon"
)

// AnalyzerSettings defines text analysis configuration for an index.
type AnalyzerSettings struct {
	// Default analyzer for all text fields (if not overridden)
	DefaultAnalyzer string `json:"default_analyzer"`

	// Per-field analyzer overrides
	FieldAnalyzers map[string]string `json:"field_analyzers,omitempty"`

	// Custom analyzer definitions (future enhancement)
	CustomAnalyzers map[string]AnalyzerDefinition `json:"custom_analyzers,omitempty"`
}

// AnalyzerDefinition defines a custom analyzer configuration.
// This is for future enhancement to support custom tokenizers and filters.
type AnalyzerDefinition struct {
	Tokenizer string   `json:"tokenizer"`
	Filters   []string `json:"filters,omitempty"`
}

// DefaultAnalyzerSettings returns default analyzer settings.
func DefaultAnalyzerSettings() *AnalyzerSettings {
	return &AnalyzerSettings{
		DefaultAnalyzer: "standard",
		FieldAnalyzers:  make(map[string]string),
		CustomAnalyzers: make(map[string]AnalyzerDefinition),
	}
}

// GetAnalyzerForField returns the analyzer name for a given field.
// Returns the field-specific analyzer if defined, otherwise the default analyzer.
func (as *AnalyzerSettings) GetAnalyzerForField(fieldName string) string {
	if analyzerName, exists := as.FieldAnalyzers[fieldName]; exists {
		return analyzerName
	}
	return as.DefaultAnalyzer
}

// SetFieldAnalyzer sets the analyzer for a specific field.
func (as *AnalyzerSettings) SetFieldAnalyzer(fieldName, analyzerName string) {
	if as.FieldAnalyzers == nil {
		as.FieldAnalyzers = make(map[string]string)
	}
	as.FieldAnalyzers[fieldName] = analyzerName
}

// Validate checks if the analyzer settings are valid.
func (as *AnalyzerSettings) Validate() error {
	// Check if default analyzer is valid
	if err := validateAnalyzerName(as.DefaultAnalyzer); err != nil {
		return fmt.Errorf("invalid default analyzer: %w", err)
	}

	// Check if field analyzers are valid
	for field, analyzerName := range as.FieldAnalyzers {
		if err := validateAnalyzerName(analyzerName); err != nil {
			return fmt.Errorf("invalid analyzer for field %s: %w", field, err)
		}
	}

	return nil
}

// validateAnalyzerName checks if an analyzer name is valid.
func validateAnalyzerName(name string) error {
	validAnalyzers := map[string]bool{
		"standard":      true,
		"simple":        true,
		"whitespace":    true,
		"keyword":       true,
		"chinese":       true,
		"english":       true,
		"multilingual":  true,
		"search":        true,
	}

	if !validAnalyzers[name] {
		return fmt.Errorf("unknown analyzer: %s", name)
	}

	return nil
}

// AnalyzerCache caches analyzer instances to avoid recreating them for each document.
type AnalyzerCache struct {
	analyzers map[string]*diagon.Analyzer
}

// NewAnalyzerCache creates a new analyzer cache.
func NewAnalyzerCache() *AnalyzerCache {
	return &AnalyzerCache{
		analyzers: make(map[string]*diagon.Analyzer),
	}
}

// GetOrCreate gets an analyzer from the cache or creates it if not found.
func (ac *AnalyzerCache) GetOrCreate(name string) (*diagon.Analyzer, error) {
	if analyzer, exists := ac.analyzers[name]; exists {
		return analyzer, nil
	}

	// Create new analyzer
	analyzer, err := diagon.NewAnalyzer(name)
	if err != nil {
		return nil, err
	}

	ac.analyzers[name] = analyzer
	return analyzer, nil
}

// Close closes all cached analyzers.
func (ac *AnalyzerCache) Close() {
	for _, analyzer := range ac.analyzers {
		analyzer.Close()
	}
	ac.analyzers = make(map[string]*diagon.Analyzer)
}

// AnalyzeField analyzes a field value using the appropriate analyzer.
func AnalyzeField(cache *AnalyzerCache, settings *AnalyzerSettings, fieldName, fieldValue string) ([]string, error) {
	// Get analyzer name for field
	analyzerName := settings.GetAnalyzerForField(fieldName)

	// Get or create analyzer
	analyzer, err := cache.GetOrCreate(analyzerName)
	if err != nil {
		return nil, fmt.Errorf("failed to get analyzer %s: %w", analyzerName, err)
	}

	// Analyze field value
	tokens, err := analyzer.AnalyzeToStrings(fieldValue)
	if err != nil {
		return nil, fmt.Errorf("failed to analyze field %s: %w", fieldName, err)
	}

	return tokens, nil
}
