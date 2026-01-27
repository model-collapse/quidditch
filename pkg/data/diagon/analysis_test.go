package diagon

import (
	"testing"
)

func TestStandardAnalyzer(t *testing.T) {
	analyzer, err := NewStandardAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create standard analyzer: %v", err)
	}
	defer analyzer.Close()

	text := "The quick brown fox jumps over the lazy dog"
	tokens, err := analyzer.Analyze(text)
	if err != nil {
		t.Fatalf("Failed to analyze text: %v", err)
	}

	if len(tokens) == 0 {
		t.Fatal("Expected tokens, got none")
	}

	// Standard analyzer should lowercase and remove stop words
	// "The" and "the" should be filtered out
	// Should have: quick, brown, fox, jumps, over, lazy, dog (7 tokens)
	t.Logf("Tokens: %v", tokens)
	if len(tokens) < 5 {
		t.Errorf("Expected at least 5 tokens, got %d", len(tokens))
	}
}

func TestSimpleAnalyzer(t *testing.T) {
	analyzer, err := NewSimpleAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create simple analyzer: %v", err)
	}
	defer analyzer.Close()

	text := "Hello World"
	tokens, err := analyzer.Analyze(text)
	if err != nil {
		t.Fatalf("Failed to analyze text: %v", err)
	}

	if len(tokens) != 2 {
		t.Errorf("Expected 2 tokens, got %d", len(tokens))
	}

	// Should be lowercased
	if tokens[0].Text != "hello" {
		t.Errorf("Expected 'hello', got '%s'", tokens[0].Text)
	}
	if tokens[1].Text != "world" {
		t.Errorf("Expected 'world', got '%s'", tokens[1].Text)
	}
}

func TestWhitespaceAnalyzer(t *testing.T) {
	analyzer, err := NewWhitespaceAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create whitespace analyzer: %v", err)
	}
	defer analyzer.Close()

	text := "one two three"
	tokens, err := analyzer.Analyze(text)
	if err != nil {
		t.Fatalf("Failed to analyze text: %v", err)
	}

	if len(tokens) != 3 {
		t.Errorf("Expected 3 tokens, got %d", len(tokens))
	}
}

func TestKeywordAnalyzer(t *testing.T) {
	analyzer, err := NewKeywordAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create keyword analyzer: %v", err)
	}
	defer analyzer.Close()

	text := "this is a keyword field"
	tokens, err := analyzer.Analyze(text)
	if err != nil {
		t.Fatalf("Failed to analyze text: %v", err)
	}

	if len(tokens) != 1 {
		t.Errorf("Expected 1 token, got %d", len(tokens))
	}

	if tokens[0].Text != text {
		t.Errorf("Expected '%s', got '%s'", text, tokens[0].Text)
	}
}

func TestChineseAnalyzer(t *testing.T) {
	analyzer, err := NewChineseAnalyzer("")
	if err != nil {
		t.Skipf("Skipping Chinese analyzer test (may not have dictionaries): %v", err)
		return
	}
	defer analyzer.Close()

	text := "我爱北京天安门"
	tokens, err := analyzer.Analyze(text)
	if err != nil {
		t.Fatalf("Failed to analyze Chinese text: %v", err)
	}

	if len(tokens) == 0 {
		t.Fatal("Expected tokens from Chinese text, got none")
	}

	t.Logf("Chinese tokens: %v", tokens)
}

func TestEnglishAnalyzer(t *testing.T) {
	analyzer, err := NewEnglishAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create English analyzer: %v", err)
	}
	defer analyzer.Close()

	text := "café résumé naïve"
	tokens, err := analyzer.Analyze(text)
	if err != nil {
		t.Fatalf("Failed to analyze text: %v", err)
	}

	if len(tokens) != 3 {
		t.Errorf("Expected 3 tokens, got %d", len(tokens))
	}

	// Should be ASCII folded
	// café -> cafe, résumé -> resume, naïve -> naive
	expected := []string{"cafe", "resume", "naive"}
	for i, token := range tokens {
		if token.Text != expected[i] {
			t.Errorf("Token %d: expected '%s', got '%s'", i, expected[i], token.Text)
		}
	}
}

func TestAnalyzeToStrings(t *testing.T) {
	analyzer, err := NewSimpleAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	text := "Hello World"
	tokens, err := analyzer.AnalyzeToStrings(text)
	if err != nil {
		t.Fatalf("Failed to analyze text: %v", err)
	}

	expected := []string{"hello", "world"}
	if len(tokens) != len(expected) {
		t.Errorf("Expected %d tokens, got %d", len(expected), len(tokens))
	}

	for i, token := range tokens {
		if token != expected[i] {
			t.Errorf("Token %d: expected '%s', got '%s'", i, expected[i], token)
		}
	}
}

func TestNewAnalyzer(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"standard", "standard"},
		{"simple", "simple"},
		{"whitespace", "whitespace"},
		{"keyword", "keyword"},
		{"english", "english"},
		{"multilingual", "multilingual"},
		{"search", "search"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			analyzer, err := NewAnalyzer(tt.name)
			if err != nil {
				t.Fatalf("Failed to create %s analyzer: %v", tt.name, err)
			}
			defer analyzer.Close()

			name := analyzer.Name()
			if name != tt.expected {
				t.Errorf("Expected name '%s', got '%s'", tt.expected, name)
			}
		})
	}
}

func TestAnalyzerInfo(t *testing.T) {
	analyzer, err := NewStandardAnalyzer()
	if err != nil {
		t.Fatalf("Failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	name := analyzer.Name()
	if name == "" {
		t.Error("Expected non-empty name")
	}

	desc := analyzer.Description()
	if desc == "" {
		t.Error("Expected non-empty description")
	}

	t.Logf("Analyzer: %s - %s", name, desc)
}

func BenchmarkStandardAnalyzer(b *testing.B) {
	analyzer, err := NewStandardAnalyzer()
	if err != nil {
		b.Fatalf("Failed to create analyzer: %v", err)
	}
	defer analyzer.Close()

	text := "The quick brown fox jumps over the lazy dog"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := analyzer.Analyze(text)
		if err != nil {
			b.Fatalf("Failed to analyze: %v", err)
		}
	}
}
