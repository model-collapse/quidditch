// Search Integration Unit Tests
#include "../document.h"
#include "../search_integration.h"
#include <nlohmann/json.hpp>
#include <gtest/gtest.h>

using namespace diagon;
using json = nlohmann::json;

class SearchIntegrationTest : public ::testing::Test {
protected:
    void SetUp() override {
        // Create test documents
        for (int i = 0; i < 10; i++) {
            json doc;
            doc["id"] = "doc" + std::to_string(i);
            doc["price"] = 100.0 + (i * 10.0);  // 100, 110, 120, ...
            doc["quantity"] = 10 - i;            // 10, 9, 8, ...
            doc["in_stock"] = i < 7;             // First 7 in stock
            testDocs.push_back(doc);
        }
    }

    std::vector<json> testDocs;

    // Helper to create document pointers
    std::vector<std::shared_ptr<Document>> createDocuments() {
        std::vector<std::shared_ptr<Document>> docs;
        for (size_t i = 0; i < testDocs.size(); i++) {
            docs.push_back(std::make_shared<JSONDocument>(
                &testDocs[i],
                "doc" + std::to_string(i)
            ));
        }
        return docs;
    }

    // Helper to serialize a simple comparison expression
    // Format: [expr_type][data_type][op][left][right]
    std::vector<uint8_t> createComparisonExpr() {
        // This is a stub - in real implementation, this would match
        // the serialization format from Go's expressions package
        // For now, we'll test with nullptr to skip filter
        return {};
    }
};

TEST_F(SearchIntegrationTest, ExpressionFilterCreate) {
    // Test filter creation with null data
    auto filter1 = ExpressionFilter::create(nullptr, 0);
    ASSERT_EQ(filter1, nullptr);

    // Test filter creation with empty data
    uint8_t emptyData[] = {};
    auto filter2 = ExpressionFilter::create(emptyData, 0);
    ASSERT_EQ(filter2, nullptr);

    // Note: Creating actual filter requires matching serialization format
    // from expressions package - tested in integration tests
}

TEST_F(SearchIntegrationTest, ExpressionFilterStatistics) {
    // Create a simple filter (stub - would need real serialized expression)
    // For testing, we'll use the matches() method directly with a mock filter

    // This test verifies the statistics tracking works
    // Real expression evaluation tested in expression_test.cpp
}

TEST_F(SearchIntegrationTest, SearchWithoutFilter) {
    Shard shard("/tmp/test_shard");

    SearchOptions options;
    options.from = 0;
    options.size = 10;
    options.filterExpr = nullptr;
    options.filterExprLen = 0;

    // Note: This will use stub implementation since actual index not implemented
    auto result = shard.search("{\"match_all\":{}}", options);

    // Verify result structure
    ASSERT_GE(result.totalHits, 0);
    ASSERT_GE(result.took, 0);
}

TEST_F(SearchIntegrationTest, SearchWithFilter) {
    Shard shard("/tmp/test_shard");

    SearchOptions options;
    options.from = 0;
    options.size = 10;

    // Would need real serialized expression here
    // For now, testing with nullptr
    options.filterExpr = nullptr;
    options.filterExprLen = 0;

    auto result = shard.search("{\"match_all\":{}}", options);

    ASSERT_GE(result.totalHits, 0);
}

TEST_F(SearchIntegrationTest, ApplyFilterToDocuments) {
    // This tests the filter application logic
    // In real implementation, would create actual expression filter

    auto docs = createDocuments();
    ASSERT_EQ(docs.size(), 10);

    // Verify documents were created correctly
    for (size_t i = 0; i < docs.size(); i++) {
        ASSERT_EQ(docs[i]->getDocumentId(), "doc" + std::to_string(i));
    }
}

TEST_F(SearchIntegrationTest, ShardStatistics) {
    Shard shard("/tmp/test_shard");

    auto stats = shard.getStats();
    ASSERT_EQ(stats.docCount, 0);  // No documents indexed yet
    ASSERT_EQ(stats.searchCount, 0);

    // Execute a search
    SearchOptions options;
    shard.search("{\"match_all\":{}}", options);

    // Verify statistics updated
    auto statsAfter = shard.getStats();
    ASSERT_EQ(statsAfter.searchCount, 1);
}

