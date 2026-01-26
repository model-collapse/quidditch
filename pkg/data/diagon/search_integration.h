// Search Integration with Expression Evaluator
// Part of Diagon Search Engine
//
// This header shows how to integrate the expression evaluator
// into the search loop for native filter evaluation.

#ifndef DIAGON_SEARCH_INTEGRATION_H
#define DIAGON_SEARCH_INTEGRATION_H

#include "document.h"
#include "expression_evaluator.h"
#include "document_store.h"
#include <memory>
#include <vector>
#include <cstdint>
#include <unordered_map>

namespace diagon {

// SearchOptions for controlling search behavior
struct SearchOptions {
    int from = 0;                    // Offset for pagination
    int size = 10;                   // Number of results to return
    bool trackTotalHits = true;      // Count all matching documents
    const uint8_t* filterExpr = nullptr;  // Optional filter expression
    size_t filterExprLen = 0;        // Filter expression length
};

// Aggregation results
struct AggregationResult {
    std::string name;
    std::string type;  // "terms", "stats", "histogram", "date_histogram", "percentiles", "cardinality", "extended_stats", "avg", "min", "max", "sum", "value_count"

    // Terms aggregation buckets
    std::vector<std::pair<std::string, int64_t>> buckets;

    // Stats aggregation values
    int64_t count = 0;
    double min = 0.0;
    double max = 0.0;
    double avg = 0.0;
    double sum = 0.0;

    // Histogram aggregation buckets
    std::vector<DocumentStore::HistogramBucket> histogramBuckets;

    // Date histogram aggregation buckets
    std::vector<DocumentStore::DateHistogramBucket> dateHistogramBuckets;

    // Percentiles aggregation values (percentile -> value)
    std::unordered_map<double, double> percentiles;

    // Cardinality aggregation value
    int64_t cardinality = 0;

    // Extended stats aggregation (additional fields beyond basic stats)
    double sumOfSquares = 0.0;
    double variance = 0.0;
    double stdDeviation = 0.0;
    double stdDeviationBounds_upper = 0.0;
    double stdDeviationBounds_lower = 0.0;

    // Simple metric aggregations (single-value metrics)
    // For avg, min, max, sum: use the existing fields above
    // For value_count: use 'count' field
    double value = 0.0;  // Generic value field for single-metric aggregations
};

// SearchResult represents the result of a search query
struct SearchResult {
    int64_t totalHits = 0;           // Total matching documents
    double maxScore = 0.0;           // Highest score in results
    int64_t took = 0;                // Time taken in milliseconds
    std::vector<std::shared_ptr<Document>> hits;  // Result documents
    std::unordered_map<std::string, AggregationResult> aggregations;  // Aggregation results
};

// ExpressionFilter wraps the expression evaluator for search
class ExpressionFilter {
public:
    // Create filter from serialized expression
    static std::unique_ptr<ExpressionFilter> create(
        const uint8_t* exprData,
        size_t exprLen
    );

    // Check if document matches the filter
    bool matches(const Document& doc) const;

    // Get statistics
    uint64_t getEvaluationCount() const { return evaluationCount_; }
    uint64_t getMatchCount() const { return matchCount_; }

private:
    ExpressionFilter(std::unique_ptr<Expression> expr);

    std::unique_ptr<Expression> expr_;
    mutable uint64_t evaluationCount_ = 0;
    mutable uint64_t matchCount_ = 0;
};

// Forward declaration
class DocumentStore;

// SearchIntegration - provides search functionality for a DocumentStore
class SearchIntegration {
public:
    explicit SearchIntegration(std::shared_ptr<DocumentStore> store);
    ~SearchIntegration() = default;

    // Execute search query
    SearchResult search(
        const std::string& queryJson,
        const uint8_t* filterExpr,
        size_t filterExprLen,
        int from,
        int size
    );

private:
    std::shared_ptr<DocumentStore> store_;

    // Helper: Execute search without filter
    SearchResult searchWithoutFilter(
        const std::string& queryJson,
        int from,
        int size
    );
};

// Shard represents a search shard
class Shard {
public:
    Shard(const std::string& path);
    ~Shard();

