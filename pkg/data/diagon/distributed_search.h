#pragma once

#include "shard_manager.h"
#include "search_integration.h"
#include <string>
#include <vector>
#include <memory>
#include <future>

namespace diagon {

/**
 * ShardSearchResult - Result from a single shard
 */
struct ShardSearchResult {
    int shardIndex;
    std::string nodeId;
    SearchResult result;
    bool success;
    std::string error;
    int64_t latencyMs;
};

/**
 * DistributedSearchCoordinator - Coordinates distributed search across shards
 *
 * Responsibilities:
 * - Route queries to appropriate shards
 * - Execute queries in parallel across shards
 * - Merge results from multiple shards
 * - Handle partial failures
 * - Aggregate aggregations across shards
 */
class DistributedSearchCoordinator {
public:
    /**
     * Constructor
     * @param shardManager - Shard manager for routing
     */
    explicit DistributedSearchCoordinator(std::shared_ptr<ShardManager> shardManager);
    ~DistributedSearchCoordinator() = default;

    /**
     * Execute a distributed search across shards
     *
     * @param query - Query JSON
     * @param filterExprBytes - Filter expression bytes
     * @param filterExprLen - Filter expression length
     * @param from - Result offset
     * @param size - Number of results to return
     * @return Merged search results
     */
    SearchResult search(
        const std::string& query,
        const uint8_t* filterExprBytes,
        size_t filterExprLen,
        int from,
        int size);

    /**
     * Execute search on a specific shard (local)
     *
     * @param shardIndex - Shard to query
     * @param query - Query JSON
     * @param filterExprBytes - Filter expression bytes
     * @param filterExprLen - Filter expression length
     * @param from - Result offset
     * @param size - Number of results to return
     * @return Search result from shard
     */
    ShardSearchResult searchShard(
        int shardIndex,
        const std::string& query,
        const uint8_t* filterExprBytes,
        size_t filterExprLen,
        int from,
        int size);

private:
    /**
     * Merge results from multiple shards
     *
     * Handles:
     * - Score-based ranking across shards
     * - Total hits aggregation
     * - Pagination (from/size)
     * - Max score calculation
     * - Aggregation merging
     *
     * @param shardResults - Results from individual shards
     * @param from - Result offset
     * @param size - Number of results to return
     * @return Merged search result
     */
    SearchResult mergeResults(
        const std::vector<ShardSearchResult>& shardResults,
        int from,
        int size);

    /**
     * Merge aggregations from multiple shards
     *
     * @param shardResults - Results from individual shards
     * @return Merged aggregations
     */
    std::unordered_map<std::string, AggregationResult> mergeAggregations(
        const std::vector<ShardSearchResult>& shardResults);

    /**
     * Merge terms aggregations from multiple shards
     *
     * @param aggResults - Aggregation results from shards
     * @param size - Number of top terms to return
     * @return Merged terms aggregation
     */
    AggregationResult mergeTermsAggregation(
        const std::vector<AggregationResult>& aggResults,
        int size);

    /**
     * Merge stats aggregations from multiple shards
     *
     * @param aggResults - Aggregation results from shards
     * @return Merged stats aggregation
     */
    AggregationResult mergeStatsAggregation(
        const std::vector<AggregationResult>& aggResults);

    std::shared_ptr<ShardManager> shardManager_;
};

} // namespace diagon
