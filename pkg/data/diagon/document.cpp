// Document Interface Implementation
// Part of Diagon Search Engine

#include "document.h"
#include <nlohmann/json.hpp>
#include <sstream>
#include <algorithm>

namespace diagon {

using json = nlohmann::json;

// FieldPath implementation
FieldPath::FieldPath(const std::string& path) : path_(path) {
    // Split path by dots
    std::stringstream ss(path);
    std::string component;
    while (std::getline(ss, component, '.')) {
        if (!component.empty()) {
            components_.push_back(component);
        }
    }
}

std::vector<std::string> FieldPath::components() const {
    return components_;
}

bool FieldPath::isSimple() const {
    return components_.size() == 1;
}

// JSONDocument implementation
JSONDocument::JSONDocument(const void* jsonData, const std::string& docId)
    : jsonData_(jsonData), docId_(docId), score_(0.0) {
}

std::optional<ExprValue> JSONDocument::getField(const std::string& fieldPath) const {
    try {
        // Parse field path
        FieldPath path(fieldPath);

        // Navigate to nested field
        const void* fieldValue = getNestedField(path.components());
        if (fieldValue == nullptr) {
            return std::nullopt;
        }

        // Convert JSON value to ExprValue
        return jsonToExprValue(fieldValue);
    } catch (const std::exception& e) {
        // Field access failed - return nullopt
        return std::nullopt;
    }
}

bool JSONDocument::hasField(const std::string& fieldPath) const {
    try {
        FieldPath path(fieldPath);
        return getNestedField(path.components()) != nullptr;
    } catch (const std::exception& e) {
        return false;
    }
}

FieldType JSONDocument::getFieldType(const std::string& fieldPath) const {
    try {
        FieldPath path(fieldPath);
        const void* fieldValue = getNestedField(path.components());

        if (fieldValue == nullptr) {
            return FieldType::NULL_VALUE;
        }

        auto* jsonValue = static_cast<const json*>(fieldValue);

        if (jsonValue->is_boolean()) {
            return FieldType::BOOL;
        }
        if (jsonValue->is_number_integer()) {
            return FieldType::INT64;
        }
        if (jsonValue->is_number_float()) {
            return FieldType::DOUBLE;
        }
        if (jsonValue->is_string()) {
            return FieldType::STRING;
        }
        if (jsonValue->is_array()) {
            return FieldType::ARRAY;
        }
        if (jsonValue->is_object()) {
            return FieldType::OBJECT;
        }
        if (jsonValue->is_null()) {
            return FieldType::NULL_VALUE;
        }

        return FieldType::NULL_VALUE;
    } catch (const std::exception& e) {
        return FieldType::NULL_VALUE;
    }
}

std::string JSONDocument::getDocumentId() const {
    return docId_;
}

double JSONDocument::getScore() const {
    return score_;
}

void JSONDocument::setScore(double score) {
    score_ = score;
}

const void* JSONDocument::getNestedField(const std::vector<std::string>& components) const {
    auto* jsonPtr = static_cast<const json*>(jsonData_);
    const json* current = jsonPtr;

    // Navigate nested structure
    for (const auto& component : components) {
        if (!current->is_object() || !current->contains(component)) {
            return nullptr;  // Field not found
        }
        current = &(*current)[component];
    }

    return current;
}

std::optional<ExprValue> JSONDocument::jsonToExprValue(const void* jsonValue) const {
    auto* jsonPtr = static_cast<const json*>(jsonValue);

    // Convert JSON type to ExprValue
    if (jsonPtr->is_boolean()) {
        return jsonPtr->get<bool>();
    }
    if (jsonPtr->is_number_integer()) {
        return jsonPtr->get<int64_t>();
    }
    if (jsonPtr->is_number_float()) {
        return jsonPtr->get<double>();
    }
    if (jsonPtr->is_string()) {
        return jsonPtr->get<std::string>();
    }

    // Unsupported type (array, object, null)
    return std::nullopt;
}

} // namespace diagon

/*
 * Integration Notes:
 *
 * 1. JSON Library Integration:
 *    - Recommended: nlohmann/json (header-only, easy to use)
 *    - Alternative: rapidjson (faster, but more complex API)
 *    - Alternative: simdjson (fastest, but read-only)
 *
 * 2. Performance Optimization:
 *    - Cache frequently accessed fields
 *    - Use string_view for field paths to avoid copies
 *    - Consider field index for common fields (price, timestamp, etc.)
 *
 * 3. Type System:
 *    - Support null values explicitly
 *    - Handle type conversions gracefully (int -> double, etc.)
 *    - Consider array access for future (tags[0], etc.)
 *
 * 4. Error Handling:
 *    - Return std::nullopt for missing fields (don't throw)
 *    - Log warnings for type mismatches
 *    - Graceful degradation for malformed documents
 *
 * 5. Memory Management:
 *    - Document objects are short-lived (per-query)
 *    - No need to copy JSON data (use references)
 *    - Ensure thread-safety for concurrent queries
 */
