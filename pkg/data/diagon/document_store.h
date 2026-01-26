// Document Store - In-Memory Document Storage
// Part of Diagon Search Engine
//
// Provides efficient document storage and retrieval with basic indexing

#ifndef DIAGON_DOCUMENT_STORE_H
#define DIAGON_DOCUMENT_STORE_H

#include <string>
#include <unordered_map>
#include <memory>
#include <shared_mutex>
#include <nlohmann/json.hpp>
#include "document.h"

namespace diagon {

using json = nlohmann::json;

// StoredDocument represents a document in the store
struct StoredDocument {
    std::string docId;
    json data;              // Parsed JSON document
    double score;           // BM25 or other scoring
    int64_t indexTime;      // When document was indexed (ms since epoch)

    StoredDocument() : score(0.0), indexTime(0) {}

    StoredDocument(const std::string& id, const json& jsonData)
        : docId(id), data(jsonData), score(0.0), indexTime(0) {}
};

// Term position for positional indexing
struct TermPosition {
    std::string docId;
    std::string field;      // Which field contains this term
    int position;           // Position within field

    TermPosition(const std::string& d, const std::string& f, int p)
        : docId(d), field(f), position(p) {}
};

// Inverted index entry
struct PostingsList {
    int64_t documentFrequency;              // Number of documents with this term
    std::vector<TermPosition> positions;    // All positions of this term

    PostingsList() : documentFrequency(0) {}
};

// DocumentStore manages document storage and retrieval
class DocumentStore {
public:
    DocumentStore();
    ~DocumentStore();

    // Add or update a document
    // Returns true on success
    bool addDocument(const std::string& docId, const std::string& docJson);

    // Get a document by ID
    // Returns nullptr if not found
    std::shared_ptr<StoredDocument> getDocument(const std::string& docId) const;

    // Delete a document
    // Returns true if document existed
    bool deleteDocument(const std::string& docId);

    // Get all document IDs (for iteration)
    std::vector<std::string> getAllDocumentIds() const;

    // Get documents by IDs (batch retrieval)
    std::vector<std::shared_ptr<StoredDocument>> getDocuments(
        const std::vector<std::string>& docIds) const;

    // Search for documents matching a term
    // Returns document IDs containing the term
    std::vector<std::string> searchTerm(const std::string& term,
                                         const std::string& field = "") const;

    // Get posting list for a term
    const PostingsList* getPostingsList(const std::string& term) const;

    // BM25 scoring for a term in documents
    // Returns map of docId -> BM25 score
    std::unordered_map<std::string, double> scoreBM25(
        const std::string& term,
        const std::string& field = "",
        double k1 = 1.2,
        double b = 0.75) const;

    // Search phrase (consecutive terms)
    // Returns document IDs containing the exact phrase
    std::vector<std::string> searchPhrase(
        const std::vector<std::string>& terms,
        const std::string& field = "") const;

    // Range query for numeric fields
    struct RangeQuery {
        std::string field;
        double min;
        double max;
        bool includeMin;
        bool includeMax;
    };
    std::vector<std::string> searchRange(const RangeQuery& query) const;

    // Prefix search
    std::vector<std::string> searchPrefix(
        const std::string& prefix,
        const std::string& field = "") const;

    // Wildcard search (* and ?)
    std::vector<std::string> searchWildcard(
        const std::string& pattern,
        const std::string& field = "") const;

    // Fuzzy search (Levenshtein distance)
    std::vector<std::string> searchFuzzy(
        const std::string& term,
        const std::string& field = "",
        int maxDistance = 2) const;

    // Aggregations
    struct TermBucket {
        std::string term;
        int64_t count;
    };

    struct StatsAggregation {
        int64_t count;
        double min;
        double max;
        double avg;
        double sum;
    };

    struct HistogramBucket {
        double key;           // Lower bound of bucket
        int64_t docCount;     // Number of documents in bucket
    };

    struct DateHistogramBucket {
        int64_t key;          // Timestamp (milliseconds since epoch)
        int64_t docCount;     // Number of documents in bucket
        std::string keyAsString;  // Human-readable date
    };

    struct PercentilesAggregation {
        std::unordered_map<double, double> values;  // percentile -> value
    };

    struct CardinalityAggregation {
        int64_t value;        // Approximate unique count
    };

    struct ExtendedStatsAggregation {
        int64_t count;
        double min;
        double max;
        double avg;
        double sum;
        double sumOfSquares;
        double variance;
        double stdDeviation;
        double stdDeviationBounds_upper;
        double stdDeviationBounds_lower;
    };

