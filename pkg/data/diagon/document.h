// Document Interface for Expression Evaluation
// Part of Diagon Search Engine
//
// This header defines the Document interface that allows expression evaluators
// to access document fields during filter evaluation.

#ifndef DIAGON_DOCUMENT_H
#define DIAGON_DOCUMENT_H

#include <string>
#include <variant>
#include <optional>
#include <vector>

namespace diagon {

// ExprValue represents a field value (matches expression_evaluator.h)
using ExprValue = std::variant<bool, int64_t, double, std::string>;

// FieldType enum for type checking
enum class FieldType {
    BOOL,
    INT64,
    DOUBLE,
    STRING,
    ARRAY,
    OBJECT,
    NULL_VALUE
};

// Document interface for expression evaluation
// All document implementations must provide these methods
class Document {
public:
    virtual ~Document() = default;

    // Get a field value by path (e.g., "price", "metadata.category")
    // Returns std::nullopt if field doesn't exist
    virtual std::optional<ExprValue> getField(const std::string& fieldPath) const = 0;

    // Check if a field exists
    virtual bool hasField(const std::string& fieldPath) const = 0;

    // Get field type (for optimization)
    virtual FieldType getFieldType(const std::string& fieldPath) const = 0;

    // Get document ID
    virtual std::string getDocumentId() const = 0;

    // Get document score (for scoring expressions)
    virtual double getScore() const = 0;

    // Compatibility aliases for expression_evaluator.cpp (snake_case)
    ExprValue get_field(const std::string& path) const {
        auto value = getField(path);
        return value.value_or(ExprValue(false));  // Return false if field not found
    }

    bool has_field(const std::string& path) const {
        return hasField(path);
    }
};

// Helper class for parsing field paths
class FieldPath {
public:
    explicit FieldPath(const std::string& path);

    // Split path into components (e.g., "a.b.c" -> ["a", "b", "c"])
    std::vector<std::string> components() const;

    // Check if path is simple (no dots)
    bool isSimple() const;

    // Get the full path
    const std::string& path() const { return path_; }

private:
    std::string path_;
    std::vector<std::string> components_;
};

// JSONDocument implementation (example)
// Represents a document stored as JSON
class JSONDocument : public Document {
public:
    // Constructor takes parsed JSON (nlohmann::json or similar)
    explicit JSONDocument(const void* jsonData, const std::string& docId);

    std::optional<ExprValue> getField(const std::string& fieldPath) const override;
    bool hasField(const std::string& fieldPath) const override;
    FieldType getFieldType(const std::string& fieldPath) const override;
    std::string getDocumentId() const override;
    double getScore() const override;

    // Set score (updated during query execution)
    void setScore(double score);

    // Get JSON data (for serialization)
    const void* getJsonData() const { return jsonData_; }

private:
    const void* jsonData_;  // Pointer to parsed JSON object
    std::string docId_;
    double score_;

    // Helper to navigate nested fields
    const void* getNestedField(const std::vector<std::string>& components) const;

    // Helper to convert JSON value to ExprValue
    std::optional<ExprValue> jsonToExprValue(const void* jsonValue) const;
};

} // namespace diagon

#endif // DIAGON_DOCUMENT_H
