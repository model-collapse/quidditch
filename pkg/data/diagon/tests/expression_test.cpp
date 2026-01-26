// Expression Evaluator Unit Tests
#include "../document.h"
#include "../expression_evaluator.h"
#include <nlohmann/json.hpp>
#include <gtest/gtest.h>

using namespace diagon;
using json = nlohmann::json;

class ExpressionTest : public ::testing::Test {
protected:
    void SetUp() override {
        // Create test JSON document
        testJson = json{
            {"price", 150.0},
            {"quantity", 5},
            {"discount", 0.2},
            {"in_stock", true},
            {"category", "electronics"}
        };
        doc = std::make_unique<JSONDocument>(&testJson, "doc1");
    }

    json testJson;
    std::unique_ptr<JSONDocument> doc;
};

TEST_F(ExpressionTest, ConstantExpression) {
    ConstExpression intExpr(int64_t(42), DataType::INT64);
    ASSERT_EQ(std::get<int64_t>(intExpr.evaluate(*doc)), 42);

    ConstExpression doubleExpr(3.14, DataType::FLOAT64);
    ASSERT_DOUBLE_EQ(std::get<double>(doubleExpr.evaluate(*doc)), 3.14);

    ConstExpression boolExpr(true, DataType::BOOL);
    ASSERT_TRUE(std::get<bool>(boolExpr.evaluate(*doc)));

    ConstExpression stringExpr(std::string("test"), DataType::STRING);
    ASSERT_EQ(std::get<std::string>(stringExpr.evaluate(*doc)), "test");
}

TEST_F(ExpressionTest, FieldExpression) {
    FieldExpression priceExpr("price", DataType::FLOAT64);
    auto priceVal = priceExpr.evaluate(*doc);
    ASSERT_DOUBLE_EQ(std::get<double>(priceVal), 150.0);

    FieldExpression quantityExpr("quantity", DataType::INT64);
    auto quantityVal = quantityExpr.evaluate(*doc);
    ASSERT_EQ(std::get<int64_t>(quantityVal), 5);

    FieldExpression inStockExpr("in_stock", DataType::BOOL);
    auto inStockVal = inStockExpr.evaluate(*doc);
    ASSERT_TRUE(std::get<bool>(inStockVal));
}

TEST_F(ExpressionTest, BinaryOpComparison) {
    // price > 100
    auto left = std::make_unique<FieldExpression>("price", DataType::FLOAT64);
    auto right = std::make_unique<ConstExpression>(100.0, DataType::FLOAT64);
    BinaryOpExpression greaterThan(
        BinaryOp::GREATER_THAN,
        std::move(left),
        std::move(right),
        DataType::BOOL
    );

    auto result = greaterThan.evaluate(*doc);
    ASSERT_TRUE(std::get<bool>(result));
}

TEST_F(ExpressionTest, BinaryOpArithmetic) {
    // price * (1 - discount) = 150 * 0.8 = 120
    auto price = std::make_unique<FieldExpression>("price", DataType::FLOAT64);
    auto one = std::make_unique<ConstExpression>(1.0, DataType::FLOAT64);
    auto discount = std::make_unique<FieldExpression>("discount", DataType::FLOAT64);

    auto oneMinusDiscount = std::make_unique<BinaryOpExpression>(
        BinaryOp::SUBTRACT,
        std::move(one),
        std::move(discount),
        DataType::FLOAT64
    );

    BinaryOpExpression finalPrice(
        BinaryOp::MULTIPLY,
        std::move(price),
        std::move(oneMinusDiscount),
        DataType::FLOAT64
    );

    auto result = finalPrice.evaluate(*doc);
    ASSERT_DOUBLE_EQ(std::get<double>(result), 120.0);
}

TEST_F(ExpressionTest, BinaryOpLogical) {
    // price > 100 AND in_stock == true
    auto priceCheck = std::make_unique<BinaryOpExpression>(
        BinaryOp::GREATER_THAN,
        std::make_unique<FieldExpression>("price", DataType::FLOAT64),
        std::make_unique<ConstExpression>(100.0, DataType::FLOAT64),
        DataType::BOOL
    );

    auto stockCheck = std::make_unique<FieldExpression>("in_stock", DataType::BOOL);

    BinaryOpExpression andExpr(
        BinaryOp::AND,
        std::move(priceCheck),
        std::move(stockCheck),
        DataType::BOOL
    );

    auto result = andExpr.evaluate(*doc);
    ASSERT_TRUE(std::get<bool>(result));
}

TEST_F(ExpressionTest, UnaryOpNegate) {
    // -price = -150
    UnaryOpExpression negate(
        UnaryOp::NEGATE,
        std::make_unique<FieldExpression>("price", DataType::FLOAT64),
        DataType::FLOAT64
    );

    auto result = negate.evaluate(*doc);
    ASSERT_DOUBLE_EQ(std::get<double>(result), -150.0);
}

TEST_F(ExpressionTest, UnaryOpNot) {
    // NOT in_stock
    UnaryOpExpression notExpr(
        UnaryOp::NOT,
        std::make_unique<FieldExpression>("in_stock", DataType::BOOL),
        DataType::BOOL
    );

    auto result = notExpr.evaluate(*doc);
    ASSERT_FALSE(std::get<bool>(result));
}

