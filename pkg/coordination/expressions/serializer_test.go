package expressions

import (
	"bytes"
	"testing"
)

func TestSerializeConst(t *testing.T) {
	serializer := NewSerializer()

	tests := []struct {
		name string
		expr *ConstExpression
	}{
		{
			name: "bool constant",
			expr: NewConstBool(true),
		},
		{
			name: "int constant",
			expr: NewConstInt(42),
		},
		{
			name: "float constant",
			expr: NewConstFloat(3.14),
		},
		{
			name: "string constant",
			expr: NewConstString("hello"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := serializer.Serialize(tt.expr)
			if err != nil {
				t.Fatalf("Serialize failed: %v", err)
			}

			if len(data) == 0 {
				t.Error("Serialized data is empty")
			}

			// Verify first byte is expression type
			if data[0] != byte(ExprTypeConst) {
				t.Errorf("Expected expression type %d, got %d", ExprTypeConst, data[0])
			}
		})
	}
}

func TestSerializeField(t *testing.T) {
	serializer := NewSerializer()

	expr := NewField("price", DataTypeFloat64)

	data, err := serializer.Serialize(expr)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data is empty")
	}

	// Verify first byte is expression type
	if data[0] != byte(ExprTypeField) {
		t.Errorf("Expected expression type %d, got %d", ExprTypeField, data[0])
	}

	// Verify data type byte
	if data[1] != byte(DataTypeFloat64) {
		t.Errorf("Expected data type %d, got %d", DataTypeFloat64, data[1])
	}
}

func TestSerializeBinaryOp(t *testing.T) {
	serializer := NewSerializer()

	// 10 + 20
	expr := NewBinaryOp(
		OpAdd,
		NewConstInt(10),
		NewConstInt(20),
		DataTypeInt64,
	)

	data, err := serializer.Serialize(expr)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data is empty")
	}

	// Verify first byte is expression type
	if data[0] != byte(ExprTypeBinaryOp) {
		t.Errorf("Expected expression type %d, got %d", ExprTypeBinaryOp, data[0])
	}

	// Verify operator byte
	if data[1] != byte(OpAdd) {
		t.Errorf("Expected operator %d, got %d", OpAdd, data[1])
	}

	// Verify result type
	if data[2] != byte(DataTypeInt64) {
		t.Errorf("Expected result type %d, got %d", DataTypeInt64, data[2])
	}
}

func TestSerializeUnaryOp(t *testing.T) {
	serializer := NewSerializer()

	// -42
	expr := NewUnaryOp(
		OpNegate,
		NewConstInt(42),
		DataTypeInt64,
	)

	data, err := serializer.Serialize(expr)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data is empty")
	}

	// Verify first byte is expression type
	if data[0] != byte(ExprTypeUnaryOp) {
		t.Errorf("Expected expression type %d, got %d", ExprTypeUnaryOp, data[0])
	}

	// Verify operator byte
	if data[1] != byte(OpNegate) {
		t.Errorf("Expected operator %d, got %d", OpNegate, data[1])
	}
}

func TestSerializeTernary(t *testing.T) {
	serializer := NewSerializer()

	// true ? 10 : 20
	expr := NewTernary(
		NewConstBool(true),
		NewConstInt(10),
		NewConstInt(20),
		DataTypeInt64,
	)

	data, err := serializer.Serialize(expr)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data is empty")
	}

	// Verify first byte is expression type
	if data[0] != byte(ExprTypeTernary) {
		t.Errorf("Expected expression type %d, got %d", ExprTypeTernary, data[0])
	}

	// Verify result type
	if data[1] != byte(DataTypeInt64) {
		t.Errorf("Expected result type %d, got %d", DataTypeInt64, data[1])
	}
}

func TestSerializeFunction(t *testing.T) {
	serializer := NewSerializer()

	// abs(-42)
	expr := NewFunction(
		FuncAbs,
		[]Expression{NewConstInt(-42)},
		DataTypeFloat64,
	)

	data, err := serializer.Serialize(expr)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data is empty")
	}

	// Verify first byte is expression type
	if data[0] != byte(ExprTypeFunction) {
		t.Errorf("Expected expression type %d, got %d", ExprTypeFunction, data[0])
	}

	// Verify function byte
	if data[1] != byte(FuncAbs) {
		t.Errorf("Expected function %d, got %d", FuncAbs, data[1])
	}

	// Verify result type
	if data[2] != byte(DataTypeFloat64) {
		t.Errorf("Expected result type %d, got %d", DataTypeFloat64, data[2])
	}
}

func TestSerializeComplexExpression(t *testing.T) {
	serializer := NewSerializer()

	// (price * 1.2) > 100
	expr := NewBinaryOp(
		OpGreaterThan,
		NewBinaryOp(
			OpMultiply,
			NewField("price", DataTypeFloat64),
			NewConstFloat(1.2),
			DataTypeFloat64,
		),
		NewConstFloat(100.0),
		DataTypeBool,
	)

	data, err := serializer.Serialize(expr)
	if err != nil {
		t.Fatalf("Serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data is empty")
	}

	// Basic sanity check - should have reasonable size
	if len(data) < 10 {
		t.Errorf("Serialized data seems too small: %d bytes", len(data))
	}
}

func TestSerializeAndValidate(t *testing.T) {
	serializer := NewSerializer()
	validator := NewValidator()

	// Create expression
	expr := NewBinaryOp(
		OpAdd,
		NewConstInt(10),
		NewConstInt(20),
		DataTypeInt64,
	)

	// Validate first
	if err := validator.Validate(expr); err != nil {
		t.Fatalf("Validation failed: %v", err)
	}

	// Then serialize
	data, err := serializer.Serialize(expr)
	if err != nil {
		t.Fatalf("Serialization failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("Serialized data is empty")
	}
}

func TestSerializeMultipleTimes(t *testing.T) {
	serializer := NewSerializer()

	expr := NewConstInt(42)

	// Serialize multiple times - should produce identical output
	data1, err1 := serializer.Serialize(expr)
	if err1 != nil {
		t.Fatalf("First serialization failed: %v", err1)
	}

	data2, err2 := serializer.Serialize(expr)
	if err2 != nil {
		t.Fatalf("Second serialization failed: %v", err2)
	}

	if !bytes.Equal(data1, data2) {
		t.Error("Multiple serializations produced different output")
	}
}

func TestSerializeDifferentExpressions(t *testing.T) {
	serializer := NewSerializer()

	expr1 := NewConstInt(42)
	expr2 := NewConstInt(99)

	data1, _ := serializer.Serialize(expr1)
	data2, _ := serializer.Serialize(expr2)

	// Should produce different output
	if bytes.Equal(data1, data2) {
		t.Error("Different expressions produced identical serialization")
	}
}

func BenchmarkSerializeConst(b *testing.B) {
	serializer := NewSerializer()
	expr := NewConstInt(42)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = serializer.Serialize(expr)
	}
}

func BenchmarkSerializeComplexExpression(b *testing.B) {
	serializer := NewSerializer()

	// (price * 1.2) > 100
	expr := NewBinaryOp(
		OpGreaterThan,
		NewBinaryOp(
			OpMultiply,
			NewField("price", DataTypeFloat64),
			NewConstFloat(1.2),
			DataTypeFloat64,
		),
		NewConstFloat(100.0),
		DataTypeBool,
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = serializer.Serialize(expr)
	}
}
