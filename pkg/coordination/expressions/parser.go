package expressions

import (
	"fmt"
	"strconv"
)

// Parser parses JSON expression trees into AST
type Parser struct{}

// NewParser creates a new expression parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses a JSON expression map into an Expression AST
func (p *Parser) Parse(exprMap map[string]interface{}) (Expression, error) {
	if exprMap == nil {
		return nil, fmt.Errorf("expression map is nil")
	}

	// Check for operator type
	if op, ok := exprMap["op"].(string); ok {
		return p.parseOperator(op, exprMap)
	}

	// Check for constant
	if constVal, ok := exprMap["const"]; ok {
		return p.parseConst(constVal)
	}

	// Check for field access
	if fieldPath, ok := exprMap["field"].(string); ok {
		return p.parseField(fieldPath, exprMap)
	}

	// Check for function call
	if fnName, ok := exprMap["func"].(string); ok {
		return p.parseFunction(fnName, exprMap)
	}

	return nil, fmt.Errorf("unrecognized expression format")
}

// parseOperator parses an operator expression
func (p *Parser) parseOperator(opStr string, exprMap map[string]interface{}) (Expression, error) {
	// Try binary operators first
	binOp := p.parseBinaryOperator(opStr)
	if binOp != OpUnknown {
		return p.parseBinaryOp(binOp, exprMap)
	}

	// Try unary operators
	unOp := p.parseUnaryOperator(opStr)
	if unOp != OpNegate && unOp != OpNot {
		return nil, fmt.Errorf("unknown operator: %s", opStr)
	}
	return p.parseUnaryOp(unOp, exprMap)
}

// parseBinaryOperator converts string to BinaryOperator
func (p *Parser) parseBinaryOperator(opStr string) BinaryOperator {
	switch opStr {
	case "+":
		return OpAdd
	case "-":
		return OpSubtract
	case "*":
		return OpMultiply
	case "/":
		return OpDivide
	case "%":
		return OpModulo
	case "pow", "**":
		return OpPower
	case "==", "eq":
		return OpEqual
	case "!=", "ne":
		return OpNotEqual
	case "<", "lt":
		return OpLessThan
	case "<=", "lte", "le":
		return OpLessEqual
	case ">", "gt":
		return OpGreaterThan
	case ">=", "gte", "ge":
		return OpGreaterEqual
	case "&&", "and":
		return OpAnd
	case "||", "or":
		return OpOr
	default:
		return OpUnknown
	}
}

// parseUnaryOperator converts string to UnaryOperator
func (p *Parser) parseUnaryOperator(opStr string) UnaryOperator {
	switch opStr {
	case "-", "neg":
		return OpNegate
	case "!", "not":
		return OpNot
	default:
		return OpNegate // Default, will be caught as unknown
	}
}

// parseBinaryOp parses a binary operation
func (p *Parser) parseBinaryOp(op BinaryOperator, exprMap map[string]interface{}) (Expression, error) {
	leftMap, okLeft := exprMap["left"].(map[string]interface{})
	rightMap, okRight := exprMap["right"].(map[string]interface{})

	if !okLeft || !okRight {
		return nil, fmt.Errorf("binary operator %s requires 'left' and 'right' expressions", op)
	}

	left, err := p.Parse(leftMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse left operand: %w", err)
	}

	right, err := p.Parse(rightMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse right operand: %w", err)
	}

	// Determine result type
	resultType := p.inferBinaryOpResultType(op, left.DataType(), right.DataType())

	return NewBinaryOp(op, left, right, resultType), nil
}

// parseUnaryOp parses a unary operation
func (p *Parser) parseUnaryOp(op UnaryOperator, exprMap map[string]interface{}) (Expression, error) {
	operandMap, ok := exprMap["operand"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unary operator %s requires 'operand' expression", op)
	}

	operand, err := p.Parse(operandMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse operand: %w", err)
	}

	// Determine result type
	resultType := operand.DataType()
	if op == OpNot {
		resultType = DataTypeBool
	}

	return NewUnaryOp(op, operand, resultType), nil
}

// parseConst parses a constant value
func (p *Parser) parseConst(constVal interface{}) (Expression, error) {
	switch v := constVal.(type) {
	case bool:
		return NewConstBool(v), nil
	case int:
		return NewConstInt(int64(v)), nil
	case int64:
		return NewConstInt(v), nil
	case float32:
		return NewConstFloat(float64(v)), nil
	case float64:
		return NewConstFloat(v), nil
	case string:
		// Try to parse as number first
		if intVal, err := strconv.ParseInt(v, 10, 64); err == nil {
			return NewConstInt(intVal), nil
		}
		if floatVal, err := strconv.ParseFloat(v, 64); err == nil {
			return NewConstFloat(floatVal), nil
		}
		// Parse as string
		return NewConstString(v), nil
	default:
		return nil, fmt.Errorf("unsupported constant type: %T", constVal)
	}
}

