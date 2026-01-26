// Document Interface Unit Tests
#include "../document.h"
#include "../expression_evaluator.h"
#include <nlohmann/json.hpp>
#include <gtest/gtest.h>

using namespace diagon;
using json = nlohmann::json;

class DocumentTest : public ::testing::Test {
protected:
    void SetUp() override {
        // Create test JSON document
        testJson = json{
            {"id", "doc1"},
            {"price", 99.99},
            {"quantity", 10},
            {"in_stock", true},
            {"name", "Test Product"},
            {"metadata", {
                {"category", "electronics"},
                {"rating", 4.5},
                {"tags", {"new", "sale"}}
            }}
        };
    }

    json testJson;
};

TEST_F(DocumentTest, GetSimpleFields) {
    JSONDocument doc(&testJson, "doc1");

    // Test integer field
    auto quantity = doc.getField("quantity");
    ASSERT_TRUE(quantity.has_value());
    ASSERT_EQ(std::get<int64_t>(*quantity), 10);

    // Test double field
    auto price = doc.getField("price");
    ASSERT_TRUE(price.has_value());
    ASSERT_DOUBLE_EQ(std::get<double>(*price), 99.99);

    // Test boolean field
    auto inStock = doc.getField("in_stock");
    ASSERT_TRUE(inStock.has_value());
    ASSERT_TRUE(std::get<bool>(*inStock));

    // Test string field
    auto name = doc.getField("name");
    ASSERT_TRUE(name.has_value());
    ASSERT_EQ(std::get<std::string>(*name), "Test Product");
}

TEST_F(DocumentTest, GetNestedFields) {
    JSONDocument doc(&testJson, "doc1");

    // Test nested string
    auto category = doc.getField("metadata.category");
    ASSERT_TRUE(category.has_value());
    ASSERT_EQ(std::get<std::string>(*category), "electronics");

    // Test nested double
    auto rating = doc.getField("metadata.rating");
    ASSERT_TRUE(rating.has_value());
    ASSERT_DOUBLE_EQ(std::get<double>(*rating), 4.5);
}

TEST_F(DocumentTest, GetNonExistentField) {
    JSONDocument doc(&testJson, "doc1");

    auto field = doc.getField("nonexistent");
    ASSERT_FALSE(field.has_value());

    auto nestedField = doc.getField("metadata.nonexistent");
    ASSERT_FALSE(nestedField.has_value());
}

TEST_F(DocumentTest, HasField) {
    JSONDocument doc(&testJson, "doc1");

    ASSERT_TRUE(doc.hasField("price"));
    ASSERT_TRUE(doc.hasField("metadata.category"));
    ASSERT_FALSE(doc.hasField("nonexistent"));
    ASSERT_FALSE(doc.hasField("metadata.nonexistent"));
}

TEST_F(DocumentTest, GetFieldType) {
    JSONDocument doc(&testJson, "doc1");

    ASSERT_EQ(doc.getFieldType("quantity"), FieldType::INT64);
    ASSERT_EQ(doc.getFieldType("price"), FieldType::DOUBLE);
    ASSERT_EQ(doc.getFieldType("in_stock"), FieldType::BOOL);
    ASSERT_EQ(doc.getFieldType("name"), FieldType::STRING);
    ASSERT_EQ(doc.getFieldType("metadata"), FieldType::OBJECT);
    ASSERT_EQ(doc.getFieldType("metadata.tags"), FieldType::ARRAY);
    ASSERT_EQ(doc.getFieldType("nonexistent"), FieldType::NULL_VALUE);
}

TEST_F(DocumentTest, DocumentMetadata) {
    JSONDocument doc(&testJson, "doc123");

    ASSERT_EQ(doc.getDocumentId(), "doc123");
    ASSERT_DOUBLE_EQ(doc.getScore(), 0.0);

    doc.setScore(0.95);
    ASSERT_DOUBLE_EQ(doc.getScore(), 0.95);
}

TEST_F(DocumentTest, FieldPathParsing) {
    FieldPath simple("price");
    ASSERT_TRUE(simple.isSimple());
    ASSERT_EQ(simple.components().size(), 1);
    ASSERT_EQ(simple.components()[0], "price");

    FieldPath nested("metadata.category");
    ASSERT_FALSE(nested.isSimple());
    ASSERT_EQ(nested.components().size(), 2);
    ASSERT_EQ(nested.components()[0], "metadata");
    ASSERT_EQ(nested.components()[1], "category");

    FieldPath deepNested("a.b.c.d");
    ASSERT_EQ(deepNested.components().size(), 4);
}

TEST_F(DocumentTest, TypeConversionHelpers) {
    // Test to_bool
    ASSERT_TRUE(to_bool(ExprValue(true)));
    ASSERT_FALSE(to_bool(ExprValue(false)));
    ASSERT_TRUE(to_bool(ExprValue(int64_t(1))));
    ASSERT_FALSE(to_bool(ExprValue(int64_t(0))));
    ASSERT_TRUE(to_bool(ExprValue(1.5)));
    ASSERT_FALSE(to_bool(ExprValue(0.0)));
    ASSERT_TRUE(to_bool(ExprValue(std::string("test"))));
    ASSERT_FALSE(to_bool(ExprValue(std::string(""))));

    // Test to_double
    ASSERT_DOUBLE_EQ(to_double(ExprValue(3.14)), 3.14);
    ASSERT_DOUBLE_EQ(to_double(ExprValue(int64_t(42))), 42.0);
    ASSERT_DOUBLE_EQ(to_double(ExprValue(true)), 1.0);
    ASSERT_DOUBLE_EQ(to_double(ExprValue(false)), 0.0);

    // Test to_int64
    ASSERT_EQ(to_int64(ExprValue(int64_t(42))), 42);
    ASSERT_EQ(to_int64(ExprValue(3.7)), 3);
    ASSERT_EQ(to_int64(ExprValue(true)), 1);
    ASSERT_EQ(to_int64(ExprValue(false)), 0);
}

TEST_F(DocumentTest, ErrorHandling) {
    JSONDocument doc(&testJson, "doc1");

    // Invalid path should return nullopt
    auto invalidPath = doc.getField("a..b");
    ASSERT_FALSE(invalidPath.has_value());

    // Type mismatch - requesting array/object as scalar should return nullopt
    auto arrayField = doc.getField("metadata.tags");
    ASSERT_FALSE(arrayField.has_value());  // Arrays not convertible to ExprValue

    auto objectField = doc.getField("metadata");
    ASSERT_FALSE(objectField.has_value());  // Objects not convertible to ExprValue
}
