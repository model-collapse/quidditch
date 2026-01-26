package expressions

import (
	"testing"
)

func TestParseConst(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		input    map[string]interface{}
		expected DataType
		value    interface{}
	}{
		{
			name:     "bool constant",
			input:    map[string]interface{}{"const": true},
			expected: DataTypeBool,
			value:    true,
		},
		{
			name:     "int constant",
			input:    map[string]interface{}{"const": int64(42)},
			expected: DataTypeInt64,
			value:    int64(42),
		},
		{
			name:     "float constant",
			input:    map[string]interface{}{"const": 3.14},
			expected: DataTypeFloat64,
			value:    3.14,
		},
		{
			name:     "string constant",
			input:    map[string]interface{}{"const": "hello"},
			expected: DataTypeString,
			value:    "hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			constExpr, ok := expr.(*ConstExpression)
			if !ok {
				t.Fatalf("Expected ConstExpression, got %T", expr)
			}

			if constExpr.DataType() != tt.expected {
				t.Errorf("Expected data type %v, got %v", tt.expected, constExpr.DataType())
			}

			if constExpr.Value != tt.value {
				t.Errorf("Expected value %v, got %v", tt.value, constExpr.Value)
			}
		})
	}
}

func TestParseField(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		input    map[string]interface{}
		expected string
		dataType DataType
	}{
		{
			name:     "simple field",
			input:    map[string]interface{}{"field": "price"},
			expected: "price",
			dataType: DataTypeFloat64, // Default
		},
		{
			name:     "field with type",
			input:    map[string]interface{}{"field": "active", "type": "bool"},
			expected: "active",
			dataType: DataTypeBool,
		},
		{
			name:     "nested field",
			input:    map[string]interface{}{"field": "metadata.category"},
			expected: "metadata.category",
			dataType: DataTypeFloat64,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			fieldExpr, ok := expr.(*FieldExpression)
			if !ok {
				t.Fatalf("Expected FieldExpression, got %T", expr)
			}

			if fieldExpr.FieldPath != tt.expected {
				t.Errorf("Expected field path %s, got %s", tt.expected, fieldExpr.FieldPath)
			}

			if fieldExpr.DataType() != tt.dataType {
				t.Errorf("Expected data type %v, got %v", tt.dataType, fieldExpr.DataType())
			}
		})
	}
}

func TestParseBinaryOp(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		input    map[string]interface{}
		operator BinaryOperator
		resType  DataType
	}{
		{
			name: "addition",
			input: map[string]interface{}{
				"op":    "+",
				"left":  map[string]interface{}{"const": 1.0},
				"right": map[string]interface{}{"const": 2.0},
			},
			operator: OpAdd,
			resType:  DataTypeFloat64,
		},
		{
			name: "comparison",
			input: map[string]interface{}{
				"op":    ">",
				"left":  map[string]interface{}{"field": "price"},
				"right": map[string]interface{}{"const": 100.0},
			},
			operator: OpGreaterThan,
			resType:  DataTypeBool,
		},
		{
			name: "logical and",
			input: map[string]interface{}{
				"op": "&&",
				"left": map[string]interface{}{
					"op":    ">",
					"left":  map[string]interface{}{"field": "price"},
					"right": map[string]interface{}{"const": 100.0},
				},
				"right": map[string]interface{}{
					"op":    "<",
					"left":  map[string]interface{}{"field": "price"},
					"right": map[string]interface{}{"const": 1000.0},
				},
			},
			operator: OpAnd,
			resType:  DataTypeBool,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			binOpExpr, ok := expr.(*BinaryOpExpression)
			if !ok {
				t.Fatalf("Expected BinaryOpExpression, got %T", expr)
			}

			if binOpExpr.Operator != tt.operator {
				t.Errorf("Expected operator %v, got %v", tt.operator, binOpExpr.Operator)
			}

			if binOpExpr.DataType() != tt.resType {
				t.Errorf("Expected result type %v, got %v", tt.resType, binOpExpr.DataType())
			}
		})
	}
}

