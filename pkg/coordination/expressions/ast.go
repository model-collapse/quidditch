package expressions

import (
	"fmt"
)

// ExpressionType defines the type of expression node
type ExpressionType int

const (
	ExprTypeUnknown ExpressionType = iota
	ExprTypeConst                  // Constant value
	ExprTypeField                  // Document field access
	ExprTypeBinaryOp               // Binary operation (+, -, *, /, ==, <, etc.)
	ExprTypeUnaryOp                // Unary operation (-, !)
	ExprTypeTernary                // Ternary operation (condition ? true : false)
	ExprTypeFunction               // Function call (abs, sqrt, min, max, etc.)
)

// DataType defines the data type of a value
type DataType int

const (
	DataTypeUnknown DataType = iota
	DataTypeBool
	DataTypeInt64
	DataTypeFloat64
	DataTypeString
)

func (dt DataType) String() string {
	switch dt {
	case DataTypeBool:
		return "bool"
	case DataTypeInt64:
		return "int64"
	case DataTypeFloat64:
		return "float64"
	case DataTypeString:
		return "string"
	default:
		return "unknown"
	}
}

// BinaryOperator defines binary operators
type BinaryOperator int

const (
	OpUnknown BinaryOperator = iota

	// Arithmetic operators
	OpAdd      // +
	OpSubtract // -
	OpMultiply // *
	OpDivide   // /
	OpModulo   // %
	OpPower    // pow(x, y) or **

	// Comparison operators
	OpEqual        // ==
	OpNotEqual     // !=
	OpLessThan     // <
	OpLessEqual    // <=
	OpGreaterThan  // >
	OpGreaterEqual // >=

	// Logical operators
	OpAnd // &&
	OpOr  // ||
)

func (op BinaryOperator) String() string {
	switch op {
	case OpAdd:
		return "+"
	case OpSubtract:
		return "-"
	case OpMultiply:
		return "*"
	case OpDivide:
		return "/"
	case OpModulo:
		return "%"
	case OpPower:
		return "pow"
	case OpEqual:
		return "=="
	case OpNotEqual:
		return "!="
	case OpLessThan:
		return "<"
	case OpLessEqual:
		return "<="
	case OpGreaterThan:
		return ">"
	case OpGreaterEqual:
		return ">="
	case OpAnd:
		return "&&"
	case OpOr:
		return "||"
	default:
		return "unknown"
	}
}

// IsArithmetic returns true if the operator is arithmetic
func (op BinaryOperator) IsArithmetic() bool {
	return op >= OpAdd && op <= OpPower
}

// IsComparison returns true if the operator is comparison
func (op BinaryOperator) IsComparison() bool {
	return op >= OpEqual && op <= OpGreaterEqual
}

// IsLogical returns true if the operator is logical
func (op BinaryOperator) IsLogical() bool {
	return op == OpAnd || op == OpOr
}

// UnaryOperator defines unary operators
type UnaryOperator int

const (
	OpNegate UnaryOperator = iota // - (negation)
	OpNot                          // ! (logical not)
)

func (op UnaryOperator) String() string {
	switch op {
	case OpNegate:
		return "-"
	case OpNot:
		return "!"
	default:
		return "unknown"
	}
}

// FunctionName defines built-in functions
type FunctionName int

const (
	FuncUnknown FunctionName = iota
	FuncAbs                  // abs(x)
	FuncSqrt                 // sqrt(x)
	FuncMin                  // min(x, y, ...)
	FuncMax                  // max(x, y, ...)
	FuncFloor                // floor(x)
	FuncCeil                 // ceil(x)
	FuncRound                // round(x)
	FuncLog                  // log(x)
	FuncLog10                // log10(x)
	FuncExp                  // exp(x)
	FuncPow                  // pow(x, y)
	FuncSin                  // sin(x)
	FuncCos                  // cos(x)
	FuncTan                  // tan(x)
)

func (fn FunctionName) String() string {
	switch fn {
	case FuncAbs:
		return "abs"
	case FuncSqrt:
		return "sqrt"
	case FuncMin:
		return "min"
	case FuncMax:
		return "max"
	case FuncFloor:
		return "floor"
	case FuncCeil:
		return "ceil"
	case FuncRound:
		return "round"
	case FuncLog:
		return "log"
	case FuncLog10:
		return "log10"
	case FuncExp:
		return "exp"
	case FuncPow:
		return "pow"
	case FuncSin:
		return "sin"
	case FuncCos:
		return "cos"
	case FuncTan:
		return "tan"
	default:
		return "unknown"
	}
}

// Expression is the base interface for all expression nodes
type Expression interface {
	Type() ExpressionType
	DataType() DataType
	String() string
}

// ConstExpression represents a constant value
type ConstExpression struct {
	Value    interface{} // bool, int64, float64, or string
	DataTyp  DataType
}

