#include "shard_manager.h"
#include "document_store.h"
#include <algorithm>
#include <chrono>

namespace diagon {

// MurmurHash3 32-bit implementation (simplified)
uint32_t ShardManager::hash(const std::string& key) const {
    const uint32_t seed = 0x9747b28c;
    const uint32_t m = 0x5bd1e995;
    const int r = 24;

    uint32_t h = seed ^ key.length();

    const unsigned char* data = (const unsigned char*)key.c_str();
    size_t len = key.length();

    while (len >= 4) {
        uint32_t k = *(uint32_t*)data;

        k *= m;
        k ^= k >> r;
        k *= m;

        h *= m;
        h ^= k;

        data += 4;
        len -= 4;
    }

    // Handle remaining bytes
    switch (len) {
        case 3: h ^= data[2] << 16;
        case 2: h ^= data[1] << 8;
        case 1: h ^= data[0];
                h *= m;
    }

    h ^= h >> 13;
    h *= m;
    h ^= h >> 15;

    return h;
}

ShardManager::ShardManager(const std::string& nodeId, int totalShards)
    : nodeId_(nodeId), totalShards_(totalShards) {

    if (totalShards_ <= 0) {
        throw std::invalid_argument("totalShards must be positive");
    }
}

int ShardManager::getShardForDocument(const std::string& docId) const {
    // Use consistent hashing to determine shard
    uint32_t hashValue = hash(docId);
    return hashValue % totalShards_;
}

std::vector<int> ShardManager::getShardsForQuery(const std::string& query) const {
    // For now, return all shards (broadcast query)
    // In the future, this could be optimized based on query type
    // For example, if query contains a specific docId, only query that shard

    std::vector<int> shards;
    shards.reserve(totalShards_);

    for (int i = 0; i < totalShards_; i++) {
        shards.push_back(i);
    }

    return shards;
}

void ShardManager::registerShard(int shardIndex, std::shared_ptr<DocumentStore> store, bool isPrimary) {
    if (shardIndex < 0 || shardIndex >= totalShards_) {
        throw std::invalid_argument("Invalid shard index");
    }

    std::lock_guard<std::mutex> lock(shardsMutex_);

    localShards_[shardIndex] = store;

    // Create shard info
    auto info = std::make_shared<ShardInfo>();
    info->shardId = nodeId_ + "_shard_" + std::to_string(shardIndex);
    info->nodeId = nodeId_;
    info->shardIndex = shardIndex;
    info->totalShards = totalShards_;
    info->isPrimary = isPrimary;

    shardInfo_[shardIndex] = info;
}

std::shared_ptr<DocumentStore> ShardManager::getShardStore(int shardIndex) const {
    std::lock_guard<std::mutex> lock(shardsMutex_);

    auto it = localShards_.find(shardIndex);
    if (it != localShards_.end()) {
        return it->second;
    }

    return nullptr;
}

std::vector<int> ShardManager::getLocalShards() const {
    std::lock_guard<std::mutex> lock(shardsMutex_);

    std::vector<int> shards;
    shards.reserve(localShards_.size());

    for (const auto& pair : localShards_) {
        shards.push_back(pair.first);
    }

    std::sort(shards.begin(), shards.end());

    return shards;
}

void ShardManager::addNode(const NodeInfo& nodeInfo) {
    std::lock_guard<std::mutex> lock(nodesMutex_);

    auto node = std::make_shared<NodeInfo>(nodeInfo);
    nodes_[nodeInfo.nodeId] = node;
}

void ShardManager::removeNode(const std::string& nodeId) {
    std::lock_guard<std::mutex> lock(nodesMutex_);
    nodes_.erase(nodeId);
}

std::shared_ptr<NodeInfo> ShardManager::getNode(const std::string& nodeId) const {
    std::lock_guard<std::mutex> lock(nodesMutex_);

    auto it = nodes_.find(nodeId);
    if (it != nodes_.end()) {
        return it->second;
    }

    return nullptr;
}

std::vector<std::string> ShardManager::getActiveNodes() const {
    std::lock_guard<std::mutex> lock(nodesMutex_);

    std::vector<std::string> activeNodes;
    activeNodes.reserve(nodes_.size());

    auto now = std::chrono::system_clock::now().time_since_epoch().count();
    const int64_t heartbeatTimeout = 30000; // 30 seconds

    for (const auto& pair : nodes_) {
        if (pair.second->isActive &&
            (now - pair.second->lastHeartbeat) < heartbeatTimeout) {
            activeNodes.push_back(pair.first);
        }
    }

    return activeNodes;
}

std::shared_ptr<ShardInfo> ShardManager::getShardInfo(int shardIndex) const {
    std::lock_guard<std::mutex> lock(shardsMutex_);

    auto it = shardInfo_.find(shardIndex);
    if (it != shardInfo_.end()) {
        return it->second;
    }

    return nullptr;
}

} // namespace diagon
