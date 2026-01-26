#pragma once

#include <string>
#include <vector>
#include <unordered_map>
#include <memory>
#include <mutex>
#include <functional>

namespace diagon {

// Forward declaration
class DocumentStore;

/**
 * ShardInfo - Information about a shard
 */
struct ShardInfo {
    std::string shardId;
    std::string nodeId;
    int shardIndex;
    int totalShards;
    bool isPrimary;
    std::vector<std::string> replicaNodes;
};

/**
 * NodeInfo - Information about a cluster node
 */
struct NodeInfo {
    std::string nodeId;
    std::string address;
    int port;
    bool isActive;
    int64_t lastHeartbeat;
    std::vector<std::string> shardIds;
};

/**
 * ShardManager - Manages shard distribution and routing
 *
 * Responsibilities:
 * - Shard assignment using consistent hashing
 * - Document routing to correct shard
 * - Cluster topology management
 * - Replica placement
 */
class ShardManager {
public:
    /**
     * Constructor
     * @param nodeId - ID of this node
     * @param totalShards - Total number of shards in cluster
     */
    ShardManager(const std::string& nodeId, int totalShards);
    ~ShardManager() = default;

    /**
     * Get shard index for a document ID
     * Uses consistent hashing to determine which shard should store a document
     *
     * @param docId - Document ID
     * @return Shard index (0 to totalShards-1)
     */
    int getShardForDocument(const std::string& docId) const;

    /**
     * Get shard index for a search query
     * For distributed search, this determines which shards need to be queried
     *
     * @param query - Query JSON string
     * @return Vector of shard indices to query (empty = query all shards)
     */
    std::vector<int> getShardsForQuery(const std::string& query) const;

    /**
     * Register a shard on this node
     *
     * @param shardIndex - Shard index
     * @param store - Document store for this shard
     * @param isPrimary - Whether this is the primary replica
     */
    void registerShard(int shardIndex, std::shared_ptr<DocumentStore> store, bool isPrimary = true);

    /**
     * Get document store for a shard
     *
     * @param shardIndex - Shard index
     * @return Document store or nullptr if not found
     */
    std::shared_ptr<DocumentStore> getShardStore(int shardIndex) const;

    /**
     * Get all local shards on this node
     *
     * @return Vector of shard indices
     */
    std::vector<int> getLocalShards() const;

    /**
     * Add a node to the cluster topology
     *
     * @param nodeInfo - Node information
     */
    void addNode(const NodeInfo& nodeInfo);

    /**
     * Remove a node from the cluster topology
     *
     * @param nodeId - Node ID to remove
     */
    void removeNode(const std::string& nodeId);

    /**
     * Get information about a node
     *
     * @param nodeId - Node ID
     * @return Node info or nullptr if not found
     */
    std::shared_ptr<NodeInfo> getNode(const std::string& nodeId) const;

    /**
     * Get all active nodes in the cluster
     *
     * @return Vector of node IDs
     */
    std::vector<std::string> getActiveNodes() const;

    /**
     * Get shard information
     *
     * @param shardIndex - Shard index
     * @return Shard info or nullptr if not found
     */
    std::shared_ptr<ShardInfo> getShardInfo(int shardIndex) const;

    /**
     * Get this node's ID
     */
    std::string getNodeId() const { return nodeId_; }

    /**
     * Get total number of shards
     */
    int getTotalShards() const { return totalShards_; }

private:
    /**
     * Hash function for consistent hashing
     * Uses MurmurHash3 for good distribution
     */
    uint32_t hash(const std::string& key) const;

    std::string nodeId_;
    int totalShards_;

    // Shard -> DocumentStore mapping for local shards
    mutable std::mutex shardsMutex_;
    std::unordered_map<int, std::shared_ptr<DocumentStore>> localShards_;
    std::unordered_map<int, std::shared_ptr<ShardInfo>> shardInfo_;

    // Cluster topology
    mutable std::mutex nodesMutex_;
    std::unordered_map<std::string, std::shared_ptr<NodeInfo>> nodes_;
};

} // namespace diagon
