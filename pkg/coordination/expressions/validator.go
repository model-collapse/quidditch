package expressions

import (
	"fmt"
)

// Validator validates expression ASTs for type safety and semantic correctness
type Validator struct{}

// NewValidator creates a new expression validator
func NewValidator() *Validator {
	return &Validator{}
}

// Validate validates an expression AST
func (v *Validator) Validate(expr Expression) error {
	if expr == nil {
		return fmt.Errorf("expression is nil")
	}

	switch e := expr.(type) {
	case *ConstExpression:
		return v.validateConst(e)
	case *FieldExpression:
		return v.validateField(e)
	case *BinaryOpExpression:
		return v.validateBinaryOp(e)
	case *UnaryOpExpression:
		return v.validateUnaryOp(e)
	case *TernaryExpression:
		return v.validateTernary(e)
	case *FunctionExpression:
		return v.validateFunction(e)
	default:
		return fmt.Errorf("unknown expression type: %T", expr)
	}
}

// validateConst validates a constant expression
func (v *Validator) validateConst(expr *ConstExpression) error {
	if expr.Value == nil {
		return fmt.Errorf("constant value is nil")
	}

	// Verify value type matches declared data type
	switch expr.DataTyp {
	case DataTypeBool:
		if _, ok := expr.Value.(bool); !ok {
			return fmt.Errorf("constant declared as bool but value is %T", expr.Value)
		}
	case DataTypeInt64:
		if _, ok := expr.Value.(int64); !ok {
			return fmt.Errorf("constant declared as int64 but value is %T", expr.Value)
		}
	case DataTypeFloat64:
		if _, ok := expr.Value.(float64); !ok {
			return fmt.Errorf("constant declared as float64 but value is %T", expr.Value)
		}
	case DataTypeString:
		if _, ok := expr.Value.(string); !ok {
			return fmt.Errorf("constant declared as string but value is %T", expr.Value)
		}
	default:
		return fmt.Errorf("unknown data type: %v", expr.DataTyp)
	}

	return nil
}

// validateField validates a field expression
func (v *Validator) validateField(expr *FieldExpression) error {
	if expr.FieldPath == "" {
		return fmt.Errorf("field path is empty")
	}

	if expr.DataTyp == DataTypeUnknown {
		return fmt.Errorf("field %s has unknown data type", expr.FieldPath)
	}

	return nil
}

// validateBinaryOp validates a binary operation expression
func (v *Validator) validateBinaryOp(expr *BinaryOpExpression) error {
	// Validate operands
	if err := v.Validate(expr.Left); err != nil {
		return fmt.Errorf("left operand invalid: %w", err)
	}
	if err := v.Validate(expr.Right); err != nil {
		return fmt.Errorf("right operand invalid: %w", err)
	}

	leftType := expr.Left.DataType()
	rightType := expr.Right.DataType()

	// Type checking based on operator
	if expr.Operator.IsArithmetic() {
		// Arithmetic operators require numeric operands
		if !v.isNumeric(leftType) {
			return fmt.Errorf("left operand of %s must be numeric, got %s", expr.Operator, leftType)
		}
		if !v.isNumeric(rightType) {
			return fmt.Errorf("right operand of %s must be numeric, got %s", expr.Operator, rightType)
		}

		// Result type should be numeric
		if !v.isNumeric(expr.DataTyp) {
			return fmt.Errorf("arithmetic operation %s should return numeric type, got %s", expr.Operator, expr.DataTyp)
		}
	}

	if expr.Operator.IsComparison() {
		// Comparison operators require compatible operand types
		if !v.isCompatible(leftType, rightType) {
			return fmt.Errorf("cannot compare %s with %s using %s", leftType, rightType, expr.Operator)
		}

		// Result type should be bool
		if expr.DataTyp != DataTypeBool {
			return fmt.Errorf("comparison operation %s should return bool, got %s", expr.Operator, expr.DataTyp)
		}
	}

	if expr.Operator.IsLogical() {
		// Logical operators require bool operands
		if leftType != DataTypeBool {
			return fmt.Errorf("left operand of %s must be bool, got %s", expr.Operator, leftType)
		}
		if rightType != DataTypeBool {
			return fmt.Errorf("right operand of %s must be bool, got %s", expr.Operator, rightType)
		}

		// Result type should be bool
		if expr.DataTyp != DataTypeBool {
			return fmt.Errorf("logical operation %s should return bool, got %s", expr.Operator, expr.DataTyp)
		}
	}

	return nil
}

// validateUnaryOp validates a unary operation expression
func (v *Validator) validateUnaryOp(expr *UnaryOpExpression) error {
	// Validate operand
	if err := v.Validate(expr.Operand); err != nil {
		return fmt.Errorf("operand invalid: %w", err)
	}

	operandType := expr.Operand.DataType()

	switch expr.Operator {
	case OpNegate:
		// Negation requires numeric operand
		if !v.isNumeric(operandType) {
			return fmt.Errorf("operand of negation must be numeric, got %s", operandType)
		}
		if !v.isNumeric(expr.DataTyp) {
			return fmt.Errorf("negation should return numeric type, got %s", expr.DataTyp)
		}

	case OpNot:
		// Logical NOT requires bool operand
		if operandType != DataTypeBool {
			return fmt.Errorf("operand of logical NOT must be bool, got %s", operandType)
		}
		if expr.DataTyp != DataTypeBool {
			return fmt.Errorf("logical NOT should return bool, got %s", expr.DataTyp)
		}
	}

	return nil
}

