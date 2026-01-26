// Document Store Implementation
// Part of Diagon Search Engine

#include "document_store.h"
#include <algorithm>
#include <cctype>
#include <sstream>
#include <chrono>
#include <mutex>
#include <cmath>
#include <unordered_set>
#include <ctime>
#include <map>

namespace diagon {

// Constructor
DocumentStore::DocumentStore() {
}

// Destructor
DocumentStore::~DocumentStore() {
}

// Add or update a document
bool DocumentStore::addDocument(const std::string& docId, const std::string& docJson) {
    try {
        // Parse JSON
        json parsedDoc = json::parse(docJson);

        // Create stored document
        auto storedDoc = std::make_shared<StoredDocument>(docId, parsedDoc);

        // Set index time
        auto now = std::chrono::system_clock::now();
        auto ms = std::chrono::duration_cast<std::chrono::milliseconds>(
            now.time_since_epoch()
        );
        storedDoc->indexTime = ms.count();

        // Store document
        {
            std::unique_lock<std::shared_mutex> lock(documentsMutex_);

            // If document already exists, remove it from index first
            if (documents_.find(docId) != documents_.end()) {
                removeFromIndex(docId);
            }

            documents_[docId] = storedDoc;
        }

        // Index document
        {
            std::unique_lock<std::shared_mutex> lock(indexMutex_);
            indexJsonObject(docId, "", parsedDoc);
        }

        return true;
    } catch (const std::exception& e) {
        // Failed to parse or index document
        return false;
    }
}

// Get a document by ID
std::shared_ptr<StoredDocument> DocumentStore::getDocument(const std::string& docId) const {
    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    auto it = documents_.find(docId);
    if (it != documents_.end()) {
        return it->second;
    }

    return nullptr;
}

// Delete a document
bool DocumentStore::deleteDocument(const std::string& docId) {
    std::unique_lock<std::shared_mutex> lock(documentsMutex_);

    auto it = documents_.find(docId);
    if (it == documents_.end()) {
        return false;  // Document doesn't exist
    }

    // Remove from index
    removeFromIndex(docId);

    // Remove from storage
    documents_.erase(it);

    return true;
}

// Get all document IDs
std::vector<std::string> DocumentStore::getAllDocumentIds() const {
    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    std::vector<std::string> ids;
    ids.reserve(documents_.size());

    for (const auto& pair : documents_) {
        ids.push_back(pair.first);
    }

    return ids;
}

// Get documents by IDs
std::vector<std::shared_ptr<StoredDocument>> DocumentStore::getDocuments(
    const std::vector<std::string>& docIds) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    std::vector<std::shared_ptr<StoredDocument>> docs;
    docs.reserve(docIds.size());

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it != documents_.end()) {
            docs.push_back(it->second);
        }
    }

    return docs;
}

// Search for documents matching a term
std::vector<std::string> DocumentStore::searchTerm(
    const std::string& term,
    const std::string& field) const {

    std::shared_lock<std::shared_mutex> lock(indexMutex_);

    // Convert term to lowercase for case-insensitive search
    std::string lowerTerm = term;
    std::transform(lowerTerm.begin(), lowerTerm.end(), lowerTerm.begin(),
                   [](unsigned char c){ return std::tolower(c); });

    auto it = invertedIndex_.find(lowerTerm);
    if (it == invertedIndex_.end()) {
        return {};  // Term not found
    }

    const PostingsList& postings = it->second;

    // If field specified, filter by field
    if (!field.empty()) {
        std::vector<std::string> filteredIds;
        for (const auto& pos : postings.positions) {
            if (pos.field == field) {
                // Add if not already in list
                if (std::find(filteredIds.begin(), filteredIds.end(), pos.docId)
                    == filteredIds.end()) {
                    filteredIds.push_back(pos.docId);
                }
            }
        }
        return filteredIds;
    }

    // Return all document IDs (deduplicated)
    std::vector<std::string> docIds;
    for (const auto& pos : postings.positions) {
        if (std::find(docIds.begin(), docIds.end(), pos.docId) == docIds.end()) {
            docIds.push_back(pos.docId);
        }
    }

    return docIds;
}

