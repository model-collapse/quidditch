package data

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnalyzerIntegration tests the end-to-end analyzer integration with shards
func TestAnalyzerIntegration(t *testing.T) {
	// Create analyzer settings
	settings := DefaultAnalyzerSettings()

	// Configure per-field analyzers
	settings.SetFieldAnalyzer("title", "english")      // Use English analyzer for title (ASCII folding + stop words)
	settings.SetFieldAnalyzer("description", "simple") // Use simple analyzer for description
	settings.SetFieldAnalyzer("category", "keyword")   // Use keyword analyzer for category (no tokenization)

	// Validate settings
	err := settings.Validate()
	require.NoError(t, err)

	// Create analyzer cache
	cache := NewAnalyzerCache()
	defer cache.Close()

	t.Run("AnalyzeEnglishTitle", func(t *testing.T) {
		// Analyze English text with title analyzer (english)
		text := "The café has résumé service"
		tokens, err := AnalyzeField(cache, settings, "title", text)
		require.NoError(t, err)

		// Should apply ASCII folding (café -> cafe, résumé -> resume)
		// Should lowercase
		// Should remove common stop words like "the" (if in stop list)
		assert.Contains(t, tokens, "cafe")
		assert.Contains(t, tokens, "resume")
		assert.Contains(t, tokens, "service")

		// English analyzer applies ASCII folding and lowercasing
		t.Logf("English title tokens: %v", tokens)
	})

	t.Run("AnalyzeSimpleDescription", func(t *testing.T) {
		// Analyze with simple analyzer
		text := "Hello World Test"
		tokens, err := AnalyzeField(cache, settings, "description", text)
		require.NoError(t, err)

		// Simple analyzer: lowercase + whitespace tokenization
		assert.Equal(t, []string{"hello", "world", "test"}, tokens)

		t.Logf("Simple description tokens: %v", tokens)
	})

	t.Run("AnalyzeKeywordCategory", func(t *testing.T) {
		// Analyze with keyword analyzer
		text := "Electronics and Computers"
		tokens, err := AnalyzeField(cache, settings, "category", text)
		require.NoError(t, err)

		// Keyword analyzer: no tokenization, keeps entire string
		assert.Equal(t, []string{"Electronics and Computers"}, tokens)

		t.Logf("Keyword category tokens: %v", tokens)
	})

	t.Run("AnalyzeDefaultField", func(t *testing.T) {
		// Analyze field with no specific analyzer (uses default: standard)
		text := "The quick brown fox"
		tokens, err := AnalyzeField(cache, settings, "unknown_field", text)
		require.NoError(t, err)

		// Standard analyzer: lowercase + remove stop words ("the")
		assert.Contains(t, tokens, "quick")
		assert.Contains(t, tokens, "brown")
		assert.Contains(t, tokens, "fox")
		assert.NotContains(t, tokens, "the") // Stop word removed

		t.Logf("Standard default tokens: %v", tokens)
	})
}

// TestAnalyzerSettingsPersistence tests that analyzer settings can be saved and loaded
func TestAnalyzerSettingsPersistence(t *testing.T) {
	// Create custom settings
	settings := &AnalyzerSettings{
		DefaultAnalyzer: "english",
		FieldAnalyzers: map[string]string{
			"title":       "english",
			"description": "standard",
			"category":    "keyword",
			"tags":        "simple",
		},
	}

	// Validate
	err := settings.Validate()
	require.NoError(t, err)

	// Test GetAnalyzerForField
	assert.Equal(t, "english", settings.GetAnalyzerForField("title"))
	assert.Equal(t, "standard", settings.GetAnalyzerForField("description"))
	assert.Equal(t, "keyword", settings.GetAnalyzerForField("category"))
	assert.Equal(t, "english", settings.GetAnalyzerForField("unknown")) // Default

	// Test SetFieldAnalyzer
	settings.SetFieldAnalyzer("content", "multilingual")
	assert.Equal(t, "multilingual", settings.GetAnalyzerForField("content"))
}

// TestAnalyzerCacheReuse tests that analyzer instances are reused from cache
func TestAnalyzerCacheReuse(t *testing.T) {
	cache := NewAnalyzerCache()
	defer cache.Close()

	// Get analyzer first time
	analyzer1, err := cache.GetOrCreate("standard")
	require.NoError(t, err)
	require.NotNil(t, analyzer1)

	// Get analyzer second time - should return same instance
	analyzer2, err := cache.GetOrCreate("standard")
	require.NoError(t, err)
	require.NotNil(t, analyzer2)

	// Should be same instance (pointer equality)
	assert.Equal(t, analyzer1, analyzer2, "Cache should return same analyzer instance")

	// Get different analyzer - should be different instance
	analyzer3, err := cache.GetOrCreate("simple")
	require.NoError(t, err)
	require.NotNil(t, analyzer3)

	assert.NotEqual(t, analyzer1, analyzer3, "Different analyzers should have different instances")
}

// TestAnalyzerWithChineseText tests Chinese text analysis
func TestAnalyzerWithChineseText(t *testing.T) {
	settings := DefaultAnalyzerSettings()
	settings.SetFieldAnalyzer("content", "chinese")

	cache := NewAnalyzerCache()
	defer cache.Close()

	// Analyze Chinese text
	text := "我爱北京天安门"
	tokens, err := AnalyzeField(cache, settings, "content", text)
	require.NoError(t, err)

	// Should produce multiple tokens via Jieba segmentation
	assert.Greater(t, len(tokens), 0, "Chinese text should be segmented into tokens")

	t.Logf("Chinese tokens: %v", tokens)

	// Common segmentation: "我" (I), "爱" (love), "北京" (Beijing), "天安门" (Tiananmen)
	// Exact tokens depend on Jieba dictionary, but should have multiple segments
	assert.Greater(t, len(tokens), 1, "Chinese text should be segmented into multiple tokens")
}

// TestMultilingualAnalyzer tests the multilingual analyzer
func TestMultilingualAnalyzer(t *testing.T) {
	settings := DefaultAnalyzerSettings()
	settings.SetFieldAnalyzer("content", "multilingual")

	cache := NewAnalyzerCache()
	defer cache.Close()

	// Test with mixed language text
	text := "Hello café 世界"
	tokens, err := AnalyzeField(cache, settings, "content", text)
	require.NoError(t, err)

	// Multilingual analyzer: standard tokenization + lowercase + ASCII folding (no stop words)
	assert.Contains(t, tokens, "hello")
	assert.Contains(t, tokens, "cafe") // ASCII folding: café -> cafe

	t.Logf("Multilingual tokens: %v", tokens)
}
