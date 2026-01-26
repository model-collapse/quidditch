/**
 * Minimal C API Wrapper for Diagon Search Engine
 *
 * This is a simplified wrapper that doesn't use Diagon's full CMake build.
 * It provides just enough functionality for Quidditch's CGO integration.
 *
 * Full Diagon integration will be completed in Phase 6.
 */

#ifndef DIAGON_MINIMAL_WRAPPER_H
#define DIAGON_MINIMAL_WRAPPER_H

#ifdef __cplusplus
extern "C" {
#endif

#include <stdint.h>
#include <stdbool.h>

// Opaque handle types
typedef void* DiagonIndex;
typedef void* DiagonSearcher;

/**
 * Create a new in-memory Diagon index
 * Returns: DiagonIndex handle or NULL on error
 */
DiagonIndex diagon_create_index();

/**
 * Add a document to the index
 *
 * @param index The index handle
 * @param doc_id Document ID (string)
 * @param doc_json Document as JSON string
 * @return true on success, false on error
 */
bool diagon_add_document(DiagonIndex index, const char* doc_id, const char* doc_json);

/**
 * Commit changes to make them searchable
 *
 * @param index The index handle
 * @return true on success, false on error
 */
bool diagon_commit(DiagonIndex index);

/**
 * Create a searcher for the index
 *
 * @param index The index handle
 * @return DiagonSearcher handle or NULL on error
 */
DiagonSearcher diagon_create_searcher(DiagonIndex index);

/**
 * Execute a search query
 *
 * @param searcher The searcher handle
 * @param query_json Query as JSON string
 * @param top_k Number of top results to return
 * @param results_json Output: JSON array of search results (caller must free)
 * @return true on success, false on error
 */
bool diagon_search(DiagonSearcher searcher, const char* query_json, int top_k, char** results_json);

/**
 * Close the index and free resources
 */
void diagon_close_index(DiagonIndex index);

/**
 * Close the searcher
 */
void diagon_close_searcher(DiagonSearcher searcher);

/**
 * Free a string returned by diagon_search
 */
void diagon_free_string(char* str);

/**
 * Get last error message
 * @return Error message string (do not free)
 */
const char* diagon_last_error();

#ifdef __cplusplus
}
#endif

#endif // DIAGON_MINIMAL_WRAPPER_H
