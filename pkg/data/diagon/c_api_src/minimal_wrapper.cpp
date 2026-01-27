/**
 * Minimal C API Wrapper for Diagon - STUB IMPLEMENTATION
 *
 * This is a temporary stub implementation to unblock Quidditch integration.
 * It provides basic in-memory search functionality using simple data structures.
 *
 * TODO Phase 6: Replace with real Diagon C++ engine when CMake build is fixed.
 */

#include "minimal_wrapper.h"
#include <map>
#include <vector>
#include <string>
#include <cstring>
#include <cstdlib>
#include <sstream>
#include <algorithm>

// Thread-local error storage
static thread_local std::string g_last_error;

namespace {

struct Document {
    std::string id;
    std::string json;
    std::map<std::string, std::string> fields;
};

struct Index {
    std::map<std::string, Document> documents;
    bool committed = false;
};

struct Searcher {
    Index* index;
};

// Helper: Parse simple JSON (very basic, just for stubs)
std::map<std::string, std::string> parse_json_fields(const std::string& json) {
    std::map<std::string, std::string> fields;
    // Simplified JSON parsing - just extract "field":"value" pairs
    size_t pos = 0;
    while (pos < json.length()) {
        size_t key_start = json.find('"', pos);
        if (key_start == std::string::npos) break;
        size_t key_end = json.find('"', key_start + 1);
        if (key_end == std::string::npos) break;

        std::string key = json.substr(key_start + 1, key_end - key_start - 1);

        size_t colon = json.find(':', key_end);
        if (colon == std::string::npos) break;

        size_t val_start = json.find('"', colon);
        if (val_start == std::string::npos) {
            // Number or boolean
            size_t val_end = json.find_first_of(",}", colon);
            if (val_end != std::string::npos) {
                std::string val = json.substr(colon + 1, val_end - colon - 1);
                // Trim whitespace
                val.erase(0, val.find_first_not_of(" \t\n\r"));
                val.erase(val.find_last_not_of(" \t\n\r") + 1);
                fields[key] = val;
                pos = val_end + 1;
            } else {
                break;
            }
        } else {
            size_t val_end = json.find('"', val_start + 1);
            if (val_end == std::string::npos) break;
            std::string val = json.substr(val_start + 1, val_end - val_start - 1);
            fields[key] = val;
            pos = val_end + 1;
        }
    }
    return fields;
}

} // anonymous namespace

extern "C" {

DiagonIndex diagon_create_index() {
    try {
        Index* idx = new Index();
        return static_cast<DiagonIndex>(idx);
    } catch (const std::exception& e) {
        g_last_error = std::string("Failed to create index: ") + e.what();
        return nullptr;
    }
}

bool diagon_add_document(DiagonIndex index, const char* doc_id, const char* doc_json) {
    if (!index || !doc_id || !doc_json) {
        g_last_error = "Invalid arguments to diagon_add_document";
        return false;
    }

    try {
        Index* idx = static_cast<Index*>(index);

        Document doc;
        doc.id = doc_id;
        doc.json = doc_json;
        doc.fields = parse_json_fields(doc_json);

        idx->documents[doc.id] = doc;
        idx->committed = false; // Mark as uncommitted

        return true;
    } catch (const std::exception& e) {
        g_last_error = std::string("Failed to add document: ") + e.what();
        return false;
    }
}

bool diagon_commit(DiagonIndex index) {
    if (!index) {
        g_last_error = "Invalid index";
        return false;
    }

    try {
        Index* idx = static_cast<Index*>(index);
        idx->committed = true;
        return true;
    } catch (const std::exception& e) {
        g_last_error = std::string("Failed to commit: ") + e.what();
        return false;
    }
}

DiagonSearcher diagon_create_searcher(DiagonIndex index) {
    if (!index) {
        g_last_error = "Invalid index";
        return nullptr;
    }

    try {
        Index* idx = static_cast<Index*>(index);
        if (!idx->committed) {
            g_last_error = "Index must be committed before searching";
            return nullptr;
        }

        Searcher* searcher = new Searcher();
        searcher->index = idx;
        return static_cast<DiagonSearcher>(searcher);
    } catch (const std::exception& e) {
        g_last_error = std::string("Failed to create searcher: ") + e.what();
        return nullptr;
    }
}

bool diagon_search(DiagonSearcher searcher, const char* query_json, int top_k, char** results_json) {
    if (!searcher || !query_json || !results_json) {
        g_last_error = "Invalid arguments to diagon_search";
        return false;
    }

    try {
        Searcher* s = static_cast<Searcher*>(searcher);
        Index* idx = s->index;

        // Simplified search: just return all documents (TODO: actual query parsing)
        std::ostringstream json_out;
        json_out << "{\"total_hits\":" << idx->documents.size()
                 << ",\"max_score\":1.0,\"hits\":[";

        int count = 0;
        for (const auto& kv : idx->documents) {
            if (count >= top_k) break;
            if (count > 0) json_out << ",";

            const Document& doc = kv.second;
            json_out << "{\"id\":\"" << doc.id
                     << "\",\"score\":1.0"
                     << ",\"source\":" << doc.json << "}";
            count++;
        }

        json_out << "]}";

        std::string result = json_out.str();
        *results_json = strdup(result.c_str());

        return true;
    } catch (const std::exception& e) {
        g_last_error = std::string("Failed to search: ") + e.what();
        return false;
    }
}

void diagon_close_index(DiagonIndex index) {
    if (index) {
        Index* idx = static_cast<Index*>(index);
        delete idx;
    }
}

void diagon_close_searcher(DiagonSearcher searcher) {
    if (searcher) {
        Searcher* s = static_cast<Searcher*>(searcher);
        delete s;
    }
}

void diagon_free_string(char* str) {
    if (str) {
        free(str);
    }
}

const char* diagon_last_error() {
    return g_last_error.c_str();
}

} // extern "C"
