package expressions

import (
	"testing"
)

func TestValidateConst(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		expr    *ConstExpression
		wantErr bool
	}{
		{
			name:    "valid bool",
			expr:    NewConstBool(true),
			wantErr: false,
		},
		{
			name:    "valid int",
			expr:    NewConstInt(42),
			wantErr: false,
		},
		{
			name:    "valid float",
			expr:    NewConstFloat(3.14),
			wantErr: false,
		},
		{
			name:    "valid string",
			expr:    NewConstString("hello"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateField(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		expr    *FieldExpression
		wantErr bool
	}{
		{
			name:    "valid field",
			expr:    NewField("price", DataTypeFloat64),
			wantErr: false,
		},
		{
			name:    "valid nested field",
			expr:    NewField("metadata.category", DataTypeString),
			wantErr: false,
		},
		{
			name:    "empty field path",
			expr:    &FieldExpression{FieldPath: "", DataTyp: DataTypeFloat64},
			wantErr: true,
		},
		{
			name:    "unknown data type",
			expr:    &FieldExpression{FieldPath: "price", DataTyp: DataTypeUnknown},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateBinaryOpArithmetic(t *testing.T) {
	validator := NewValidator()

	// Valid: 10 + 20
	validExpr := NewBinaryOp(
		OpAdd,
		NewConstInt(10),
		NewConstInt(20),
		DataTypeInt64,
	)

	if err := validator.Validate(validExpr); err != nil {
		t.Errorf("Valid arithmetic expression failed: %v", err)
	}

	// Invalid: "hello" + 20 (string arithmetic)
	invalidExpr := NewBinaryOp(
		OpAdd,
		NewConstString("hello"),
		NewConstInt(20),
		DataTypeFloat64,
	)

	if err := validator.Validate(invalidExpr); err == nil {
		t.Error("Expected error for string arithmetic, got nil")
	}

	// Invalid: arithmetic result as bool
	invalidResult := NewBinaryOp(
		OpAdd,
		NewConstInt(10),
		NewConstInt(20),
		DataTypeBool, // Wrong result type
	)

	if err := validator.Validate(invalidResult); err == nil {
		t.Error("Expected error for arithmetic with bool result, got nil")
	}
}

func TestValidateBinaryOpComparison(t *testing.T) {
	validator := NewValidator()

	// Valid: 10 > 5
	validExpr := NewBinaryOp(
		OpGreaterThan,
		NewConstInt(10),
		NewConstInt(5),
		DataTypeBool,
	)

	if err := validator.Validate(validExpr); err != nil {
		t.Errorf("Valid comparison expression failed: %v", err)
	}

	// Invalid: comparison result not bool
	invalidResult := NewBinaryOp(
		OpGreaterThan,
		NewConstInt(10),
		NewConstInt(5),
		DataTypeInt64, // Wrong result type
	)

	if err := validator.Validate(invalidResult); err == nil {
		t.Error("Expected error for comparison with non-bool result, got nil")
	}

	// Valid: string comparison
	validStrComp := NewBinaryOp(
		OpEqual,
		NewConstString("hello"),
		NewConstString("world"),
		DataTypeBool,
	)

	if err := validator.Validate(validStrComp); err != nil {
		t.Errorf("Valid string comparison failed: %v", err)
	}
}

func TestValidateBinaryOpLogical(t *testing.T) {
	validator := NewValidator()

	// Valid: true && false
	validExpr := NewBinaryOp(
		OpAnd,
		NewConstBool(true),
		NewConstBool(false),
		DataTypeBool,
	)

	if err := validator.Validate(validExpr); err != nil {
		t.Errorf("Valid logical expression failed: %v", err)
	}

	// Invalid: 10 && 20 (non-bool operands)
	invalidOperands := NewBinaryOp(
		OpAnd,
		NewConstInt(10),
		NewConstInt(20),
		DataTypeBool,
	)

	if err := validator.Validate(invalidOperands); err == nil {
		t.Error("Expected error for logical op with non-bool operands, got nil")
	}

	// Invalid: logical result not bool
	invalidResult := NewBinaryOp(
		OpAnd,
		NewConstBool(true),
		NewConstBool(false),
		DataTypeInt64, // Wrong result type
	)

	if err := validator.Validate(invalidResult); err == nil {
		t.Error("Expected error for logical op with non-bool result, got nil")
	}
}

func TestValidateUnaryOp(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		expr    *UnaryOpExpression
		wantErr bool
	}{
		{
			name: "valid negation",
			expr: NewUnaryOp(
				OpNegate,
				NewConstInt(42),
				DataTypeInt64,
			),
			wantErr: false,
		},
		{
			name: "valid logical not",
			expr: NewUnaryOp(
				OpNot,
				NewConstBool(true),
				DataTypeBool,
			),
			wantErr: false,
		},
		{
			name: "invalid negation of bool",
			expr: NewUnaryOp(
				OpNegate,
				NewConstBool(true),
				DataTypeBool,
			),
			wantErr: true,
		},
		{
			name: "invalid not of number",
			expr: NewUnaryOp(
				OpNot,
				NewConstInt(42),
				DataTypeBool,
			),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTernary(t *testing.T) {
	validator := NewValidator()

	// Valid: true ? 10 : 20
	validExpr := NewTernary(
		NewConstBool(true),
		NewConstInt(10),
		NewConstInt(20),
		DataTypeInt64,
	)

	if err := validator.Validate(validExpr); err != nil {
		t.Errorf("Valid ternary expression failed: %v", err)
	}

	// Invalid: non-bool condition
	invalidCond := NewTernary(
		NewConstInt(42), // Not a bool
		NewConstInt(10),
		NewConstInt(20),
		DataTypeInt64,
	)

	if err := validator.Validate(invalidCond); err == nil {
		t.Error("Expected error for ternary with non-bool condition, got nil")
	}

	// Invalid: incompatible branches
	incompatibleBranches := NewTernary(
		NewConstBool(true),
		NewConstInt(10),      // int
		NewConstString("hi"), // string
		DataTypeInt64,
	)

	if err := validator.Validate(incompatibleBranches); err == nil {
		t.Error("Expected error for ternary with incompatible branches, got nil")
	}
}

func TestValidateFunction(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name    string
		expr    *FunctionExpression
		wantErr bool
	}{
		{
			name: "valid abs",
			expr: NewFunction(
				FuncAbs,
				[]Expression{NewConstInt(-42)},
				DataTypeFloat64,
			),
			wantErr: false,
		},
		{
			name: "valid min",
			expr: NewFunction(
				FuncMin,
				[]Expression{
					NewConstInt(10),
					NewConstInt(20),
					NewConstInt(5),
				},
				DataTypeInt64,
			),
			wantErr: false,
		},
		{
			name: "valid pow",
			expr: NewFunction(
				FuncPow,
				[]Expression{
					NewConstFloat(2.0),
					NewConstFloat(3.0),
				},
				DataTypeFloat64,
			),
			wantErr: false,
		},
		{
			name: "invalid abs - too many args",
			expr: NewFunction(
				FuncAbs,
				[]Expression{
					NewConstInt(10),
					NewConstInt(20),
				},
				DataTypeFloat64,
			),
			wantErr: true,
		},
		{
			name: "invalid abs - non-numeric arg",
			expr: NewFunction(
				FuncAbs,
				[]Expression{NewConstString("hello")},
				DataTypeFloat64,
			),
			wantErr: true,
		},
		{
			name: "invalid min - too few args",
			expr: NewFunction(
				FuncMin,
				[]Expression{NewConstInt(10)},
				DataTypeInt64,
			),
			wantErr: true,
		},
		{
			name: "invalid floor - wrong return type",
			expr: NewFunction(
				FuncFloor,
				[]Expression{NewConstFloat(3.7)},
				DataTypeFloat64, // Should be int64
			),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.expr)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateComplexExpression(t *testing.T) {
	validator := NewValidator()

	// Valid: (price * 1.2) > 100
	complexExpr := NewBinaryOp(
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

	if err := validator.Validate(complexExpr); err != nil {
		t.Errorf("Valid complex expression failed: %v", err)
	}

	// Valid: (price > 100) && (price < 1000)
	logicalExpr := NewBinaryOp(
		OpAnd,
		NewBinaryOp(
			OpGreaterThan,
			NewField("price", DataTypeFloat64),
			NewConstFloat(100.0),
			DataTypeBool,
		),
		NewBinaryOp(
			OpLessThan,
			NewField("price", DataTypeFloat64),
			NewConstFloat(1000.0),
			DataTypeBool,
		),
		DataTypeBool,
	)

	if err := validator.Validate(logicalExpr); err != nil {
		t.Errorf("Valid logical expression failed: %v", err)
	}
}