func (e *ConstExpression) Type() ExpressionType { return ExprTypeConst }
func (e *ConstExpression) DataType() DataType   { return e.DataTyp }
func (e *ConstExpression) String() string {
	return fmt.Sprintf("Const(%v)", e.Value)
}

// NewConstBool creates a boolean constant
func NewConstBool(value bool) *ConstExpression {
	return &ConstExpression{Value: value, DataTyp: DataTypeBool}
}

// NewConstInt creates an integer constant
func NewConstInt(value int64) *ConstExpression {
	return &ConstExpression{Value: value, DataTyp: DataTypeInt64}
}

// NewConstFloat creates a float constant
func NewConstFloat(value float64) *ConstExpression {
	return &ConstExpression{Value: value, DataTyp: DataTypeFloat64}
}

// NewConstString creates a string constant
func NewConstString(value string) *ConstExpression {
	return &ConstExpression{Value: value, DataTyp: DataTypeString}
}

// FieldExpression represents a document field access
type FieldExpression struct {
	FieldPath string   // e.g., "price", "metadata.category"
	DataTyp   DataType // Expected data type
}

func (e *FieldExpression) Type() ExpressionType { return ExprTypeField }
func (e *FieldExpression) DataType() DataType   { return e.DataTyp }
func (e *FieldExpression) String() string {
	return fmt.Sprintf("Field(%s)", e.FieldPath)
}

// NewField creates a field expression
func NewField(fieldPath string, dataType DataType) *FieldExpression {
	return &FieldExpression{FieldPath: fieldPath, DataTyp: dataType}
}

// BinaryOpExpression represents a binary operation
type BinaryOpExpression struct {
	Operator BinaryOperator
	Left     Expression
	Right    Expression
	DataTyp  DataType // Result data type
}

func (e *BinaryOpExpression) Type() ExpressionType { return ExprTypeBinaryOp }
func (e *BinaryOpExpression) DataType() DataType   { return e.DataTyp }
func (e *BinaryOpExpression) String() string {
	return fmt.Sprintf("BinaryOp(%s, %s, %s)", e.Operator, e.Left, e.Right)
}

// NewBinaryOp creates a binary operation expression
func NewBinaryOp(op BinaryOperator, left, right Expression, resultType DataType) *BinaryOpExpression {
	return &BinaryOpExpression{
		Operator: op,
		Left:     left,
		Right:    right,
		DataTyp:  resultType,
	}
}

// UnaryOpExpression represents a unary operation
type UnaryOpExpression struct {
	Operator UnaryOperator
	Operand  Expression
	DataTyp  DataType
}

func (e *UnaryOpExpression) Type() ExpressionType { return ExprTypeUnaryOp }
func (e *UnaryOpExpression) DataType() DataType   { return e.DataTyp }
func (e *UnaryOpExpression) String() string {
	return fmt.Sprintf("UnaryOp(%s, %s)", e.Operator, e.Operand)
}

// NewUnaryOp creates a unary operation expression
func NewUnaryOp(op UnaryOperator, operand Expression, resultType DataType) *UnaryOpExpression {
	return &UnaryOpExpression{
		Operator: op,
		Operand:  operand,
		DataTyp:  resultType,
	}
}

// TernaryExpression represents a ternary conditional (condition ? true_val : false_val)
type TernaryExpression struct {
	Condition  Expression
	TrueValue  Expression
	FalseValue Expression
	DataTyp    DataType // Result data type
}

func (e *TernaryExpression) Type() ExpressionType { return ExprTypeTernary }
func (e *TernaryExpression) DataType() DataType   { return e.DataTyp }
func (e *TernaryExpression) String() string {
	return fmt.Sprintf("Ternary(%s ? %s : %s)", e.Condition, e.TrueValue, e.FalseValue)
}

// NewTernary creates a ternary expression
func NewTernary(condition, trueValue, falseValue Expression, resultType DataType) *TernaryExpression {
	return &TernaryExpression{
		Condition:  condition,
		TrueValue:  trueValue,
		FalseValue: falseValue,
		DataTyp:    resultType,
	}
}

// FunctionExpression represents a function call
type FunctionExpression struct {
	Function FunctionName
	Args     []Expression
	DataTyp  DataType // Result data type
}

func (e *FunctionExpression) Type() ExpressionType { return ExprTypeFunction }
func (e *FunctionExpression) DataType() DataType   { return e.DataTyp }
func (e *FunctionExpression) String() string {
	return fmt.Sprintf("Function(%s, %d args)", e.Function, len(e.Args))
}

// NewFunction creates a function expression
func NewFunction(fn FunctionName, args []Expression, resultType DataType) *FunctionExpression {
	return &FunctionExpression{
		Function: fn,
		Args:     args,
		DataTyp:  resultType,
	}
}
