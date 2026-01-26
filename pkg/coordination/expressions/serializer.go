package expressions

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

// Serializer serializes expression ASTs to binary format for C++ evaluation
type Serializer struct {
	buf *bytes.Buffer
}

// NewSerializer creates a new expression serializer
func NewSerializer() *Serializer {
	return &Serializer{
		buf: new(bytes.Buffer),
	}
}

// Serialize serializes an expression to bytes
func (s *Serializer) Serialize(expr Expression) ([]byte, error) {
	s.buf.Reset()

	if err := s.serializeNode(expr); err != nil {
		return nil, err
	}

	return s.buf.Bytes(), nil
}

// serializeNode serializes a single expression node
func (s *Serializer) serializeNode(expr Expression) error {
	switch e := expr.(type) {
	case *ConstExpression:
		return s.serializeConst(e)
	case *FieldExpression:
		return s.serializeField(e)
	case *BinaryOpExpression:
		return s.serializeBinaryOp(e)
	case *UnaryOpExpression:
		return s.serializeUnaryOp(e)
	case *TernaryExpression:
		return s.serializeTernary(e)
	case *FunctionExpression:
		return s.serializeFunction(e)
	default:
		return fmt.Errorf("unknown expression type: %T", expr)
	}
}

// serializeConst serializes a constant expression
func (s *Serializer) serializeConst(expr *ConstExpression) error {
	// Write expression type
	s.writeByte(byte(ExprTypeConst))

	// Write data type
	s.writeDataType(expr.DataTyp)

	// Write value based on type
	switch expr.DataTyp {
	case DataTypeBool:
		val, ok := expr.Value.(bool)
		if !ok {
			return fmt.Errorf("expected bool value")
		}
		s.writeBool(val)

	case DataTypeInt64:
		val, ok := expr.Value.(int64)
		if !ok {
			return fmt.Errorf("expected int64 value")
		}
		s.writeInt64(val)

	case DataTypeFloat64:
		val, ok := expr.Value.(float64)
		if !ok {
			return fmt.Errorf("expected float64 value")
		}
		s.writeFloat64(val)

	case DataTypeString:
		val, ok := expr.Value.(string)
		if !ok {
			return fmt.Errorf("expected string value")
		}
		s.writeString(val)

	default:
		return fmt.Errorf("unknown data type: %v", expr.DataTyp)
	}

	return nil
}

// serializeField serializes a field expression
func (s *Serializer) serializeField(expr *FieldExpression) error {
	// Write expression type
	s.writeByte(byte(ExprTypeField))

	// Write data type
	s.writeDataType(expr.DataTyp)

	// Write field path
	s.writeString(expr.FieldPath)

	return nil
}

// serializeBinaryOp serializes a binary operation expression
func (s *Serializer) serializeBinaryOp(expr *BinaryOpExpression) error {
	// Write expression type
	s.writeByte(byte(ExprTypeBinaryOp))

	// Write operator
	s.writeBinaryOp(expr.Operator)

	// Write result type
	s.writeDataType(expr.DataTyp)

	// Write left operand
	if err := s.serializeNode(expr.Left); err != nil {
		return fmt.Errorf("failed to serialize left operand: %w", err)
	}

	// Write right operand
	if err := s.serializeNode(expr.Right); err != nil {
		return fmt.Errorf("failed to serialize right operand: %w", err)
	}

	return nil
}

// serializeUnaryOp serializes a unary operation expression
func (s *Serializer) serializeUnaryOp(expr *UnaryOpExpression) error {
	// Write expression type
	s.writeByte(byte(ExprTypeUnaryOp))

	// Write operator
	s.writeUnaryOp(expr.Operator)

	// Write result type
	s.writeDataType(expr.DataTyp)

	// Write operand
	if err := s.serializeNode(expr.Operand); err != nil {
		return fmt.Errorf("failed to serialize operand: %w", err)
	}

	return nil
}

// serializeTernary serializes a ternary expression
func (s *Serializer) serializeTernary(expr *TernaryExpression) error {
	// Write expression type
	s.writeByte(byte(ExprTypeTernary))

	// Write result type
	s.writeDataType(expr.DataTyp)

	// Write condition
	if err := s.serializeNode(expr.Condition); err != nil {
		return fmt.Errorf("failed to serialize condition: %w", err)
	}

	// Write true value
	if err := s.serializeNode(expr.TrueValue); err != nil {
		return fmt.Errorf("failed to serialize true value: %w", err)
	}

	// Write false value
	if err := s.serializeNode(expr.FalseValue); err != nil {
		return fmt.Errorf("failed to serialize false value: %w", err)
	}

	return nil
}

// serializeFunction serializes a function expression
func (s *Serializer) serializeFunction(expr *FunctionExpression) error {
	// Write expression type
	s.writeByte(byte(ExprTypeFunction))

	// Write function
	s.writeFunction(expr.Function)

	// Write result type
	s.writeDataType(expr.DataTyp)

	// Write argument count
	s.writeUint32(uint32(len(expr.Args)))

	// Write arguments
	for i, arg := range expr.Args {
		if err := s.serializeNode(arg); err != nil {
			return fmt.Errorf("failed to serialize argument %d: %w", i, err)
		}
	}

	return nil
}

// Low-level write methods

func (s *Serializer) writeByte(b byte) {
	s.buf.WriteByte(b)
}

func (s *Serializer) writeDataType(dt DataType) {
	s.buf.WriteByte(byte(dt))
}

func (s *Serializer) writeBinaryOp(op BinaryOperator) {
	s.buf.WriteByte(byte(op))
}

func (s *Serializer) writeUnaryOp(op UnaryOperator) {
	s.buf.WriteByte(byte(op))
}

func (s *Serializer) writeFunction(fn FunctionName) {
	s.buf.WriteByte(byte(fn))
}

func (s *Serializer) writeBool(val bool) {
	if val {
		s.buf.WriteByte(1)
	} else {
		s.buf.WriteByte(0)
	}
}

func (s *Serializer) writeInt64(val int64) {
	binary.Write(s.buf, binary.LittleEndian, val)
}

func (s *Serializer) writeUint32(val uint32) {
	binary.Write(s.buf, binary.LittleEndian, val)
}

func (s *Serializer) writeFloat64(val float64) {
	binary.Write(s.buf, binary.LittleEndian, val)
}

func (s *Serializer) writeString(val string) {
	// Write length
	s.writeUint32(uint32(len(val)))
	// Write string bytes
	s.buf.WriteString(val)
}
