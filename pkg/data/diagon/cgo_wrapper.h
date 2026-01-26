#ifndef CGO_WRAPPER_H
#define CGO_WRAPPER_H

#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

// Opaque types for C API
typedef struct diagon_shard_t diagon_shard_t;
typedef struct diagon_filter_t diagon_filter_t;

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
// Returns 0 on success, -1 on error
int diagon_index_document(
    diagon_shard_t* shard,
    const char* doc_id,
    const char* doc_json
);

// Returns JSON string (caller must free) or NULL if not found
char* diagon_get_document(
    diagon_shard_t* shard,
    const char* doc_id
);

// Returns 0 on success, -1 on error
int diagon_delete_document(
    diagon_shard_t* shard,
    const char* doc_id
);

// Index management
// Returns 0 on success, -1 on error
int diagon_refresh(diagon_shard_t* shard);
int diagon_flush(diagon_shard_t* shard);

// Get shard statistics
// Returns JSON string (caller must free) or NULL on error
char* diagon_get_stats(diagon_shard_t* shard);

// Distributed search types
typedef struct diagon_shard_manager_t diagon_shard_manager_t;
typedef struct diagon_distributed_coordinator_t diagon_distributed_coordinator_t;

// Create/destroy shard manager
diagon_shard_manager_t* diagon_create_shard_manager(
    const char* node_id,
    int total_shards
);
void diagon_destroy_shard_manager(diagon_shard_manager_t* manager);

// Register a shard with the manager
int diagon_register_shard(
    diagon_shard_manager_t* manager,
    int shard_index,
    diagon_shard_t* shard,
    int is_primary
);

// Get shard index for a document
int diagon_get_shard_for_document(
    diagon_shard_manager_t* manager,
    const char* doc_id
);

// Create/destroy distributed search coordinator
diagon_distributed_coordinator_t* diagon_create_coordinator(
    diagon_shard_manager_t* manager
);
void diagon_destroy_coordinator(diagon_distributed_coordinator_t* coordinator);

// Execute distributed search
// Returns JSON string with merged results (caller must free)
char* diagon_distributed_search(
    diagon_distributed_coordinator_t* coordinator,
    const char* query_json,
    const uint8_t* filter_expr,
    size_t filter_expr_len,
    int from,
    int size
);

#ifdef __cplusplus
}
#endif

#endif // CGO_WRAPPER_H