// Get posting list for a term
const PostingsList* DocumentStore::getPostingsList(const std::string& term) const {
    std::shared_lock<std::shared_mutex> lock(indexMutex_);

    std::string lowerTerm = term;
    std::transform(lowerTerm.begin(), lowerTerm.end(), lowerTerm.begin(),
                   [](unsigned char c){ return std::tolower(c); });

    auto it = invertedIndex_.find(lowerTerm);
    if (it != invertedIndex_.end()) {
        return &it->second;
    }

    return nullptr;
}

// Get statistics
DocumentStore::Stats DocumentStore::getStats() const {
    Stats stats;

    {
        std::shared_lock<std::shared_mutex> lock(documentsMutex_);
        stats.documentCount = documents_.size();

        // Estimate storage bytes
        stats.storageBytes = 0;
        for (const auto& pair : documents_) {
            stats.storageBytes += pair.second->data.dump().size();
        }
    }

    {
        std::shared_lock<std::shared_mutex> lock(indexMutex_);
        stats.uniqueTerms = invertedIndex_.size();
        stats.totalTerms = 0;
        for (const auto& pair : invertedIndex_) {
            stats.totalTerms += pair.second.positions.size();
        }
    }

    return stats;
}

// Clear all documents
void DocumentStore::clear() {
    {
        std::unique_lock<std::shared_mutex> lock(documentsMutex_);
        documents_.clear();
    }

    {
        std::unique_lock<std::shared_mutex> lock(indexMutex_);
        invertedIndex_.clear();
    }
}

// Helper: Index a text field
void DocumentStore::indexTextField(
    const std::string& docId,
    const std::string& fieldName,
    const std::string& text) {

    // Tokenize text
    std::vector<std::string> terms = tokenize(text);

    // Track document field length for BM25
    documentFieldLengths_[docId][fieldName] = terms.size();
    totalDocumentLength_ += terms.size();

    // Add each term to inverted index
    for (size_t position = 0; position < terms.size(); ++position) {
        const std::string& term = terms[position];

        auto& postings = invertedIndex_[term];

        // Add position
        postings.positions.emplace_back(docId, fieldName, static_cast<int>(position));

        // Update document frequency (count unique documents)
        // Note: This is approximate - should deduplicate document IDs
        postings.documentFrequency = postings.positions.size();
    }
}

// Helper: Tokenize text into terms
std::vector<std::string> DocumentStore::tokenize(const std::string& text) const {
    std::vector<std::string> terms;

    std::stringstream ss(text);
    std::string word;

    while (ss >> word) {
        // Convert to lowercase
        std::transform(word.begin(), word.end(), word.begin(),
                       [](unsigned char c){ return std::tolower(c); });

        // Remove punctuation from start and end
        while (!word.empty() && std::ispunct(word.front())) {
            word.erase(word.begin());
        }
        while (!word.empty() && std::ispunct(word.back())) {
            word.pop_back();
        }

        // Only add non-empty words
        if (!word.empty()) {
            terms.push_back(word);
        }
    }

    return terms;
}

// Helper: Remove document from inverted index
void DocumentStore::removeFromIndex(const std::string& docId) {
    // Remove all entries for this document
    for (auto& pair : invertedIndex_) {
        auto& postings = pair.second;

        // Remove positions for this document
        postings.positions.erase(
            std::remove_if(
                postings.positions.begin(),
                postings.positions.end(),
                [&docId](const TermPosition& pos) {
                    return pos.docId == docId;
                }
            ),
            postings.positions.end()
        );

        // Update document frequency
        postings.documentFrequency = postings.positions.size();
    }

    // Remove empty posting lists
    for (auto it = invertedIndex_.begin(); it != invertedIndex_.end(); ) {
        if (it->second.positions.empty()) {
            it = invertedIndex_.erase(it);
        } else {
            ++it;
        }
    }
}

