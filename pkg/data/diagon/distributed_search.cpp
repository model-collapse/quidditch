#include "distributed_search.h"
#include "document_store.h"
#include <algorithm>
#include <chrono>
#include <thread>
#include <unordered_map>

namespace diagon {

DistributedSearchCoordinator::DistributedSearchCoordinator(std::shared_ptr<ShardManager> shardManager)
    : shardManager_(shardManager) {
    if (!shardManager_) {
        throw std::invalid_argument("ShardManager cannot be null");
    }
}

SearchResult DistributedSearchCoordinator::search(
    const std::string& query,
    const uint8_t* filterExprBytes,
    size_t filterExprLen,
    int from,
    int size) {

    auto startTime = std::chrono::steady_clock::now();

    // Determine which shards to query
    std::vector<int> targetShards = shardManager_->getShardsForQuery(query);

    // Filter to only local shards
    std::vector<int> localShards = shardManager_->getLocalShards();
    std::vector<int> shardsToQuery;

    for (int shardIndex : targetShards) {
        if (std::find(localShards.begin(), localShards.end(), shardIndex) != localShards.end()) {
            shardsToQuery.push_back(shardIndex);
        }
    }

    if (shardsToQuery.empty()) {
        // No local shards to query
        SearchResult emptyResult;
        emptyResult.totalHits = 0;
        emptyResult.maxScore = 0.0;
        emptyResult.took = 0;
        return emptyResult;
    }

    // Execute searches in parallel using futures
    // For distributed search, we need to fetch more results from each shard
    // to ensure we have enough for global pagination after merging by score
    int shardFrom = 0;
    int shardSize = (from + size) * shardsToQuery.size(); // Fetch extra to account for score distribution across shards

    std::vector<std::future<ShardSearchResult>> futures;
    futures.reserve(shardsToQuery.size());

    for (int shardIndex : shardsToQuery) {
        // Launch async search on each shard
        auto future = std::async(std::launch::async, [this, shardIndex, &query, filterExprBytes, filterExprLen, shardFrom, shardSize]() {
            return searchShard(shardIndex, query, filterExprBytes, filterExprLen, shardFrom, shardSize);
        });

        futures.push_back(std::move(future));
    }

    // Collect results
    std::vector<ShardSearchResult> shardResults;
    shardResults.reserve(futures.size());

    for (auto& future : futures) {
        try {
            shardResults.push_back(future.get());
        } catch (const std::exception& e) {
            // Log error but continue with other shards
            ShardSearchResult errorResult;
            errorResult.success = false;
            errorResult.error = e.what();
            shardResults.push_back(errorResult);
        }
    }

    // Merge results from all shards
    SearchResult mergedResult = mergeResults(shardResults, from, size);

    auto endTime = std::chrono::steady_clock::now();
    mergedResult.took = std::chrono::duration_cast<std::chrono::milliseconds>(endTime - startTime).count();

    return mergedResult;
}

ShardSearchResult DistributedSearchCoordinator::searchShard(
    int shardIndex,
    const std::string& query,
    const uint8_t* filterExprBytes,
    size_t filterExprLen,
    int from,
    int size) {

    auto startTime = std::chrono::steady_clock::now();

    ShardSearchResult shardResult;
    shardResult.shardIndex = shardIndex;
    shardResult.nodeId = shardManager_->getNodeId();

    try {
        // Get shard's document store
        auto store = shardManager_->getShardStore(shardIndex);
        if (!store) {
            shardResult.success = false;
            shardResult.error = "Shard not found";
            return shardResult;
        }

        // Create search integration for this shard
        SearchIntegration searchIntegration(store);

        // Execute search
        shardResult.result = searchIntegration.search(query, filterExprBytes, filterExprLen, from, size);
        shardResult.success = true;

    } catch (const std::exception& e) {
        shardResult.success = false;
        shardResult.error = e.what();
    }

    auto endTime = std::chrono::steady_clock::now();
    shardResult.latencyMs = std::chrono::duration_cast<std::chrono::milliseconds>(endTime - startTime).count();

    return shardResult;
}

SearchResult DistributedSearchCoordinator::mergeResults(
    const std::vector<ShardSearchResult>& shardResults,
    int from,
    int size) {

    SearchResult merged;
    merged.totalHits = 0;
    merged.maxScore = 0.0;
    merged.took = 0;

    // Collect all hits from all shards
    std::vector<std::pair<std::shared_ptr<Document>, int>> allHits; // (document, shardIndex)

    for (const auto& shardResult : shardResults) {
        if (!shardResult.success) {
            continue;
        }

        merged.totalHits += shardResult.result.totalHits;
        merged.maxScore = std::max(merged.maxScore, shardResult.result.maxScore);
        merged.took = std::max(merged.took, shardResult.latencyMs);

        for (const auto& hit : shardResult.result.hits) {
            allHits.push_back({hit, shardResult.shardIndex});
        }
    }

    // Sort all hits by score (descending)
    std::sort(allHits.begin(), allHits.end(),
        [](const auto& a, const auto& b) {
            return a.first->getScore() > b.first->getScore();
        });

    // Apply pagination: skip 'from' results, take 'size' results
    merged.hits.clear();
    merged.hits.reserve(size);

    int skipCount = 0;
    int takeCount = 0;

    for (const auto& hit : allHits) {
        if (skipCount < from) {
            skipCount++;
            continue;
        }

        if (takeCount >= size) {
            break;
        }

        merged.hits.push_back(hit.first);
        takeCount++;
    }

    // Merge aggregations
    merged.aggregations = mergeAggregations(shardResults);

    return merged;
}

std::unordered_map<std::string, AggregationResult> DistributedSearchCoordinator::mergeAggregations(
    const std::vector<ShardSearchResult>& shardResults) {

    std::unordered_map<std::string, AggregationResult> merged;

    // Group aggregations by name
    std::unordered_map<std::string, std::vector<AggregationResult>> aggGroups;

    for (const auto& shardResult : shardResults) {
        if (!shardResult.success) {
            continue;
        }

        for (const auto& aggPair : shardResult.result.aggregations) {
            aggGroups[aggPair.first].push_back(aggPair.second);
        }
    }

    // Merge each aggregation group
    for (const auto& aggGroup : aggGroups) {
        const std::string& aggName = aggGroup.first;
        const std::vector<AggregationResult>& aggResults = aggGroup.second;

        if (aggResults.empty()) {
            continue;
        }

        // Determine aggregation type from first result
        const std::string& aggType = aggResults[0].type;

        if (aggType == "terms") {
            merged[aggName] = mergeTermsAggregation(aggResults, 10);
        } else if (aggType == "stats") {
            merged[aggName] = mergeStatsAggregation(aggResults);
        }
    }

    return merged;
}

AggregationResult DistributedSearchCoordinator::mergeTermsAggregation(
    const std::vector<AggregationResult>& aggResults,
    int size) {

    AggregationResult merged;
    merged.name = aggResults[0].name;
    merged.type = "terms";

    // Collect all term buckets and sum their counts
    std::unordered_map<std::string, int64_t> termCounts;

    for (const auto& aggResult : aggResults) {
        for (const auto& bucket : aggResult.buckets) {
            std::string term = bucket.first;
            int64_t count = bucket.second;
            termCounts[term] += count;
        }
    }

    // Convert to vector and sort by count (descending)
    std::vector<std::pair<std::string, int64_t>> sortedTerms(termCounts.begin(), termCounts.end());

    std::sort(sortedTerms.begin(), sortedTerms.end(),
        [](const auto& a, const auto& b) {
            return a.second > b.second;
        });

    // Take top N terms
    merged.buckets.clear();
    for (size_t i = 0; i < std::min(static_cast<size_t>(size), sortedTerms.size()); i++) {
        merged.buckets.push_back(sortedTerms[i]);
    }

    return merged;
}

AggregationResult DistributedSearchCoordinator::mergeStatsAggregation(
    const std::vector<AggregationResult>& aggResults) {

    AggregationResult merged;
    merged.name = aggResults[0].name;
    merged.type = "stats";
    merged.count = 0;
    merged.sum = 0.0;
    merged.min = std::numeric_limits<double>::max();
    merged.max = std::numeric_limits<double>::lowest();

    for (const auto& aggResult : aggResults) {
        merged.count += aggResult.count;
        merged.sum += aggResult.sum;
        merged.min = std::min(merged.min, aggResult.min);
        merged.max = std::max(merged.max, aggResult.max);
    }

    // Calculate average across all shards
    if (merged.count > 0) {
        merged.avg = merged.sum / merged.count;
    } else {
        merged.avg = 0.0;
    }

    return merged;
}

} // namespace diagon
