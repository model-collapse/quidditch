package diagon

/*
#cgo CFLAGS: -I${SRCDIR}/upstream/src/core/include
#cgo LDFLAGS: -L${SRCDIR}/upstream/build/src/core -L/home/ubuntu/miniconda3/lib -ldiagon_core -licuuc -licui18n -lstdc++ -lm -Wl,-rpath,${SRCDIR}/upstream/build/src/core -Wl,-rpath,/home/ubuntu/miniconda3/lib
#include <stdlib.h>
#include "diagon/analysis_c.h"
*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

// ==================== Analyzer ====================

// Analyzer performs text analysis, converting text into tokens.
type Analyzer struct {
	handle *C.diagon_analyzer_t
	name   string
}

// Token represents a single analyzed token with position information.
type Token struct {
	Text        string
	Position    int
	StartOffset int
	EndOffset   int
	Type        string
}

// ==================== Analyzer Creation ====================

// NewStandardAnalyzer creates a standard analyzer (standard tokenizer + lowercase + stop).
func NewStandardAnalyzer() (*Analyzer, error) {
	handle := C.diagon_create_standard_analyzer()
	if handle == nil {
		return nil, getLastError()
	}
	return &Analyzer{handle: handle, name: "standard"}, nil
}

// NewSimpleAnalyzer creates a simple analyzer (whitespace tokenizer + lowercase).
func NewSimpleAnalyzer() (*Analyzer, error) {
	handle := C.diagon_create_simple_analyzer()
	if handle == nil {
		return nil, getLastError()
	}
	return &Analyzer{handle: handle, name: "simple"}, nil
}

// NewWhitespaceAnalyzer creates a whitespace analyzer.
func NewWhitespaceAnalyzer() (*Analyzer, error) {
	handle := C.diagon_create_whitespace_analyzer()
	if handle == nil {
		return nil, getLastError()
	}
	return &Analyzer{handle: handle, name: "whitespace"}, nil
}

// NewKeywordAnalyzer creates a keyword analyzer (no tokenization).
func NewKeywordAnalyzer() (*Analyzer, error) {
	handle := C.diagon_create_keyword_analyzer()
	if handle == nil {
		return nil, getLastError()
	}
	return &Analyzer{handle: handle, name: "keyword"}, nil
}

// NewChineseAnalyzer creates a Chinese analyzer (Jieba + Chinese stop words).
// If dictPath is empty, uses default dictionary location.
func NewChineseAnalyzer(dictPath string) (*Analyzer, error) {
	var cDictPath *C.char
	if dictPath != "" {
		cDictPath = C.CString(dictPath)
		defer C.free(unsafe.Pointer(cDictPath))
	}

	handle := C.diagon_create_chinese_analyzer(cDictPath)
	if handle == nil {
		return nil, getLastError()
	}
	return &Analyzer{handle: handle, name: "chinese"}, nil
}

// NewEnglishAnalyzer creates an English analyzer (standard + lowercase + ascii folding + stop).
func NewEnglishAnalyzer() (*Analyzer, error) {
	handle := C.diagon_create_english_analyzer()
	if handle == nil {
		return nil, getLastError()
	}
	return &Analyzer{handle: handle, name: "english"}, nil
}

// NewMultilingualAnalyzer creates a multilingual analyzer (standard + lowercase + ascii folding).
func NewMultilingualAnalyzer() (*Analyzer, error) {
	handle := C.diagon_create_multilingual_analyzer()
	if handle == nil {
		return nil, getLastError()
	}
	return &Analyzer{handle: handle, name: "multilingual"}, nil
}

// NewSearchAnalyzer creates a search analyzer optimized for queries.
func NewSearchAnalyzer() (*Analyzer, error) {
	handle := C.diagon_create_search_analyzer()
	if handle == nil {
		return nil, getLastError()
	}
	return &Analyzer{handle: handle, name: "search"}, nil
}

// NewAnalyzer creates an analyzer by name.
func NewAnalyzer(name string) (*Analyzer, error) {
	switch name {
	case "standard":
		return NewStandardAnalyzer()
	case "simple":
		return NewSimpleAnalyzer()
	case "whitespace":
		return NewWhitespaceAnalyzer()
	case "keyword":
		return NewKeywordAnalyzer()
	case "chinese":
		return NewChineseAnalyzer("")
	case "english":
		return NewEnglishAnalyzer()
	case "multilingual":
		return NewMultilingualAnalyzer()
	case "search":
		return NewSearchAnalyzer()
	default:
		return nil, fmt.Errorf("unknown analyzer: %s", name)
	}
}

// Close destroys the analyzer and frees resources.
func (a *Analyzer) Close() {
	if a.handle != nil {
		C.diagon_destroy_analyzer(a.handle)
		a.handle = nil
	}
}

// ==================== Text Analysis ====================

// Analyze analyzes text and returns tokens.
func (a *Analyzer) Analyze(text string) ([]Token, error) {
	if a.handle == nil {
		return nil, errors.New("analyzer is closed")
	}

	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	cTokens := C.diagon_analyze_text(a.handle, cText, C.size_t(len(text)))
	if cTokens == nil {
		return nil, getLastError()
	}
	defer C.diagon_free_tokens(cTokens)

	// Convert C tokens to Go tokens
	count := int(cTokens.count)
	tokens := make([]Token, count)

	// Access token array
	cTokenArray := (*[1 << 30]*C.diagon_token_t)(unsafe.Pointer(cTokens.tokens))[:count:count]

	for i := 0; i < count; i++ {
		cToken := cTokenArray[i]

		tokens[i] = Token{
			Text:        C.GoString(C.diagon_token_get_text(cToken)),
			Position:    int(C.diagon_token_get_position(cToken)),
			StartOffset: int(C.diagon_token_get_start_offset(cToken)),
			EndOffset:   int(C.diagon_token_get_end_offset(cToken)),
			Type:        C.GoString(C.diagon_token_get_type(cToken)),
		}
	}

	return tokens, nil
}

// AnalyzeToStrings is a convenience method that returns just the token text.
func (a *Analyzer) AnalyzeToStrings(text string) ([]string, error) {
	tokens, err := a.Analyze(text)
	if err != nil {
		return nil, err
	}

	result := make([]string, len(tokens))
	for i, token := range tokens {
		result[i] = token.Text
	}
	return result, nil
}

// ==================== Analyzer Info ====================

// Name returns the analyzer name.
func (a *Analyzer) Name() string {
	if a.handle == nil {
		return ""
	}
	return C.GoString(C.diagon_analyzer_get_name(a.handle))
}

// Description returns the analyzer description.
func (a *Analyzer) Description() string {
	if a.handle == nil {
		return ""
	}
	return C.GoString(C.diagon_analyzer_get_description(a.handle))
}

// ==================== Error Handling ====================

func getLastError() error {
	errMsg := C.diagon_get_last_error()
	if errMsg == nil {
		return errors.New("unknown error")
	}
	return errors.New(C.GoString(errMsg))
}

// ClearError clears the last error.
func ClearError() {
	C.diagon_clear_error()
}