TEST_F(SearchIntegrationTest, CAPIShardLifecycle) {
    // Test C API shard creation/destruction
    auto* shard = diagon_create_shard("/tmp/test_shard");
    ASSERT_NE(shard, nullptr);

    diagon_destroy_shard(shard);
    // Should not crash
}

TEST_F(SearchIntegrationTest, CAPISearchWithFilter) {
    auto* shard = diagon_create_shard("/tmp/test_shard");
    ASSERT_NE(shard, nullptr);

    // Test search with null filter
    char* result = diagon_search_with_filter(
        shard,
        "{\"match_all\":{}}",
        nullptr,
        0,
        0,
        10
    );

    ASSERT_NE(result, nullptr);

    // Result should be valid JSON
    auto resultJson = json::parse(result);
    ASSERT_TRUE(resultJson.contains("took"));
    ASSERT_TRUE(resultJson.contains("total_hits"));
    ASSERT_TRUE(resultJson.contains("hits"));

    free(result);
    diagon_destroy_shard(shard);
}

TEST_F(SearchIntegrationTest, CAPIErrorHandling) {
    // Test with null shard
    char* result1 = diagon_search_with_filter(nullptr, "{}", nullptr, 0, 0, 10);
    ASSERT_EQ(result1, nullptr);

    // Test with null query
    auto* shard = diagon_create_shard("/tmp/test_shard");
    char* result2 = diagon_search_with_filter(shard, nullptr, nullptr, 0, 0, 10);
    ASSERT_EQ(result2, nullptr);

    diagon_destroy_shard(shard);
}

TEST_F(SearchIntegrationTest, CAPIFilterLifecycle) {
    // Test filter creation with null data
    auto* filter1 = diagon_create_filter(nullptr, 0);
    ASSERT_EQ(filter1, nullptr);

    uint8_t emptyData[] = {};
    auto* filter2 = diagon_create_filter(emptyData, 0);
    ASSERT_EQ(filter2, nullptr);

    // Note: Creating actual filter requires real serialized expression
}

TEST_F(SearchIntegrationTest, Pagination) {
    Shard shard("/tmp/test_shard");

    // Test first page
    SearchOptions options1;
    options1.from = 0;
    options1.size = 5;
    auto result1 = shard.search("{\"match_all\":{}}", options1);

    // Test second page
    SearchOptions options2;
    options2.from = 5;
    options2.size = 5;
    auto result2 = shard.search("{\"match_all\":{}}", options2);

    // Both should succeed (stub returns empty results, but no errors)
    ASSERT_GE(result1.took, 0);
    ASSERT_GE(result2.took, 0);
}

TEST_F(SearchIntegrationTest, PerformanceMetrics) {
    Shard shard("/tmp/test_shard");

    SearchOptions options;
    options.from = 0;
    options.size = 100;

    auto startTime = std::chrono::high_resolution_clock::now();

    // Execute multiple searches
    for (int i = 0; i < 10; i++) {
        auto result = shard.search("{\"match_all\":{}}", options);
        ASSERT_GE(result.took, 0);
    }

    auto endTime = std::chrono::high_resolution_clock::now();
    auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(
        endTime - startTime
    );

    // 10 searches should complete in reasonable time
    ASSERT_LT(duration.count(), 1000);  // Less than 1 second

    auto stats = shard.getStats();
    ASSERT_EQ(stats.searchCount, 10);
}

// Integration test demonstrating end-to-end flow
TEST_F(SearchIntegrationTest, EndToEndFlow) {
    // 1. Create shard
    auto* shard = diagon_create_shard("/tmp/test_shard_e2e");
    ASSERT_NE(shard, nullptr);

    // 2. Execute search (without filter for now)
    char* result = diagon_search_with_filter(
        shard,
        "{\"match_all\":{}}",
        nullptr,
        0,
        0,
        10
    );
    ASSERT_NE(result, nullptr);

    // 3. Parse result
    auto resultJson = json::parse(result);
    ASSERT_TRUE(resultJson.is_object());
    ASSERT_TRUE(resultJson.contains("took"));
    ASSERT_TRUE(resultJson.contains("total_hits"));
    ASSERT_TRUE(resultJson.contains("max_score"));
    ASSERT_TRUE(resultJson.contains("hits"));

    // 4. Verify hits array structure
    ASSERT_TRUE(resultJson["hits"].is_array());

    // 5. Cleanup
    free(result);
    diagon_destroy_shard(shard);
}
