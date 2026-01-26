#include "expression_evaluator.h"
#include "document.h"
#include <cmath>
#include <stdexcept>
#include <algorithm>

namespace diagon {

// FieldExpression evaluation
ExprValue FieldExpression::evaluate(const Document& doc) const {
    if (!doc.has_field(field_path_)) {
        // Return default value based on type
        switch (dtype_) {
            case DataType::BOOL:
                return false;
            case DataType::INT64:
                return int64_t(0);
            case DataType::FLOAT64:
                return 0.0;
            case DataType::STRING:
                return std::string("");
            default:
                throw std::runtime_error("Unknown data type");
        }
    }
    return doc.get_field(field_path_);
}

// BinaryOpExpression evaluation
ExprValue BinaryOpExpression::evaluate(const Document& doc) const {
    ExprValue left_val = left_->evaluate(doc);
    ExprValue right_val = right_->evaluate(doc);

    switch (op_) {
        // Arithmetic operators
        case BinaryOp::ADD: {
            double l = to_double(left_val);
            double r = to_double(right_val);
            if (result_type_ == DataType::INT64) {
                return to_int64(left_val) + to_int64(right_val);
            }
            return l + r;
        }

        case BinaryOp::SUBTRACT: {
            double l = to_double(left_val);
            double r = to_double(right_val);
            if (result_type_ == DataType::INT64) {
                return to_int64(left_val) - to_int64(right_val);
            }
            return l - r;
        }

        case BinaryOp::MULTIPLY: {
            double l = to_double(left_val);
            double r = to_double(right_val);
            if (result_type_ == DataType::INT64) {
                return to_int64(left_val) * to_int64(right_val);
            }
            return l * r;
        }

        case BinaryOp::DIVIDE: {
            double l = to_double(left_val);
            double r = to_double(right_val);
            if (r == 0.0) {
                throw std::runtime_error("Division by zero");
            }
            if (result_type_ == DataType::INT64) {
                int64_t ri = to_int64(right_val);
                if (ri == 0) {
                    throw std::runtime_error("Division by zero");
                }
                return to_int64(left_val) / ri;
            }
            return l / r;
        }

        case BinaryOp::MODULO: {
            int64_t l = to_int64(left_val);
            int64_t r = to_int64(right_val);
            if (r == 0) {
                throw std::runtime_error("Modulo by zero");
            }
            return l % r;
        }

        case BinaryOp::POWER: {
            double l = to_double(left_val);
            double r = to_double(right_val);
            return std::pow(l, r);
        }

        // Comparison operators
        case BinaryOp::EQUAL: {
            if (std::holds_alternative<bool>(left_val)) {
                return to_bool(left_val) == to_bool(right_val);
            } else if (std::holds_alternative<std::string>(left_val)) {
                return std::get<std::string>(left_val) == std::get<std::string>(right_val);
            } else {
                return to_double(left_val) == to_double(right_val);
            }
        }

        case BinaryOp::NOT_EQUAL: {
            if (std::holds_alternative<bool>(left_val)) {
                return to_bool(left_val) != to_bool(right_val);
            } else if (std::holds_alternative<std::string>(left_val)) {
                return std::get<std::string>(left_val) != std::get<std::string>(right_val);
            } else {
                return to_double(left_val) != to_double(right_val);
            }
        }

        case BinaryOp::LESS_THAN: {
            return to_double(left_val) < to_double(right_val);
        }

        case BinaryOp::LESS_EQUAL: {
            return to_double(left_val) <= to_double(right_val);
        }

        case BinaryOp::GREATER_THAN: {
            return to_double(left_val) > to_double(right_val);
        }

        case BinaryOp::GREATER_EQUAL: {
            return to_double(left_val) >= to_double(right_val);
        }

        // Logical operators
        case BinaryOp::AND: {
            return to_bool(left_val) && to_bool(right_val);
        }

        case BinaryOp::OR: {
            return to_bool(left_val) || to_bool(right_val);
        }

        default:
            throw std::runtime_error("Unknown binary operator");
    }
}

// UnaryOpExpression evaluation
ExprValue UnaryOpExpression::evaluate(const Document& doc) const {
    ExprValue val = operand_->evaluate(doc);

    switch (op_) {
        case UnaryOp::NEGATE: {
            if (result_type_ == DataType::INT64) {
                return -to_int64(val);
            }
            return -to_double(val);
        }

        case UnaryOp::NOT: {
            return !to_bool(val);
        }

        default:
            throw std::runtime_error("Unknown unary operator");
    }
}

// TernaryExpression evaluation
ExprValue TernaryExpression::evaluate(const Document& doc) const {
    ExprValue cond_val = condition_->evaluate(doc);

    if (to_bool(cond_val)) {
        return true_value_->evaluate(doc);
    } else {
        return false_value_->evaluate(doc);
    }
}

// FunctionExpression evaluation
ExprValue FunctionExpression::evaluate(const Document& doc) const {
    // Evaluate all arguments
    std::vector<ExprValue> arg_vals;
    arg_vals.reserve(args_.size());
    for (const auto& arg : args_) {
        arg_vals.push_back(arg->evaluate(doc));
    }

    switch (func_) {
        case Function::ABS: {
            double val = to_double(arg_vals[0]);
            return std::abs(val);
        }

        case Function::SQRT: {
            double val = to_double(arg_vals[0]);
            if (val < 0) {
                throw std::runtime_error("sqrt of negative number");
            }
            return std::sqrt(val);
        }

        case Function::MIN: {
            double min_val = to_double(arg_vals[0]);
            for (size_t i = 1; i < arg_vals.size(); ++i) {
                min_val = std::min(min_val, to_double(arg_vals[i]));
            }
            if (result_type_ == DataType::INT64) {
                return static_cast<int64_t>(min_val);
            }
            return min_val;
        }

        case Function::MAX: {
            double max_val = to_double(arg_vals[0]);
            for (size_t i = 1; i < arg_vals.size(); ++i) {
                max_val = std::max(max_val, to_double(arg_vals[i]));
            }
            if (result_type_ == DataType::INT64) {
                return static_cast<int64_t>(max_val);
            }
            return max_val;
        }

        case Function::FLOOR: {
            double val = to_double(arg_vals[0]);
            return static_cast<int64_t>(std::floor(val));
        }

        case Function::CEIL: {
            double val = to_double(arg_vals[0]);
            return static_cast<int64_t>(std::ceil(val));
        }

        case Function::ROUND: {
            double val = to_double(arg_vals[0]);
            return static_cast<int64_t>(std::round(val));
        }

        case Function::LOG: {
            double val = to_double(arg_vals[0]);
            if (val <= 0) {
                throw std::runtime_error("log of non-positive number");
            }
            return std::log(val);
        }

        case Function::LOG10: {
            double val = to_double(arg_vals[0]);
            if (val <= 0) {
                throw std::runtime_error("log10 of non-positive number");
            }
            return std::log10(val);
        }

        case Function::EXP: {
            double val = to_double(arg_vals[0]);
            return std::exp(val);
        }

        case Function::POW: {
            double base = to_double(arg_vals[0]);
            double exponent = to_double(arg_vals[1]);
            return std::pow(base, exponent);
        }

        case Function::SIN: {
            double val = to_double(arg_vals[0]);
            return std::sin(val);
        }

        case Function::COS: {
            double val = to_double(arg_vals[0]);
            return std::cos(val);
        }

        case Function::TAN: {
            double val = to_double(arg_vals[0]);
            return std::tan(val);
        }

        default:
            throw std::runtime_error("Unknown function");
    }
}

// ExpressionEvaluator methods

std::unique_ptr<Expression> ExpressionEvaluator::deserialize(
    const uint8_t* data, size_t size) {
    const uint8_t* ptr = data;
    return deserialize_node(ptr);
}

ExprValue ExpressionEvaluator::evaluate(
    const Expression& expr,
    const Document& doc) {
    return expr.evaluate(doc);
}

std::vector<ExprValue> ExpressionEvaluator::evaluate_batch(
    const Expression& expr,
    const std::vector<const Document*>& docs) {
    std::vector<ExprValue> results;
    results.reserve(docs.size());

    for (const Document* doc : docs) {
        results.push_back(expr.evaluate(*doc));
    }

    return results;
}

// Deserialization helpers (simplified binary format)
std::unique_ptr<Expression> ExpressionEvaluator::deserialize_node(
    const uint8_t*& ptr) {

    // Read expression type (1 byte)
    ExprType expr_type = static_cast<ExprType>(*ptr++);

    switch (expr_type) {
        case ExprType::CONST: {
            DataType dtype = read_data_type(ptr);
            switch (dtype) {
                case DataType::BOOL: {
                    bool val = read_bool(ptr);
                    return std::make_unique<ConstExpression>(val, dtype);
                }
                case DataType::INT64: {
                    int64_t val = read_int64(ptr);
                    return std::make_unique<ConstExpression>(val, dtype);
                }
                case DataType::FLOAT64: {
                    double val = read_double(ptr);
                    return std::make_unique<ConstExpression>(val, dtype);
                }
                case DataType::STRING: {
                    std::string val = read_string(ptr);
                    return std::make_unique<ConstExpression>(val, dtype);
                }
                default:
                    throw std::runtime_error("Unknown constant type");
            }
        }

        case ExprType::FIELD: {
            DataType dtype = read_data_type(ptr);
            std::string field_path = read_string(ptr);
            return std::make_unique<FieldExpression>(field_path, dtype);
        }

        case ExprType::BINARY_OP: {
            BinaryOp op = read_binary_op(ptr);
            DataType result_type = read_data_type(ptr);
            auto left = deserialize_node(ptr);
            auto right = deserialize_node(ptr);
            return std::make_unique<BinaryOpExpression>(
                op, std::move(left), std::move(right), result_type);
        }

        case ExprType::UNARY_OP: {
            UnaryOp op = read_unary_op(ptr);
            DataType result_type = read_data_type(ptr);
            auto operand = deserialize_node(ptr);
            return std::make_unique<UnaryOpExpression>(
                op, std::move(operand), result_type);
        }

        case ExprType::TERNARY: {
            DataType result_type = read_data_type(ptr);
            auto condition = deserialize_node(ptr);
            auto true_val = deserialize_node(ptr);
            auto false_val = deserialize_node(ptr);
            return std::make_unique<TernaryExpression>(
                std::move(condition), std::move(true_val),
                std::move(false_val), result_type);
        }

        case ExprType::FUNCTION: {
            Function func = read_function(ptr);
            DataType result_type = read_data_type(ptr);
            uint32_t arg_count = *reinterpret_cast<const uint32_t*>(ptr);
            ptr += sizeof(uint32_t);

            std::vector<std::unique_ptr<Expression>> args;
            args.reserve(arg_count);
            for (uint32_t i = 0; i < arg_count; ++i) {
                args.push_back(deserialize_node(ptr));
            }

            return std::make_unique<FunctionExpression>(
                func, std::move(args), result_type);
        }

        default:
            throw std::runtime_error("Unknown expression type");
    }
}

DataType ExpressionEvaluator::read_data_type(const uint8_t*& ptr) {
    DataType dtype = static_cast<DataType>(*ptr++);
    return dtype;
}

BinaryOp ExpressionEvaluator::read_binary_op(const uint8_t*& ptr) {
    BinaryOp op = static_cast<BinaryOp>(*ptr++);
    return op;
}

UnaryOp ExpressionEvaluator::read_unary_op(const uint8_t*& ptr) {
    UnaryOp op = static_cast<UnaryOp>(*ptr++);
    return op;
}

Function ExpressionEvaluator::read_function(const uint8_t*& ptr) {
    Function func = static_cast<Function>(*ptr++);
    return func;
}

std::string ExpressionEvaluator::read_string(const uint8_t*& ptr) {
    uint32_t len = *reinterpret_cast<const uint32_t*>(ptr);
    ptr += sizeof(uint32_t);

    std::string str(reinterpret_cast<const char*>(ptr), len);
    ptr += len;

    return str;
}

double ExpressionEvaluator::read_double(const uint8_t*& ptr) {
    double val = *reinterpret_cast<const double*>(ptr);
    ptr += sizeof(double);
    return val;
}

int64_t ExpressionEvaluator::read_int64(const uint8_t*& ptr) {
    int64_t val = *reinterpret_cast<const int64_t*>(ptr);
    ptr += sizeof(int64_t);
    return val;
}

bool ExpressionEvaluator::read_bool(const uint8_t*& ptr) {
    bool val = static_cast<bool>(*ptr++);
    return val;
}

} // namespace diagon