// validateTernary validates a ternary expression
func (v *Validator) validateTernary(expr *TernaryExpression) error {
	// Validate condition
	if err := v.Validate(expr.Condition); err != nil {
		return fmt.Errorf("condition invalid: %w", err)
	}

	// Condition must be bool
	if expr.Condition.DataType() != DataTypeBool {
		return fmt.Errorf("ternary condition must be bool, got %s", expr.Condition.DataType())
	}

	// Validate branches
	if err := v.Validate(expr.TrueValue); err != nil {
		return fmt.Errorf("true branch invalid: %w", err)
	}
	if err := v.Validate(expr.FalseValue); err != nil {
		return fmt.Errorf("false branch invalid: %w", err)
	}

	// Branches should have compatible types
	trueType := expr.TrueValue.DataType()
	falseType := expr.FalseValue.DataType()

	if !v.isCompatible(trueType, falseType) {
		return fmt.Errorf("ternary branches have incompatible types: %s vs %s", trueType, falseType)
	}

	return nil
}

// validateFunction validates a function expression
func (v *Validator) validateFunction(expr *FunctionExpression) error {
	// Validate all arguments
	for i, arg := range expr.Args {
		if err := v.Validate(arg); err != nil {
			return fmt.Errorf("argument %d invalid: %w", i, err)
		}
	}

	// Function-specific validation
	switch expr.Function {
	case FuncAbs, FuncSqrt, FuncLog, FuncLog10, FuncExp:
		// Single numeric argument
		if len(expr.Args) != 1 {
			return fmt.Errorf("%s requires exactly 1 argument, got %d", expr.Function, len(expr.Args))
		}
		if !v.isNumeric(expr.Args[0].DataType()) {
			return fmt.Errorf("%s requires numeric argument, got %s", expr.Function, expr.Args[0].DataType())
		}

	case FuncFloor, FuncCeil, FuncRound:
		// Single numeric argument, returns int
		if len(expr.Args) != 1 {
			return fmt.Errorf("%s requires exactly 1 argument, got %d", expr.Function, len(expr.Args))
		}
		if !v.isNumeric(expr.Args[0].DataType()) {
			return fmt.Errorf("%s requires numeric argument, got %s", expr.Function, expr.Args[0].DataType())
		}
		if expr.DataTyp != DataTypeInt64 {
			return fmt.Errorf("%s should return int64, got %s", expr.Function, expr.DataTyp)
		}

	case FuncPow:
		// Two numeric arguments
		if len(expr.Args) != 2 {
			return fmt.Errorf("pow requires exactly 2 arguments, got %d", len(expr.Args))
		}
		if !v.isNumeric(expr.Args[0].DataType()) {
			return fmt.Errorf("pow base must be numeric, got %s", expr.Args[0].DataType())
		}
		if !v.isNumeric(expr.Args[1].DataType()) {
			return fmt.Errorf("pow exponent must be numeric, got %s", expr.Args[1].DataType())
		}

	case FuncMin, FuncMax:
		// At least 2 numeric arguments
		if len(expr.Args) < 2 {
			return fmt.Errorf("%s requires at least 2 arguments, got %d", expr.Function, len(expr.Args))
		}
		for i, arg := range expr.Args {
			if !v.isNumeric(arg.DataType()) {
				return fmt.Errorf("%s argument %d must be numeric, got %s", expr.Function, i, arg.DataType())
			}
		}

	case FuncSin, FuncCos, FuncTan:
		// Single numeric argument
		if len(expr.Args) != 1 {
			return fmt.Errorf("%s requires exactly 1 argument, got %d", expr.Function, len(expr.Args))
		}
		if !v.isNumeric(expr.Args[0].DataType()) {
			return fmt.Errorf("%s requires numeric argument, got %s", expr.Function, expr.Args[0].DataType())
		}
	}

	// Result type validation
	if !v.isNumeric(expr.DataTyp) && expr.DataTyp != DataTypeInt64 {
		return fmt.Errorf("function %s should return numeric type, got %s", expr.Function, expr.DataTyp)
	}

	return nil
}

// Helper functions

// isNumeric returns true if the type is numeric
func (v *Validator) isNumeric(dataType DataType) bool {
	return dataType == DataTypeInt64 || dataType == DataTypeFloat64
}

// isCompatible returns true if two types are compatible for operations
func (v *Validator) isCompatible(type1, type2 DataType) bool {
	// Same types are always compatible
	if type1 == type2 {
		return true
	}

	// Numeric types are compatible with each other
	if v.isNumeric(type1) && v.isNumeric(type2) {
		return true
	}

	return false
}