TEST_F(ExpressionTest, TernaryExpression) {
    // in_stock ? price : 0.0
    TernaryExpression ternary(
        std::make_unique<FieldExpression>("in_stock", DataType::BOOL),
        std::make_unique<FieldExpression>("price", DataType::FLOAT64),
        std::make_unique<ConstExpression>(0.0, DataType::FLOAT64),
        DataType::FLOAT64
    );

    auto result = ternary.evaluate(*doc);
    ASSERT_DOUBLE_EQ(std::get<double>(result), 150.0);

    // Test false condition
    json outOfStockJson = testJson;
    outOfStockJson["in_stock"] = false;
    JSONDocument outOfStockDoc(&outOfStockJson, "doc2");

    auto result2 = ternary.evaluate(outOfStockDoc);
    ASSERT_DOUBLE_EQ(std::get<double>(result2), 0.0);
}

TEST_F(ExpressionTest, FunctionAbs) {
    // ABS(-42) = 42
    std::vector<std::unique_ptr<Expression>> args;
    args.push_back(std::make_unique<ConstExpression>(int64_t(-42), DataType::INT64));

    FunctionExpression absExpr(Function::ABS, std::move(args), DataType::INT64);
    auto result = absExpr.evaluate(*doc);
    ASSERT_EQ(std::get<int64_t>(result), 42);
}

TEST_F(ExpressionTest, FunctionSqrt) {
    // SQRT(16) = 4
    std::vector<std::unique_ptr<Expression>> args;
    args.push_back(std::make_unique<ConstExpression>(16.0, DataType::FLOAT64));

    FunctionExpression sqrtExpr(Function::SQRT, std::move(args), DataType::FLOAT64);
    auto result = sqrtExpr.evaluate(*doc);
    ASSERT_DOUBLE_EQ(std::get<double>(result), 4.0);
}

TEST_F(ExpressionTest, FunctionMinMax) {
    // MIN(price, 200) = 150
    std::vector<std::unique_ptr<Expression>> minArgs;
    minArgs.push_back(std::make_unique<FieldExpression>("price", DataType::FLOAT64));
    minArgs.push_back(std::make_unique<ConstExpression>(200.0, DataType::FLOAT64));

    FunctionExpression minExpr(Function::MIN, std::move(minArgs), DataType::FLOAT64);
    auto minResult = minExpr.evaluate(*doc);
    ASSERT_DOUBLE_EQ(std::get<double>(minResult), 150.0);

    // MAX(price, 200) = 200
    std::vector<std::unique_ptr<Expression>> maxArgs;
    maxArgs.push_back(std::make_unique<FieldExpression>("price", DataType::FLOAT64));
    maxArgs.push_back(std::make_unique<ConstExpression>(200.0, DataType::FLOAT64));

    FunctionExpression maxExpr(Function::MAX, std::move(maxArgs), DataType::FLOAT64);
    auto maxResult = maxExpr.evaluate(*doc);
    ASSERT_DOUBLE_EQ(std::get<double>(maxResult), 200.0);
}

TEST_F(ExpressionTest, ComplexExpression) {
    // (price > 100) AND (quantity >= 5) OR (category == "sale")
    auto priceCheck = std::make_unique<BinaryOpExpression>(
        BinaryOp::GREATER_THAN,
        std::make_unique<FieldExpression>("price", DataType::FLOAT64),
        std::make_unique<ConstExpression>(100.0, DataType::FLOAT64),
        DataType::BOOL
    );

    auto quantityCheck = std::make_unique<BinaryOpExpression>(
        BinaryOp::GREATER_EQUAL,
        std::make_unique<FieldExpression>("quantity", DataType::INT64),
        std::make_unique<ConstExpression>(int64_t(5), DataType::INT64),
        DataType::BOOL
    );

    auto andExpr = std::make_unique<BinaryOpExpression>(
        BinaryOp::AND,
        std::move(priceCheck),
        std::move(quantityCheck),
        DataType::BOOL
    );

    auto categoryCheck = std::make_unique<BinaryOpExpression>(
        BinaryOp::EQUAL,
        std::make_unique<FieldExpression>("category", DataType::STRING),
        std::make_unique<ConstExpression>(std::string("sale"), DataType::STRING),
        DataType::BOOL
    );

    BinaryOpExpression finalExpr(
        BinaryOp::OR,
        std::move(andExpr),
        std::move(categoryCheck),
        DataType::BOOL
    );

    auto result = finalExpr.evaluate(*doc);
    ASSERT_TRUE(std::get<bool>(result));  // (150 > 100) AND (5 >= 5) = true
}

TEST_F(ExpressionTest, TypeHelpers) {
    // Test to_double
    ASSERT_DOUBLE_EQ(to_double(ExprValue(3.14)), 3.14);
    ASSERT_DOUBLE_EQ(to_double(ExprValue(int64_t(42))), 42.0);

    // Test to_int64
    ASSERT_EQ(to_int64(ExprValue(int64_t(42))), 42);
    ASSERT_EQ(to_int64(ExprValue(3.7)), 3);

    // Test to_bool
    ASSERT_TRUE(to_bool(ExprValue(true)));
    ASSERT_FALSE(to_bool(ExprValue(false)));
}
