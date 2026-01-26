#pragma once

#include <cstdint>
#include <memory>
#include <string>
#include <unordered_map>
#include <vector>
#include <variant>

namespace diagon {

// Forward declarations
class Expression;
class Document;

// Value types that expressions can produce
using ExprValue = std::variant<bool, int64_t, double, std::string>;

// Data types
enum class DataType {
    BOOL,
    INT64,
    FLOAT64,
    STRING,
    UNKNOWN
};

// Binary operators
enum class BinaryOp {
    // Arithmetic
    ADD,
    SUBTRACT,
    MULTIPLY,
    DIVIDE,
    MODULO,
    POWER,

    // Comparison
    EQUAL,
    NOT_EQUAL,
    LESS_THAN,
    LESS_EQUAL,
    GREATER_THAN,
    GREATER_EQUAL,

    // Logical
    AND,
    OR
};

// Unary operators
enum class UnaryOp {
    NEGATE,
    NOT
};

// Built-in functions
enum class Function {
    ABS,
    SQRT,
    MIN,
    MAX,
    FLOOR,
    CEIL,
    ROUND,
    LOG,
    LOG10,
    EXP,
    POW,
    SIN,
    COS,
    TAN
};

// Expression node types
enum class ExprType {
    CONST,
    FIELD,
    BINARY_OP,
    UNARY_OP,
    TERNARY,
    FUNCTION
};

// Base Expression class
class Expression {
public:
    virtual ~Expression() = default;

    virtual ExprType type() const = 0;
    virtual DataType data_type() const = 0;
    virtual ExprValue evaluate(const Document& doc) const = 0;

    // Helper to get typed value
    template<typename T>
    T get_value(const ExprValue& val) const {
        return std::get<T>(val);
    }
};

// Constant expression
class ConstExpression : public Expression {
public:
    explicit ConstExpression(ExprValue value, DataType dtype)
        : value_(std::move(value)), dtype_(dtype) {}

    ExprType type() const override { return ExprType::CONST; }
    DataType data_type() const override { return dtype_; }
    ExprValue evaluate(const Document& doc) const override { return value_; }

private:
    ExprValue value_;
    DataType dtype_;
};

// Field access expression
class FieldExpression : public Expression {
public:
    explicit FieldExpression(std::string field_path, DataType dtype)
        : field_path_(std::move(field_path)), dtype_(dtype) {}

    ExprType type() const override { return ExprType::FIELD; }
    DataType data_type() const override { return dtype_; }
    ExprValue evaluate(const Document& doc) const override;

private:
    std::string field_path_;
    DataType dtype_;
};

// Binary operation expression
class BinaryOpExpression : public Expression {
public:
    BinaryOpExpression(BinaryOp op,
                       std::unique_ptr<Expression> left,
                       std::unique_ptr<Expression> right,
                       DataType result_type)
        : op_(op),
          left_(std::move(left)),
          right_(std::move(right)),
          result_type_(result_type) {}

    ExprType type() const override { return ExprType::BINARY_OP; }
    DataType data_type() const override { return result_type_; }
    ExprValue evaluate(const Document& doc) const override;

private:
    BinaryOp op_;
    std::unique_ptr<Expression> left_;
    std::unique_ptr<Expression> right_;
    DataType result_type_;
};

// Unary operation expression
class UnaryOpExpression : public Expression {
public:
    UnaryOpExpression(UnaryOp op,
                      std::unique_ptr<Expression> operand,
                      DataType result_type)
        : op_(op),
          operand_(std::move(operand)),
          result_type_(result_type) {}

    ExprType type() const override { return ExprType::UNARY_OP; }
    DataType data_type() const override { return result_type_; }
    ExprValue evaluate(const Document& doc) const override;

private:
    UnaryOp op_;
    std::unique_ptr<Expression> operand_;
    DataType result_type_;
};

// Ternary conditional expression
class TernaryExpression : public Expression {
public:
    TernaryExpression(std::unique_ptr<Expression> condition,
                      std::unique_ptr<Expression> true_value,
                      std::unique_ptr<Expression> false_value,
                      DataType result_type)
        : condition_(std::move(condition)),
          true_value_(std::move(true_value)),
          false_value_(std::move(false_value)),
          result_type_(result_type) {}

    ExprType type() const override { return ExprType::TERNARY; }
    DataType data_type() const override { return result_type_; }
    ExprValue evaluate(const Document& doc) const override;

private:
    std::unique_ptr<Expression> condition_;
    std::unique_ptr<Expression> true_value_;
    std::unique_ptr<Expression> false_value_;
    DataType result_type_;
};

// Function call expression
class FunctionExpression : public Expression {
public:
    FunctionExpression(Function func,
                       std::vector<std::unique_ptr<Expression>> args,
                       DataType result_type)
        : func_(func),
          args_(std::move(args)),
          result_type_(result_type) {}

    ExprType type() const override { return ExprType::FUNCTION; }
    DataType data_type() const override { return result_type_; }
    ExprValue evaluate(const Document& doc) const override;

private:
    Function func_;
    std::vector<std::unique_ptr<Expression>> args_;
    DataType result_type_;
};

// Forward declaration - full interface in document.h
class Document;

// Expression evaluator - main interface
class ExpressionEvaluator {
public:
    ExpressionEvaluator() = default;

    // Deserialize expression from bytes (from Go)
    std::unique_ptr<Expression> deserialize(const uint8_t* data, size_t size);

    // Evaluate expression against document
    ExprValue evaluate(const Expression& expr, const Document& doc);

    // Batch evaluation for multiple documents
    std::vector<ExprValue> evaluate_batch(
        const Expression& expr,
        const std::vector<const Document*>& docs
    );

private:
    // Deserialization helpers
    std::unique_ptr<Expression> deserialize_node(const uint8_t*& ptr);
    DataType read_data_type(const uint8_t*& ptr);
    BinaryOp read_binary_op(const uint8_t*& ptr);
    UnaryOp read_unary_op(const uint8_t*& ptr);
    Function read_function(const uint8_t*& ptr);
    std::string read_string(const uint8_t*& ptr);
    double read_double(const uint8_t*& ptr);
    int64_t read_int64(const uint8_t*& ptr);
    bool read_bool(const uint8_t*& ptr);
};

// Helper functions for type conversions
inline double to_double(const ExprValue& val) {
    if (std::holds_alternative<double>(val)) {
        return std::get<double>(val);
    } else if (std::holds_alternative<int64_t>(val)) {
        return static_cast<double>(std::get<int64_t>(val));
    }
    return 0.0;
}

inline int64_t to_int64(const ExprValue& val) {
    if (std::holds_alternative<int64_t>(val)) {
        return std::get<int64_t>(val);
    } else if (std::holds_alternative<double>(val)) {
        return static_cast<int64_t>(std::get<double>(val));
    }
    return 0;
}

inline bool to_bool(const ExprValue& val) {
    if (std::holds_alternative<bool>(val)) {
        return std::get<bool>(val);
    }
    return false;
}

} // namespace diagon