    // Execute search with optional filter expression
    SearchResult search(
        const std::string& queryJson,
        const SearchOptions& options
    );

    // Index a document
    bool indexDocument(const std::string& docId, const std::string& docJson);

    // Get a document by ID
    std::shared_ptr<Document> getDocument(const std::string& docId);

    // Get document as JSON string
    std::string getDocumentJson(const std::string& docId) const;

    // Delete a document
    bool deleteDocument(const std::string& docId);

    // Get statistics
    struct Stats {
        int64_t docCount = 0;
        int64_t sizeBytes = 0;
        int64_t searchCount = 0;
        int64_t filterEvaluations = 0;
        int64_t uniqueTerms = 0;
        int64_t totalTerms = 0;
    };
    Stats getStats() const;

    // Get document store (for distributed search)
    std::shared_ptr<DocumentStore> getDocumentStore() const;

private:
    std::string path_;

    // Document storage
    std::unique_ptr<DocumentStore> documentStore_;

    // Statistics
    mutable Stats stats_;

    // Helper: Execute search without filter
    SearchResult searchWithoutFilter(const std::string& queryJson, const SearchOptions& options);

    // Helper: Apply filter to documents
    std::vector<std::shared_ptr<Document>> applyFilter(
        const std::vector<std::shared_ptr<Document>>& candidates,
        const ExpressionFilter& filter
    );

    // Helper: Convert StoredDocument to Document for evaluation
    std::shared_ptr<Document> storedToDocument(
        const std::shared_ptr<struct StoredDocument>& stored) const;
};

// C API for Go integration
extern "C" {

// Opaque types for C API
typedef struct diagon_shard_t diagon_shard_t;
typedef struct diagon_filter_t diagon_filter_t;
typedef struct diagon_shard_manager_t diagon_shard_manager_t;
typedef struct diagon_distributed_coordinator_t diagon_distributed_coordinator_t;

// Create/destroy shard
diagon_shard_t* diagon_create_shard(const char* path);
void diagon_destroy_shard(diagon_shard_t* shard);

// Search with filter expression
// Returns JSON string with search results (caller must free)
char* diagon_search_with_filter(
    diagon_shard_t* shard,
    const char* query_json,
    const uint8_t* filter_expr,
    size_t filter_expr_len,
    int from,
    int size
);

// Create/destroy filter (for reuse across queries)
diagon_filter_t* diagon_create_filter(const uint8_t* expr_data, size_t expr_len);
void diagon_destroy_filter(diagon_filter_t* filter);

// Check if document matches filter
int diagon_filter_matches(diagon_filter_t* filter, const char* doc_json);

// Get filter statistics
void diagon_filter_stats(
    diagon_filter_t* filter,
    uint64_t* evaluation_count,
    uint64_t* match_count
);

// Document operations
int diagon_index_document(
    diagon_shard_t* shard,
    const char* doc_id,
    const char* doc_json
);

char* diagon_get_document(
    diagon_shard_t* shard,
    const char* doc_id
);

int diagon_delete_document(
    diagon_shard_t* shard,
    const char* doc_id
);

// Index management
int diagon_refresh(diagon_shard_t* shard);
int diagon_flush(diagon_shard_t* shard);

// Get shard statistics
char* diagon_get_stats(diagon_shard_t* shard);

// Distributed search API
diagon_shard_manager_t* diagon_create_shard_manager(const char* node_id, int total_shards);
void diagon_destroy_shard_manager(diagon_shard_manager_t* manager);
int diagon_register_shard(diagon_shard_manager_t* manager, int shard_index, diagon_shard_t* shard, int is_primary);
int diagon_get_shard_for_document(diagon_shard_manager_t* manager, const char* doc_id);

diagon_distributed_coordinator_t* diagon_create_coordinator(diagon_shard_manager_t* manager);
void diagon_destroy_coordinator(diagon_distributed_coordinator_t* coordinator);
char* diagon_distributed_search(diagon_distributed_coordinator_t* coordinator, const char* query_json, const uint8_t* filter_expr, size_t filter_expr_len, int from, int size);

} // extern "C"

} // namespace diagon

#endif // DIAGON_SEARCH_INTEGRATION_H
