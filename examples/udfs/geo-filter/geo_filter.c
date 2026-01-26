/**
 * Geo Filter UDF for Quidditch Search Engine
 *
 * Filters documents based on geographic distance using the Haversine formula.
 * Returns 1 (true) if the document's location is within max_distance_km of
 * the query coordinates, 0 (false) otherwise.
 *
 * Usage:
 * {
 *   "wasm_udf": {
 *     "name": "geo_filter",
 *     "version": "1.0.0",
 *     "parameters": {
 *       "lat_field": "latitude",
 *       "lon_field": "longitude",
 *       "target_lat": 37.7749,
 *       "target_lon": -122.4194,
 *       "max_distance_km": 10.0
 *     }
 *   }
 * }
 */

#include <math.h>

// Host function declarations
__attribute__((import_module("env"), import_name("has_field")))
int has_field(long long ctx_id, const char* field_ptr, int field_len);

__attribute__((import_module("env"), import_name("get_field_f64")))
int get_field_f64(long long ctx_id, const char* field_ptr, int field_len, double* out_ptr);

__attribute__((import_module("env"), import_name("get_param_f64")))
int get_param_f64(const char* name_ptr, int name_len, double* out_ptr);

__attribute__((import_module("env"), import_name("get_param_string")))
int get_param_string(const char* name_ptr, int name_len, char* value_ptr, int* value_len_ptr);

__attribute__((import_module("env"), import_name("log")))
void log_message(int level, const char* msg_ptr, int msg_len);

// Earth radius in kilometers
#define EARTH_RADIUS_KM 6371.0

// Buffer for string parameters
static char buffer[128];

/**
 * Convert degrees to radians
 */
static inline double deg_to_rad(double degrees) {
    return degrees * M_PI / 180.0;
}

/**
 * Calculate Haversine distance between two points
 * https://en.wikipedia.org/wiki/Haversine_formula
 *
 * Returns distance in kilometers
 */
static double haversine_distance(double lat1, double lon1, double lat2, double lon2) {
    // Convert to radians
    double lat1_rad = deg_to_rad(lat1);
    double lon1_rad = deg_to_rad(lon1);
    double lat2_rad = deg_to_rad(lat2);
    double lon2_rad = deg_to_rad(lon2);

    // Differences
    double dlat = lat2_rad - lat1_rad;
    double dlon = lon2_rad - lon1_rad;

    // Haversine formula
    double a = sin(dlat / 2.0) * sin(dlat / 2.0) +
               cos(lat1_rad) * cos(lat2_rad) *
               sin(dlon / 2.0) * sin(dlon / 2.0);

    double c = 2.0 * atan2(sqrt(a), sqrt(1.0 - a));

    // Distance in kilometers
    return EARTH_RADIUS_KM * c;
}

/**
 * Helper to get string parameter with default
 */
static const char* get_string_param_or_default(const char* name, const char* default_value) {
    int len = sizeof(buffer);
    if (get_param_string(name, __builtin_strlen(name), buffer, &len) == 0 && len > 0) {
        return buffer;
    }
    return default_value;
}

/**
 * Helper to get double parameter with default
 */
static double get_double_param_or_default(const char* name, double default_value) {
    double value;
    if (get_param_f64(name, __builtin_strlen(name), &value) == 0) {
        return value;
    }
    return default_value;
}

/**
 * Helper to get field value as double
 */
static int get_field_double(long long ctx_id, const char* field_name, double* out) {
    int name_len = 0;
    while (field_name[name_len] != '\0') name_len++;

    // Check if field exists
    if (has_field(ctx_id, field_name, name_len) == 0) {
        return -1;  // Field doesn't exist
    }

    // Get field value
    return get_field_f64(ctx_id, field_name, name_len, out);
}

/**
 * Main filter function
 *
 * Parameters:
 * - lat_field: Name of latitude field (default: "latitude")
 * - lon_field: Name of longitude field (default: "longitude")
 * - target_lat: Target latitude
 * - target_lon: Target longitude
 * - max_distance_km: Maximum distance in kilometers
 *
 * Returns:
 * - 1 if document is within max_distance_km of target
 * - 0 otherwise
 */
__attribute__((export_name("filter")))
int filter(long long ctx_id) {
    // Get parameters
    const char* lat_field = get_string_param_or_default("lat_field", "latitude");
    const char* lon_field = get_string_param_or_default("lon_field", "longitude");
    double target_lat = get_double_param_or_default("target_lat", 0.0);
    double target_lon = get_double_param_or_default("target_lon", 0.0);
    double max_distance = get_double_param_or_default("max_distance_km", 10.0);

    // Get document coordinates
    double doc_lat, doc_lon;

    if (get_field_double(ctx_id, lat_field, &doc_lat) != 0) {
        return 0;  // Latitude field missing or invalid
    }

    if (get_field_double(ctx_id, lon_field, &doc_lon) != 0) {
        return 0;  // Longitude field missing or invalid
    }

    // Validate coordinates
    if (doc_lat < -90.0 || doc_lat > 90.0 ||
        doc_lon < -180.0 || doc_lon > 180.0) {
        return 0;  // Invalid coordinates
    }

    // Calculate distance
    double distance = haversine_distance(doc_lat, doc_lon, target_lat, target_lon);

    // Return 1 if within range, 0 otherwise
    return (distance <= max_distance) ? 1 : 0;
}

// Export memory for host access
__attribute__((export_name("memory")))
unsigned char __heap_base;
