package wasm

import (
	"fmt"
	"time"
)

// UDFMetadata contains metadata about a WASM User-Defined Function
type UDFMetadata struct {
	// Basic info
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`

	// Function signature
	FunctionName string               `json:"function_name"` // Entry point in WASM module
	Parameters   []UDFParameter       `json:"parameters"`
	Returns      []UDFReturnType      `json:"returns"`

	// WASM module
	WASMBytes []byte `json:"-"` // Raw WASM bytes (not serialized)
	WASMSize  int    `json:"wasm_size"`

	// Performance hints
	ExpectedLatency time.Duration `json:"expected_latency,omitempty"` // Expected execution time
	MemoryRequired  uint32        `json:"memory_required,omitempty"`  // Memory pages required

	// Metadata
	Tags        []string          `json:"tags,omitempty"`
	Category    string            `json:"category,omitempty"`
	License     string            `json:"license,omitempty"`
	Repository  string            `json:"repository,omitempty"`
	CustomMeta  map[string]string `json:"custom_meta,omitempty"`

	// Registration info
	RegisteredAt time.Time `json:"registered_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// UDFParameter describes a function parameter
type UDFParameter struct {
	Name     string      `json:"name"`
	Type     ValueType   `json:"type"`
	Required bool        `json:"required"`
	Default  interface{} `json:"default,omitempty"`
	Description string   `json:"description,omitempty"`
}

// UDFReturnType describes a return value
type UDFReturnType struct {
	Name        string    `json:"name,omitempty"`
	Type        ValueType `json:"type"`
	Description string    `json:"description,omitempty"`
}

// Validate validates the UDF metadata
func (m *UDFMetadata) Validate() error {
	// Check required fields
	if m.Name == "" {
		return fmt.Errorf("UDF name is required")
	}

	if m.Version == "" {
		return fmt.Errorf("UDF version is required")
	}

	if m.FunctionName == "" {
		return fmt.Errorf("function name is required")
	}

	if len(m.WASMBytes) == 0 {
		return fmt.Errorf("WASM bytes are required")
	}

	// Validate parameters
	paramNames := make(map[string]bool)
	for i, param := range m.Parameters {
		if param.Name == "" {
			return fmt.Errorf("parameter %d: name is required", i)
		}

		// Check for duplicate parameter names
		if paramNames[param.Name] {
			return fmt.Errorf("duplicate parameter name: %s", param.Name)
		}
		paramNames[param.Name] = true

		// Validate type
		if param.Type < ValueTypeI32 || param.Type > ValueTypeBool {
			return fmt.Errorf("parameter %s: invalid type %d", param.Name, param.Type)
		}

		// Check default value if not required
		if !param.Required && param.Default != nil {
			// Validate default matches type
			if err := validateDefaultValue(param.Type, param.Default); err != nil {
				return fmt.Errorf("parameter %s: %w", param.Name, err)
			}
		}
	}

	// Validate return types
	for i, ret := range m.Returns {
		if ret.Type < ValueTypeI32 || ret.Type > ValueTypeBool {
			return fmt.Errorf("return %d: invalid type %d", i, ret.Type)
		}
	}

	return nil
}

// validateDefaultValue checks if a default value matches the expected type
func validateDefaultValue(typ ValueType, value interface{}) error {
	switch typ {
	case ValueTypeI32:
		switch value.(type) {
		case int, int32, int64, float64:
			return nil
		}
	case ValueTypeI64:
		switch value.(type) {
		case int, int32, int64, float64:
			return nil
		}
	case ValueTypeF32:
		switch value.(type) {
		case float32, float64, int, int32, int64:
			return nil
		}
	case ValueTypeF64:
		switch value.(type) {
		case float64, float32, int, int32, int64:
			return nil
		}
	case ValueTypeString:
		switch value.(type) {
		case string:
			return nil
		}
	case ValueTypeBool:
		switch value.(type) {
		case bool:
			return nil
		}
	}

	return fmt.Errorf("default value type mismatch: expected %v, got %T", typ, value)
}

// GetParameterByName retrieves a parameter by name
func (m *UDFMetadata) GetParameterByName(name string) (*UDFParameter, bool) {
	for i := range m.Parameters {
		if m.Parameters[i].Name == name {
			return &m.Parameters[i], true
		}
	}
	return nil, false
}

// HasTag checks if the UDF has a specific tag
func (m *UDFMetadata) HasTag(tag string) bool {
	for _, t := range m.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// GetFullName returns the full UDF identifier (name@version)
func (m *UDFMetadata) GetFullName() string {
	return fmt.Sprintf("%s@%s", m.Name, m.Version)
}

// Clone creates a deep copy of the metadata (excluding WASM bytes)
func (m *UDFMetadata) Clone() *UDFMetadata {
	clone := &UDFMetadata{
		Name:            m.Name,
		Version:         m.Version,
		Description:     m.Description,
		Author:          m.Author,
		FunctionName:    m.FunctionName,
		WASMSize:        m.WASMSize,
		ExpectedLatency: m.ExpectedLatency,
		MemoryRequired:  m.MemoryRequired,
		Category:        m.Category,
		License:         m.License,
		Repository:      m.Repository,
		RegisteredAt:    m.RegisteredAt,
		UpdatedAt:       m.UpdatedAt,
	}

	// Copy parameters
	clone.Parameters = make([]UDFParameter, len(m.Parameters))
	copy(clone.Parameters, m.Parameters)

	// Copy returns
	clone.Returns = make([]UDFReturnType, len(m.Returns))
	copy(clone.Returns, m.Returns)

	// Copy tags
	if len(m.Tags) > 0 {
		clone.Tags = make([]string, len(m.Tags))
		copy(clone.Tags, m.Tags)
	}

	// Copy custom metadata
	if len(m.CustomMeta) > 0 {
		clone.CustomMeta = make(map[string]string)
		for k, v := range m.CustomMeta {
			clone.CustomMeta[k] = v
		}
	}

	return clone
}

// UDFStats contains statistics about a UDF
type UDFStats struct {
	Name             string        `json:"name"`
	Version          string        `json:"version"`
	CallCount        uint64        `json:"call_count"`
	ErrorCount       uint64        `json:"error_count"`
	TotalDuration    time.Duration `json:"total_duration"`
	AverageDuration  time.Duration `json:"average_duration"`
	MinDuration      time.Duration `json:"min_duration"`
	MaxDuration      time.Duration `json:"max_duration"`
	LastCalled       time.Time     `json:"last_called"`
	LastError        string        `json:"last_error,omitempty"`
	LastErrorTime    time.Time     `json:"last_error_time,omitempty"`
}

// UpdateStats updates statistics after a UDF call
func (s *UDFStats) UpdateStats(duration time.Duration, err error) {
	s.CallCount++
	s.TotalDuration += duration
	s.AverageDuration = s.TotalDuration / time.Duration(s.CallCount)
	s.LastCalled = time.Now()

	if s.MinDuration == 0 || duration < s.MinDuration {
		s.MinDuration = duration
	}

	if duration > s.MaxDuration {
		s.MaxDuration = duration
	}

	if err != nil {
		s.ErrorCount++
		s.LastError = err.Error()
		s.LastErrorTime = time.Now()
	}
}

// ErrorRate returns the error rate as a percentage
func (s *UDFStats) ErrorRate() float64 {
	if s.CallCount == 0 {
		return 0
	}
	return float64(s.ErrorCount) / float64(s.CallCount) * 100
}

// UDFQuery represents a query for UDFs
type UDFQuery struct {
	Name     string   // Exact name match
	Version  string   // Exact version match (empty = any)
	Tags     []string // Must have all tags
	Category string   // Exact category match (empty = any)
	Author   string   // Exact author match (empty = any)
}

// Matches checks if a UDF metadata matches the query
func (q *UDFQuery) Matches(m *UDFMetadata) bool {
	// Name filter
	if q.Name != "" && m.Name != q.Name {
		return false
	}

	// Version filter
	if q.Version != "" && m.Version != q.Version {
		return false
	}

	// Category filter
	if q.Category != "" && m.Category != q.Category {
		return false
	}

	// Author filter
	if q.Author != "" && m.Author != q.Author {
		return false
	}

	// Tags filter (must have all)
	for _, tag := range q.Tags {
		if !m.HasTag(tag) {
			return false
		}
	}

	return true
}