// Helper: Recursively index JSON object
void DocumentStore::indexJsonObject(
    const std::string& docId,
    const std::string& fieldPrefix,
    const json& obj) {

    for (auto it = obj.begin(); it != obj.end(); ++it) {
        std::string fieldName = fieldPrefix.empty()
            ? it.key()
            : fieldPrefix + "." + it.key();

        const json& value = it.value();

        if (value.is_string()) {
            // Index string fields
            indexTextField(docId, fieldName, value.get<std::string>());
        }
        else if (value.is_object()) {
            // Recursively index nested objects
            indexJsonObject(docId, fieldName, value);
        }
        else if (value.is_array()) {
            // Index array elements as separate values
            for (size_t i = 0; i < value.size(); ++i) {
                if (value[i].is_string()) {
                    indexTextField(docId, fieldName,
                                   value[i].get<std::string>());
                }
            }
        }
        // Note: Numeric and boolean fields are not indexed
        // but are still accessible via getField()
    }
}

// BM25 scoring for a term
std::unordered_map<std::string, double> DocumentStore::scoreBM25(
    const std::string& term,
    const std::string& field,
    double k1,
    double b) const {

    std::shared_lock<std::shared_mutex> lock(indexMutex_);

    std::unordered_map<std::string, double> scores;

    // Convert term to lowercase
    std::string lowerTerm = term;
    std::transform(lowerTerm.begin(), lowerTerm.end(), lowerTerm.begin(),
                   [](unsigned char c){ return std::tolower(c); });

    auto it = invertedIndex_.find(lowerTerm);
    if (it == invertedIndex_.end()) {
        return scores;  // Term not found
    }

    const PostingsList& postings = it->second;

    // Calculate IDF: log((N - df + 0.5) / (df + 0.5))
    int64_t N = documents_.size();
    int64_t df = postings.documentFrequency;
    double idf = std::log((N - df + 0.5) / (df + 0.5) + 1.0);

    // Calculate average document length
    double avgdl = averageDocumentLength_;
    if (avgdl == 0.0 && totalDocumentLength_ > 0) {
        avgdl = static_cast<double>(totalDocumentLength_) / N;
    }

    // Count term frequencies per document
    std::unordered_map<std::string, int> termFreqs;
    for (const auto& pos : postings.positions) {
        if (field.empty() || pos.field == field) {
            termFreqs[pos.docId]++;
        }
    }

    // Calculate BM25 score for each document
    for (const auto& [docId, tf] : termFreqs) {
        // Get document length
        int docLen = 0;
        if (!field.empty()) {
            auto docIt = documentFieldLengths_.find(docId);
            if (docIt != documentFieldLengths_.end()) {
                auto fieldIt = docIt->second.find(field);
                if (fieldIt != docIt->second.end()) {
                    docLen = fieldIt->second;
                }
            }
        } else {
            // Sum all field lengths
            auto docIt = documentFieldLengths_.find(docId);
            if (docIt != documentFieldLengths_.end()) {
                for (const auto& [f, len] : docIt->second) {
                    docLen += len;
                }
            }
        }

        if (docLen == 0) {
            docLen = 1;  // Avoid division by zero
        }

        // BM25 formula: IDF * (tf * (k1 + 1)) / (tf + k1 * (1 - b + b * (docLen / avgdl)))
        double numerator = tf * (k1 + 1.0);
        double denominator = tf + k1 * (1.0 - b + b * (docLen / avgdl));
        double score = idf * (numerator / denominator);

        scores[docId] = score;
    }

    return scores;
}

// Search phrase (consecutive terms)
std::vector<std::string> DocumentStore::searchPhrase(
    const std::vector<std::string>& terms,
    const std::string& field) const {

    if (terms.empty()) {
        return {};
    }

    std::shared_lock<std::shared_mutex> lock(indexMutex_);

    // Convert terms to lowercase
    std::vector<std::string> lowerTerms;
    for (const auto& term : terms) {
        std::string lower = term;
        std::transform(lower.begin(), lower.end(), lower.begin(),
                       [](unsigned char c){ return std::tolower(c); });
        lowerTerms.push_back(lower);
    }

    // Get posting lists for all terms
    std::vector<const PostingsList*> postings;
    for (const auto& term : lowerTerms) {
        auto it = invertedIndex_.find(term);
        if (it == invertedIndex_.end()) {
            return {};  // One term not found, phrase can't match
        }
        postings.push_back(&it->second);
    }

    // Find documents where all terms appear in sequence
    std::vector<std::string> matchingDocs;
    std::unordered_map<std::string, std::vector<int>> docPositions;

    // Collect positions of first term
    for (const auto& pos : postings[0]->positions) {
        if (field.empty() || pos.field == field) {
            docPositions[pos.docId].push_back(pos.position);
        }
    }

    // For each document, check if subsequent terms appear at consecutive positions
    for (const auto& [docId, positions] : docPositions) {
        bool found = false;

        for (int startPos : positions) {
            bool matches = true;

            // Check if all other terms appear at consecutive positions
            for (size_t i = 1; i < lowerTerms.size(); ++i) {
                int expectedPos = startPos + i;
                bool foundAtPos = false;

                for (const auto& pos : postings[i]->positions) {
                    if (pos.docId == docId &&
                        (field.empty() || pos.field == field) &&
                        pos.position == expectedPos) {
                        foundAtPos = true;
                        break;
                    }
                }

                if (!foundAtPos) {
                    matches = false;
                    break;
                }
            }

            if (matches) {
                found = true;
                break;
            }
        }

        if (found) {
            matchingDocs.push_back(docId);
        }
    }

    return matchingDocs;
}