// parseField parses a field access expression
func (p *Parser) parseField(fieldPath string, exprMap map[string]interface{}) (Expression, error) {
	// Try to infer type from optional "type" field
	dataType := DataTypeFloat64 // Default to float64 for numeric fields

	if typeStr, ok := exprMap["type"].(string); ok {
		switch typeStr {
		case "bool", "boolean":
			dataType = DataTypeBool
		case "int", "int64", "integer":
			dataType = DataTypeInt64
		case "float", "float64", "double":
			dataType = DataTypeFloat64
		case "string", "text":
			dataType = DataTypeString
		}
	}

	return NewField(fieldPath, dataType), nil
}

// parseFunction parses a function call
func (p *Parser) parseFunction(fnName string, exprMap map[string]interface{}) (Expression, error) {
	fn := p.parseFunctionName(fnName)
	if fn == FuncUnknown {
		return nil, fmt.Errorf("unknown function: %s", fnName)
	}

	// Parse arguments
	argsInterface, ok := exprMap["args"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("function %s requires 'args' array", fnName)
	}

	args := make([]Expression, 0, len(argsInterface))
	for i, argInterface := range argsInterface {
		argMap, ok := argInterface.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("function argument %d must be an expression object", i)
		}

		arg, err := p.Parse(argMap)
		if err != nil {
			return nil, fmt.Errorf("failed to parse argument %d: %w", i, err)
		}
		args = append(args, arg)
	}

	// Determine result type
	resultType := p.inferFunctionResultType(fn, args)

	return NewFunction(fn, args, resultType), nil
}

// parseFunctionName converts string to FunctionName
func (p *Parser) parseFunctionName(fnName string) FunctionName {
	switch fnName {
	case "abs":
		return FuncAbs
	case "sqrt":
		return FuncSqrt
	case "min":
		return FuncMin
	case "max":
		return FuncMax
	case "floor":
		return FuncFloor
	case "ceil":
		return FuncCeil
	case "round":
		return FuncRound
	case "log", "ln":
		return FuncLog
	case "log10":
		return FuncLog10
	case "exp":
		return FuncExp
	case "pow":
		return FuncPow
	case "sin":
		return FuncSin
	case "cos":
		return FuncCos
	case "tan":
		return FuncTan
	default:
		return FuncUnknown
	}
}

// inferBinaryOpResultType infers the result type of a binary operation
func (p *Parser) inferBinaryOpResultType(op BinaryOperator, leftType, rightType DataType) DataType {
	// Comparison and logical operators always return bool
	if op.IsComparison() || op.IsLogical() {
		return DataTypeBool
	}

	// Arithmetic operators: promote to wider type
	if leftType == DataTypeFloat64 || rightType == DataTypeFloat64 {
		return DataTypeFloat64
	}
	if leftType == DataTypeInt64 || rightType == DataTypeInt64 {
		return DataTypeInt64
	}

	return DataTypeFloat64 // Default
}

// inferFunctionResultType infers the result type of a function
func (p *Parser) inferFunctionResultType(fn FunctionName, args []Expression) DataType {
	switch fn {
	case FuncMin, FuncMax:
		// Return type matches first argument
		if len(args) > 0 {
			return args[0].DataType()
		}
		return DataTypeFloat64
	case FuncFloor, FuncCeil, FuncRound:
		return DataTypeInt64
	default:
		return DataTypeFloat64
	}
}

// ParseTernary parses a ternary conditional expression
func (p *Parser) ParseTernary(exprMap map[string]interface{}) (Expression, error) {
	condMap, okCond := exprMap["condition"].(map[string]interface{})
	trueMap, okTrue := exprMap["true"].(map[string]interface{})
	falseMap, okFalse := exprMap["false"].(map[string]interface{})

	if !okCond || !okTrue || !okFalse {
		return nil, fmt.Errorf("ternary requires 'condition', 'true', and 'false' expressions")
	}

	condition, err := p.Parse(condMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse condition: %w", err)
	}

	trueValue, err := p.Parse(trueMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse true value: %w", err)
	}

	falseValue, err := p.Parse(falseMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse false value: %w", err)
	}

	// Result type matches the branches
	resultType := trueValue.DataType()
	if resultType != falseValue.DataType() {
		// Promote to wider type
		if trueValue.DataType() == DataTypeFloat64 || falseValue.DataType() == DataTypeFloat64 {
			resultType = DataTypeFloat64
		}
	}

	return NewTernary(condition, trueValue, falseValue, resultType), nil
}
