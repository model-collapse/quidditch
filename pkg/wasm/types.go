package wasm

import (
	"fmt"

	"github.com/tetratelabs/wazero/api"
)

// ValueType represents WASM value types
type ValueType int

const (
	ValueTypeI32 ValueType = iota
	ValueTypeI64
	ValueTypeF32
	ValueTypeF64
	ValueTypeString
	ValueTypeBool
)

// Value represents a typed value that can be passed to/from WASM
type Value struct {
	Type ValueType
	Data interface{}
}

// NewI32Value creates an i32 value
func NewI32Value(v int32) Value {
	return Value{Type: ValueTypeI32, Data: v}
}

// NewI64Value creates an i64 value
func NewI64Value(v int64) Value {
	return Value{Type: ValueTypeI64, Data: v}
}

// NewF32Value creates an f32 value
func NewF32Value(v float32) Value {
	return Value{Type: ValueTypeF32, Data: v}
}

// NewF64Value creates an f64 value
func NewF64Value(v float64) Value {
	return Value{Type: ValueTypeF64, Data: v}
}

// NewStringValue creates a string value
func NewStringValue(v string) Value {
	return Value{Type: ValueTypeString, Data: v}
}

// NewBoolValue creates a bool value
func NewBoolValue(v bool) Value {
	return Value{Type: ValueTypeBool, Data: v}
}

// ToUint64 converts the value to uint64 for WASM
func (v Value) ToUint64() (uint64, error) {
	switch v.Type {
	case ValueTypeI32:
		if val, ok := v.Data.(int32); ok {
			return uint64(uint32(val)), nil
		}
		return 0, fmt.Errorf("invalid i32 value")
	case ValueTypeI64:
		if val, ok := v.Data.(int64); ok {
			return uint64(val), nil
		}
		return 0, fmt.Errorf("invalid i64 value")
	case ValueTypeF32:
		if val, ok := v.Data.(float32); ok {
			return uint64(api.EncodeF32(val)), nil
		}
		return 0, fmt.Errorf("invalid f32 value")
	case ValueTypeF64:
		if val, ok := v.Data.(float64); ok {
			return api.EncodeF64(val), nil
		}
		return 0, fmt.Errorf("invalid f64 value")
	case ValueTypeBool:
		if val, ok := v.Data.(bool); ok {
			if val {
				return 1, nil
			}
			return 0, nil
		}
		return 0, fmt.Errorf("invalid bool value")
	default:
		return 0, fmt.Errorf("unsupported value type for uint64 conversion: %v", v.Type)
	}
}

// FromUint64 creates a value from uint64 WASM result
func FromUint64(val uint64, typ ValueType) (Value, error) {
	switch typ {
	case ValueTypeI32:
		return Value{Type: ValueTypeI32, Data: int32(uint32(val))}, nil
	case ValueTypeI64:
		return Value{Type: ValueTypeI64, Data: int64(val)}, nil
	case ValueTypeF32:
		return Value{Type: ValueTypeF32, Data: api.DecodeF32(val)}, nil
	case ValueTypeF64:
		return Value{Type: ValueTypeF64, Data: api.DecodeF64(val)}, nil
	case ValueTypeBool:
		return Value{Type: ValueTypeBool, Data: val != 0}, nil
	default:
		return Value{}, fmt.Errorf("unsupported value type: %v", typ)
	}
}

// AsInt32 returns the value as int32
func (v Value) AsInt32() (int32, error) {
	if v.Type != ValueTypeI32 {
		return 0, fmt.Errorf("value is not i32")
	}
	if val, ok := v.Data.(int32); ok {
		return val, nil
	}
	return 0, fmt.Errorf("invalid i32 data")
}

// AsInt64 returns the value as int64
func (v Value) AsInt64() (int64, error) {
	if v.Type != ValueTypeI64 {
		return 0, fmt.Errorf("value is not i64")
	}
	if val, ok := v.Data.(int64); ok {
		return val, nil
	}
	return 0, fmt.Errorf("invalid i64 data")
}

// AsFloat32 returns the value as float32
func (v Value) AsFloat32() (float32, error) {
	if v.Type != ValueTypeF32 {
		return 0, fmt.Errorf("value is not f32")
	}
	if val, ok := v.Data.(float32); ok {
		return val, nil
	}
	return 0, fmt.Errorf("invalid f32 data")
}

// AsFloat64 returns the value as float64
func (v Value) AsFloat64() (float64, error) {
	if v.Type != ValueTypeF64 {
		return 0, fmt.Errorf("value is not f64")
	}
	if val, ok := v.Data.(float64); ok {
		return val, nil
	}
	return 0, fmt.Errorf("invalid f64 data")
}

// AsBool returns the value as bool
func (v Value) AsBool() (bool, error) {
	if v.Type != ValueTypeBool {
		return false, fmt.Errorf("value is not bool")
	}
	if val, ok := v.Data.(bool); ok {
		return val, nil
	}
	return false, fmt.Errorf("invalid bool data")
}

// AsString returns the value as string
func (v Value) AsString() (string, error) {
	if v.Type != ValueTypeString {
		return "", fmt.Errorf("value is not string")
	}
	if val, ok := v.Data.(string); ok {
		return val, nil
	}
	return "", fmt.Errorf("invalid string data")
}

// Parameter represents a function parameter
type Parameter struct {
	Name     string
	Type     ValueType
	Required bool
	Default  interface{}
}

// Result represents a function result type
type Result struct {
	Type ValueType
}

// FunctionSignature describes a WASM function signature
type FunctionSignature struct {
	Name       string
	Parameters []Parameter
	Results    []Result
}

// Validate validates that provided values match the signature
func (fs *FunctionSignature) Validate(values map[string]Value) error {
	for _, param := range fs.Parameters {
		val, exists := values[param.Name]
		if !exists {
			if param.Required {
				return fmt.Errorf("missing required parameter: %s", param.Name)
			}
			continue
		}

		if val.Type != param.Type {
			return fmt.Errorf("parameter %s: expected type %v, got %v",
				param.Name, param.Type, val.Type)
		}
	}

	return nil
}