// Range query for numeric fields
std::vector<std::string> DocumentStore::searchRange(const RangeQuery& query) const {
    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    std::vector<std::string> matchingDocs;

    for (const auto& [docId, stored] : documents_) {
        try {
            // Navigate to field (support nested fields with dot notation)
            const json* current = &stored->data;
            std::string field = query.field;

            // Split field by dots for nested access
            size_t dotPos = 0;
            while ((dotPos = field.find('.')) != std::string::npos) {
                std::string key = field.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    field = field.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(field)) {
                const json& value = (*current)[field];

                // Check if value is numeric
                if (value.is_number()) {
                    double numValue = value.get<double>();

                    bool matches = true;
                    if (query.includeMin) {
                        matches = matches && (numValue >= query.min);
                    } else {
                        matches = matches && (numValue > query.min);
                    }

                    if (query.includeMax) {
                        matches = matches && (numValue <= query.max);
                    } else {
                        matches = matches && (numValue < query.max);
                    }

                    if (matches) {
                        matchingDocs.push_back(docId);
                    }
                }
            }
        } catch (const std::exception& e) {
            // Skip documents with errors
            continue;
        }
    }

    return matchingDocs;
}

// Prefix search
std::vector<std::string> DocumentStore::searchPrefix(
    const std::string& prefix,
    const std::string& field) const {

    std::shared_lock<std::shared_mutex> lock(indexMutex_);

    // Convert prefix to lowercase
    std::string lowerPrefix = prefix;
    std::transform(lowerPrefix.begin(), lowerPrefix.end(), lowerPrefix.begin(),
                   [](unsigned char c){ return std::tolower(c); });

    std::vector<std::string> matchingDocs;
    std::unordered_set<std::string> seenDocs;

    // Scan all terms in the index
    for (const auto& [term, postings] : invertedIndex_) {
        // Check if term starts with prefix
        if (term.size() >= lowerPrefix.size() &&
            term.substr(0, lowerPrefix.size()) == lowerPrefix) {

            // Add all documents containing this term
            for (const auto& pos : postings.positions) {
                if ((field.empty() || pos.field == field) &&
                    seenDocs.find(pos.docId) == seenDocs.end()) {
                    matchingDocs.push_back(pos.docId);
                    seenDocs.insert(pos.docId);
                }
            }
        }
    }

    return matchingDocs;
}

// Wildcard search
std::vector<std::string> DocumentStore::searchWildcard(
    const std::string& pattern,
    const std::string& field) const {

    std::shared_lock<std::shared_mutex> lock(indexMutex_);

    // Convert pattern to lowercase
    std::string lowerPattern = pattern;
    std::transform(lowerPattern.begin(), lowerPattern.end(), lowerPattern.begin(),
                   [](unsigned char c){ return std::tolower(c); });

    std::vector<std::string> matchingDocs;
    std::unordered_set<std::string> seenDocs;

    // Scan all terms in the index
    for (const auto& [term, postings] : invertedIndex_) {
        // Check if term matches wildcard pattern
        if (matchWildcard(term, lowerPattern)) {
            // Add all documents containing this term
            for (const auto& pos : postings.positions) {
                if ((field.empty() || pos.field == field) &&
                    seenDocs.find(pos.docId) == seenDocs.end()) {
                    matchingDocs.push_back(pos.docId);
                    seenDocs.insert(pos.docId);
                }
            }
        }
    }

    return matchingDocs;
}