    struct RangeBucket {
        std::string key;          // Range label (e.g., "0-50", "50-100", "*-50")
        double from;              // Lower bound (or -infinity if !fromSet)
        double to;                // Upper bound (or +infinity if !toSet)
        bool fromSet;             // Whether 'from' is specified
        bool toSet;               // Whether 'to' is specified
        int64_t docCount;         // Number of documents in range
    };

    // Terms aggregation (faceting)
    std::vector<TermBucket> aggregateTerms(
        const std::string& field,
        const std::vector<std::string>& docIds,
        int size = 10) const;

    // Stats aggregation on numeric field
    StatsAggregation aggregateStats(
        const std::string& field,
        const std::vector<std::string>& docIds) const;

    // Histogram aggregation (numeric buckets)
    std::vector<HistogramBucket> aggregateHistogram(
        const std::string& field,
        const std::vector<std::string>& docIds,
        double interval) const;

    // Date histogram aggregation (time-based buckets)
    std::vector<DateHistogramBucket> aggregateDateHistogram(
        const std::string& field,
        const std::vector<std::string>& docIds,
        const std::string& interval) const;

    // Percentiles aggregation (50th, 95th, 99th, etc.)
    PercentilesAggregation aggregatePercentiles(
        const std::string& field,
        const std::vector<std::string>& docIds,
        const std::vector<double>& percentiles = {50.0, 95.0, 99.0}) const;

    // Cardinality aggregation (approximate unique count)
    CardinalityAggregation aggregateCardinality(
        const std::string& field,
        const std::vector<std::string>& docIds) const;

    // Extended stats aggregation (includes variance, std deviation)
    ExtendedStatsAggregation aggregateExtendedStats(
        const std::string& field,
        const std::vector<std::string>& docIds) const;

    // Simple metric aggregations (single-value metrics)

    // Average aggregation (single metric)
    double aggregateAvg(
        const std::string& field,
        const std::vector<std::string>& docIds) const;

    // Min aggregation (single metric)
    double aggregateMin(
        const std::string& field,
        const std::vector<std::string>& docIds) const;

    // Max aggregation (single metric)
    double aggregateMax(
        const std::string& field,
        const std::vector<std::string>& docIds) const;

    // Sum aggregation (single metric)
    double aggregateSum(
        const std::string& field,
        const std::vector<std::string>& docIds) const;

    // Value count aggregation (count non-null values)
    int64_t aggregateValueCount(
        const std::string& field,
        const std::vector<std::string>& docIds) const;

    // Range aggregation (count documents in numeric ranges)
    std::vector<RangeBucket> aggregateRange(
        const std::string& field,
        const std::vector<RangeBucket>& ranges,
        const std::vector<std::string>& docIds) const;

    // Statistics
    struct Stats {
        int64_t documentCount;
        int64_t totalTerms;
        int64_t uniqueTerms;
        int64_t storageBytes;
    };
    Stats getStats() const;

    // Clear all documents (for testing)
    void clear();

private:
    // Document storage
    mutable std::shared_mutex documentsMutex_;
    std::unordered_map<std::string, std::shared_ptr<StoredDocument>> documents_;

    // Inverted index: term -> postings list
    mutable std::shared_mutex indexMutex_;
    std::unordered_map<std::string, PostingsList> invertedIndex_;

    // Document field lengths (for BM25 scoring)
    std::unordered_map<std::string, std::unordered_map<std::string, int>> documentFieldLengths_;
    double averageDocumentLength_ = 0.0;
    int64_t totalDocumentLength_ = 0;

    // Helper: Extract and index text from a field
    void indexTextField(const std::string& docId,
                        const std::string& fieldName,
                        const std::string& text);

    // Helper: Tokenize text into terms
    std::vector<std::string> tokenize(const std::string& text) const;

    // Helper: Remove document from inverted index
    void removeFromIndex(const std::string& docId);

    // Helper: Recursively index JSON object
    void indexJsonObject(const std::string& docId,
                         const std::string& fieldPrefix,
                         const json& obj);

    // Helper: Check if string matches wildcard pattern
    bool matchWildcard(const std::string& str, const std::string& pattern) const;

    // Helper: Calculate Levenshtein distance
    int levenshteinDistance(const std::string& s1, const std::string& s2) const;
};

} // namespace diagon

#endif // DIAGON_DOCUMENT_STORE_H