func TestParseUnaryOp(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		input    map[string]interface{}
		operator UnaryOperator
	}{
		{
			name: "negation",
			input: map[string]interface{}{
				"op":      "-",
				"operand": map[string]interface{}{"const": 42.0},
			},
			operator: OpNegate,
		},
		{
			name: "logical not",
			input: map[string]interface{}{
				"op":      "!",
				"operand": map[string]interface{}{"const": true},
			},
			operator: OpNot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			unOpExpr, ok := expr.(*UnaryOpExpression)
			if !ok {
				t.Fatalf("Expected UnaryOpExpression, got %T", expr)
			}

			if unOpExpr.Operator != tt.operator {
				t.Errorf("Expected operator %v, got %v", tt.operator, unOpExpr.Operator)
			}
		})
	}
}

func TestParseFunction(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name     string
		input    map[string]interface{}
		function FunctionName
		argCount int
	}{
		{
			name: "abs function",
			input: map[string]interface{}{
				"func": "abs",
				"args": []interface{}{
					map[string]interface{}{"const": -42.0},
				},
			},
			function: FuncAbs,
			argCount: 1,
		},
		{
			name: "min function",
			input: map[string]interface{}{
				"func": "min",
				"args": []interface{}{
					map[string]interface{}{"const": 10.0},
					map[string]interface{}{"const": 20.0},
					map[string]interface{}{"const": 5.0},
				},
			},
			function: FuncMin,
			argCount: 3,
		},
		{
			name: "pow function",
			input: map[string]interface{}{
				"func": "pow",
				"args": []interface{}{
					map[string]interface{}{"const": 2.0},
					map[string]interface{}{"const": 3.0},
				},
			},
			function: FuncPow,
			argCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := parser.Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse failed: %v", err)
			}

			funcExpr, ok := expr.(*FunctionExpression)
			if !ok {
				t.Fatalf("Expected FunctionExpression, got %T", expr)
			}

			if funcExpr.Function != tt.function {
				t.Errorf("Expected function %v, got %v", tt.function, funcExpr.Function)
			}

			if len(funcExpr.Args) != tt.argCount {
				t.Errorf("Expected %d arguments, got %d", tt.argCount, len(funcExpr.Args))
			}
		})
	}
}

func TestParseComplexExpression(t *testing.T) {
	parser := NewParser()

	// (price * 1.2) > 100
	input := map[string]interface{}{
		"op": ">",
		"left": map[string]interface{}{
			"op":    "*",
			"left":  map[string]interface{}{"field": "price"},
			"right": map[string]interface{}{"const": 1.2},
		},
		"right": map[string]interface{}{"const": 100.0},
	}

	expr, err := parser.Parse(input)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify structure
	binOp, ok := expr.(*BinaryOpExpression)
	if !ok {
		t.Fatalf("Expected BinaryOpExpression at root, got %T", expr)
	}

	if binOp.Operator != OpGreaterThan {
		t.Errorf("Expected > operator, got %v", binOp.Operator)
	}

	// Verify left side is multiplication
	leftMul, ok := binOp.Left.(*BinaryOpExpression)
	if !ok {
		t.Fatalf("Expected BinaryOpExpression on left, got %T", binOp.Left)
	}

	if leftMul.Operator != OpMultiply {
		t.Errorf("Expected * operator on left, got %v", leftMul.Operator)
	}
}

func TestParseErrors(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		name  string
		input map[string]interface{}
	}{
		{
			name:  "nil map",
			input: nil,
		},
		{
			name:  "empty map",
			input: map[string]interface{}{},
		},
		{
			name: "unknown operator",
			input: map[string]interface{}{
				"op":    "???",
				"left":  map[string]interface{}{"const": 1.0},
				"right": map[string]interface{}{"const": 2.0},
			},
		},
		{
			name: "binary op missing left",
			input: map[string]interface{}{
				"op":    "+",
				"right": map[string]interface{}{"const": 2.0},
			},
		},
		{
			name: "unknown function",
			input: map[string]interface{}{
				"func": "unknown_func",
				"args": []interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parser.Parse(tt.input)
			if err == nil {
				t.Error("Expected error, got nil")
			}
		})
	}
}