// Fuzzy search using Levenshtein distance
std::vector<std::string> DocumentStore::searchFuzzy(
    const std::string& term,
    const std::string& field,
    int maxDistance) const {

    std::shared_lock<std::shared_mutex> lock(indexMutex_);

    // Convert term to lowercase
    std::string lowerTerm = term;
    std::transform(lowerTerm.begin(), lowerTerm.end(), lowerTerm.begin(),
                   [](unsigned char c){ return std::tolower(c); });

    std::vector<std::string> matchingDocs;
    std::unordered_set<std::string> seenDocs;

    // Scan all terms in the index
    for (const auto& [indexTerm, postings] : invertedIndex_) {
        // Calculate Levenshtein distance
        int distance = levenshteinDistance(lowerTerm, indexTerm);

        if (distance <= maxDistance) {
            // Add all documents containing this term
            for (const auto& pos : postings.positions) {
                if ((field.empty() || pos.field == field) &&
                    seenDocs.find(pos.docId) == seenDocs.end()) {
                    matchingDocs.push_back(pos.docId);
                    seenDocs.insert(pos.docId);
                }
            }
        }
    }

    return matchingDocs;
}

// Terms aggregation (faceting)
std::vector<DocumentStore::TermBucket> DocumentStore::aggregateTerms(
    const std::string& field,
    const std::vector<std::string>& docIds,
    int size) const {

    std::shared_lock<std::shared_mutex> lock1(documentsMutex_);
    std::shared_lock<std::shared_mutex> lock2(indexMutex_);

    std::unordered_map<std::string, int64_t> termCounts;

    // Create doc ID set for fast lookup
    std::unordered_set<std::string> docIdSet(docIds.begin(), docIds.end());

    // Count term occurrences in specified documents
    for (const auto& [term, postings] : invertedIndex_) {
        for (const auto& pos : postings.positions) {
            if ((field.empty() || pos.field == field) &&
                docIdSet.find(pos.docId) != docIdSet.end()) {
                termCounts[term]++;
            }
        }
    }

    // Convert to vector and sort by count
    std::vector<TermBucket> buckets;
    buckets.reserve(termCounts.size());

    for (const auto& [term, count] : termCounts) {
        buckets.push_back({term, count});
    }

    // Sort by count descending
    std::sort(buckets.begin(), buckets.end(),
              [](const TermBucket& a, const TermBucket& b) {
                  return a.count > b.count;
              });

    // Return top N
    if (size > 0 && size < static_cast<int>(buckets.size())) {
        buckets.resize(size);
    }

    return buckets;
}

// Stats aggregation on numeric field
DocumentStore::StatsAggregation DocumentStore::aggregateStats(
    const std::string& field,
    const std::vector<std::string>& docIds) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    StatsAggregation stats;
    stats.count = 0;
    stats.min = std::numeric_limits<double>::max();
    stats.max = std::numeric_limits<double>::lowest();
    stats.sum = 0.0;
    stats.avg = 0.0;

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            // Navigate to field (support nested fields)
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Split field by dots for nested access
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];

                // Only process numeric values
                if (value.is_number()) {
                    double numValue = value.get<double>();

                    stats.count++;
                    stats.sum += numValue;
                    stats.min = std::min(stats.min, numValue);
                    stats.max = std::max(stats.max, numValue);
                }
            }
        } catch (const std::exception& e) {
            // Skip documents with errors
            continue;
        }
    }

    // Calculate average
    if (stats.count > 0) {
        stats.avg = stats.sum / stats.count;
    } else {
        stats.min = 0.0;
        stats.max = 0.0;
    }

    return stats;
}

// Histogram aggregation (numeric buckets)
std::vector<DocumentStore::HistogramBucket> DocumentStore::aggregateHistogram(
    const std::string& field,
    const std::vector<std::string>& docIds,
    double interval) const {

    if (interval <= 0) {
        return {};
    }

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    // Collect all values and determine range
    std::vector<double> values;
    values.reserve(docIds.size());

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];
                if (value.is_number()) {
                    values.push_back(value.get<double>());
                }
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    if (values.empty()) {
        return {};
    }

    // Find min and max to determine bucket range
    double minVal = *std::min_element(values.begin(), values.end());
    double maxVal = *std::max_element(values.begin(), values.end());

    // Create buckets
    std::map<double, int64_t> bucketCounts;  // key -> count

    for (double val : values) {
        // Calculate bucket key (floor to nearest interval)
        double bucketKey = std::floor(val / interval) * interval;
        bucketCounts[bucketKey]++;
    }

    // Convert to vector
    std::vector<HistogramBucket> buckets;
    for (const auto& [key, count] : bucketCounts) {
        buckets.push_back({key, count});
    }

    return buckets;
}

// Date histogram aggregation (time-based buckets)
std::vector<DocumentStore::DateHistogramBucket> DocumentStore::aggregateDateHistogram(
    const std::string& field,
    const std::vector<std::string>& docIds,
    const std::string& interval) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    // Parse interval (e.g., "1h", "1d", "1M")
    int64_t intervalMs = 0;
    if (interval.find("ms") != std::string::npos) {
        intervalMs = std::stoll(interval.substr(0, interval.find("ms")));
    } else if (interval.find('s') != std::string::npos) {
        intervalMs = std::stoll(interval.substr(0, interval.find('s'))) * 1000;
    } else if (interval.find('m') != std::string::npos) {
        intervalMs = std::stoll(interval.substr(0, interval.find('m'))) * 60 * 1000;
    } else if (interval.find('h') != std::string::npos) {
        intervalMs = std::stoll(interval.substr(0, interval.find('h'))) * 60 * 60 * 1000;
    } else if (interval.find('d') != std::string::npos) {
        intervalMs = std::stoll(interval.substr(0, interval.find('d'))) * 24 * 60 * 60 * 1000;
    } else {
        // Default to 1 hour
        intervalMs = 60 * 60 * 1000;
    }

    // Collect timestamps
    std::map<int64_t, int64_t> bucketCounts;  // bucket timestamp -> count

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];
                if (value.is_number_integer()) {
                    int64_t timestamp = value.get<int64_t>();
                    // Floor to interval
                    int64_t bucketKey = (timestamp / intervalMs) * intervalMs;
                    bucketCounts[bucketKey]++;
                }
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    // Convert to vector with formatted dates
    std::vector<DateHistogramBucket> buckets;
    for (const auto& [key, count] : bucketCounts) {
        DateHistogramBucket bucket;
        bucket.key = key;
        bucket.docCount = count;

        // Format timestamp as ISO 8601 (simplified)
        time_t t = key / 1000;
        char buf[32];
        strftime(buf, sizeof(buf), "%Y-%m-%dT%H:%M:%SZ", gmtime(&t));
        bucket.keyAsString = buf;

        buckets.push_back(bucket);
    }

    return buckets;
}

// Percentiles aggregation
DocumentStore::PercentilesAggregation DocumentStore::aggregatePercentiles(
    const std::string& field,
    const std::vector<std::string>& docIds,
    const std::vector<double>& percentiles) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    PercentilesAggregation result;

    // Collect all values
    std::vector<double> values;
    values.reserve(docIds.size());

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];
                if (value.is_number()) {
                    values.push_back(value.get<double>());
                }
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    if (values.empty()) {
        return result;
    }

    // Sort values for percentile calculation
    std::sort(values.begin(), values.end());

    // Calculate requested percentiles
    for (double p : percentiles) {
        if (p < 0.0 || p > 100.0) {
            continue;
        }

        // Calculate index using linear interpolation
        double index = (p / 100.0) * (values.size() - 1);
        size_t lowerIndex = static_cast<size_t>(std::floor(index));
        size_t upperIndex = static_cast<size_t>(std::ceil(index));

        double percentileValue;
        if (lowerIndex == upperIndex) {
            percentileValue = values[lowerIndex];
        } else {
            // Linear interpolation between two values
            double fraction = index - lowerIndex;
            percentileValue = values[lowerIndex] * (1.0 - fraction) + values[upperIndex] * fraction;
        }

        result.values[p] = percentileValue;
    }

    return result;
}

// Cardinality aggregation (approximate unique count)
DocumentStore::CardinalityAggregation DocumentStore::aggregateCardinality(
    const std::string& field,
    const std::vector<std::string>& docIds) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    CardinalityAggregation result;

    // Use set for exact unique count (for smaller datasets)
    // For larger datasets, would use HyperLogLog
    std::unordered_set<std::string> uniqueValues;

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];

                // Convert value to string for hashing
                std::string valueStr;
                if (value.is_string()) {
                    valueStr = value.get<std::string>();
                } else if (value.is_number()) {
                    valueStr = value.dump();
                } else if (value.is_boolean()) {
                    valueStr = value.get<bool>() ? "true" : "false";
                } else {
                    valueStr = value.dump();
                }

                uniqueValues.insert(valueStr);
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    result.value = uniqueValues.size();
    return result;
}

// Extended stats aggregation
DocumentStore::ExtendedStatsAggregation DocumentStore::aggregateExtendedStats(
    const std::string& field,
    const std::vector<std::string>& docIds) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    ExtendedStatsAggregation stats;
    stats.count = 0;
    stats.min = std::numeric_limits<double>::max();
    stats.max = std::numeric_limits<double>::lowest();
    stats.sum = 0.0;
    stats.sumOfSquares = 0.0;

    std::vector<double> values;
    values.reserve(docIds.size());

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];
                if (value.is_number()) {
                    double numValue = value.get<double>();
                    values.push_back(numValue);

                    stats.count++;
                    stats.sum += numValue;
                    stats.sumOfSquares += numValue * numValue;
                    stats.min = std::min(stats.min, numValue);
                    stats.max = std::max(stats.max, numValue);
                }
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    if (stats.count > 0) {
        stats.avg = stats.sum / stats.count;

        // Calculate variance and standard deviation
        double meanSquare = stats.sumOfSquares / stats.count;
        double squareMean = stats.avg * stats.avg;
        stats.variance = meanSquare - squareMean;
        stats.stdDeviation = std::sqrt(stats.variance);

        // Standard deviation bounds (±2 sigma)
        stats.stdDeviationBounds_upper = stats.avg + 2.0 * stats.stdDeviation;
        stats.stdDeviationBounds_lower = stats.avg - 2.0 * stats.stdDeviation;
    } else {
        stats.min = 0.0;
        stats.max = 0.0;
        stats.avg = 0.0;
        stats.variance = 0.0;
        stats.stdDeviation = 0.0;
        stats.stdDeviationBounds_upper = 0.0;
        stats.stdDeviationBounds_lower = 0.0;
    }

    return stats;
}

// Simple metric aggregations

double DocumentStore::aggregateAvg(
    const std::string& field,
    const std::vector<std::string>& docIds) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    double sum = 0.0;
    int64_t count = 0;

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];
                if (value.is_number()) {
                    sum += value.get<double>();
                    count++;
                }
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    return count > 0 ? sum / count : 0.0;
}

double DocumentStore::aggregateMin(
    const std::string& field,
    const std::vector<std::string>& docIds) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    double minValue = std::numeric_limits<double>::max();
    bool found = false;

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];
                if (value.is_number()) {
                    minValue = std::min(minValue, value.get<double>());
                    found = true;
                }
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    return found ? minValue : 0.0;
}

double DocumentStore::aggregateMax(
    const std::string& field,
    const std::vector<std::string>& docIds) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    double maxValue = std::numeric_limits<double>::lowest();
    bool found = false;

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];
                if (value.is_number()) {
                    maxValue = std::max(maxValue, value.get<double>());
                    found = true;
                }
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    return found ? maxValue : 0.0;
}

double DocumentStore::aggregateSum(
    const std::string& field,
    const std::vector<std::string>& docIds) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    double sum = 0.0;

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];
                if (value.is_number()) {
                    sum += value.get<double>();
                }
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    return sum;
}

int64_t DocumentStore::aggregateValueCount(
    const std::string& field,
    const std::vector<std::string>& docIds) const {

    std::shared_lock<std::shared_mutex> lock(documentsMutex_);

    int64_t count = 0;

    for (const auto& docId : docIds) {
        auto it = documents_.find(docId);
        if (it == documents_.end()) {
            continue;
        }

        try {
            const json* current = &it->second->data;
            std::string fieldPath = field;

            // Navigate nested fields
            size_t dotPos = 0;
            while ((dotPos = fieldPath.find('.')) != std::string::npos) {
                std::string key = fieldPath.substr(0, dotPos);
                if (current->contains(key) && (*current)[key].is_object()) {
                    current = &(*current)[key];
                    fieldPath = fieldPath.substr(dotPos + 1);
                } else {
                    current = nullptr;
                    break;
                }
            }

            if (current && current->contains(fieldPath)) {
                const json& value = (*current)[fieldPath];
                // Count any non-null value
                if (!value.is_null()) {
                    count++;
                }
            }
        } catch (const std::exception& e) {
            continue;
        }
    }

    return count;
}

// Helper: Check if string matches wildcard pattern
bool DocumentStore::matchWildcard(const std::string& str, const std::string& pattern) const {
    size_t sLen = str.length();
    size_t pLen = pattern.length();

    // Dynamic programming table
    std::vector<std::vector<bool>> dp(sLen + 1, std::vector<bool>(pLen + 1, false));

    // Empty pattern matches empty string
    dp[0][0] = true;

    // Handle patterns starting with *
    for (size_t j = 1; j <= pLen; j++) {
        if (pattern[j - 1] == '*') {
            dp[0][j] = dp[0][j - 1];
        }
    }

    // Fill DP table
    for (size_t i = 1; i <= sLen; i++) {
        for (size_t j = 1; j <= pLen; j++) {
            if (pattern[j - 1] == '*') {
                // * matches zero or more characters
                dp[i][j] = dp[i][j - 1] || dp[i - 1][j];
            } else if (pattern[j - 1] == '?' || pattern[j - 1] == str[i - 1]) {
                // ? matches any single character, or exact match
                dp[i][j] = dp[i - 1][j - 1];
            }
        }
    }

    return dp[sLen][pLen];
}

// Helper: Calculate Levenshtein distance
int DocumentStore::levenshteinDistance(const std::string& s1, const std::string& s2) const {
    size_t len1 = s1.length();
    size_t len2 = s2.length();

    // Early exit for large distance
    if (std::abs(static_cast<int>(len1) - static_cast<int>(len2)) > 2) {
        return 999;  // Return large value
    }

    // Create DP table
    std::vector<std::vector<int>> dp(len1 + 1, std::vector<int>(len2 + 1));

    // Initialize base cases
    for (size_t i = 0; i <= len1; i++) {
        dp[i][0] = i;
    }
    for (size_t j = 0; j <= len2; j++) {
        dp[0][j] = j;
    }

    // Fill DP table
    for (size_t i = 1; i <= len1; i++) {
        for (size_t j = 1; j <= len2; j++) {
            if (s1[i - 1] == s2[j - 1]) {
                dp[i][j] = dp[i - 1][j - 1];  // No operation needed
            } else {
                dp[i][j] = 1 + std::min({
                    dp[i - 1][j],      // Deletion
                    dp[i][j - 1],      // Insertion
                    dp[i - 1][j - 1]   // Substitution
                });
            }
        }
    }

    return dp[len1][len2];
}

} // namespace diagon

/*
 * Performance Characteristics:
 *
 * 1. Document Storage:
 *    - O(1) insert, get, delete (hash map)
 *    - Thread-safe with shared_mutex (readers don't block each other)
 *    - Memory: ~1.5x JSON size (parsed + original)
 *
 * 2. Inverted Index:
 *    - O(k) insert where k = number of terms in document
 *    - O(1) term lookup
 *    - Positional index for phrase queries
 *    - Case-insensitive search
 *
 * 3. Tokenization:
 *    - Simple whitespace + punctuation removal
 *    - Lowercase normalization
 *    - No stemming or stopword removal (can be added)
 *
 * 4. Scalability:
 *    - In-memory only (not persistent)
 *    - Suitable for up to ~1M documents
 *    - For larger datasets, use disk-based index
 *
 * 5. Future Improvements:
 *    - BM25 scoring
 *    - Stemming (Porter stemmer)
 *    - Stopword filtering
 *    - N-gram indexing
 *    - Field-specific analyzers
 *    - Disk persistence
 */
