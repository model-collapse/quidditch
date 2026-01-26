//! String Distance UDF for Quidditch Search Engine
//!
//! This UDF implements fuzzy string matching using Levenshtein distance.
//! It allows queries to match documents where a field value is "close" to
//! a target string, useful for typo-tolerant search.
//!
//! ## Usage
//!
//! ```json
//! {
//!   "wasm_udf": {
//!     "name": "string_distance",
//!     "version": "1.0.0",
//!     "parameters": {
//!       "field": "product_name",
//!       "target": "iPhone",
//!       "max_distance": 2
//!     }
//!   }
//! }
//! ```
//!
//! This will match documents where `product_name` differs from "iPhone"
//! by at most 2 character edits (insertions, deletions, or substitutions).

use core::slice;

// Host function imports
extern "C" {
    /// Check if a document has a specific field
    fn has_field(ctx_id: i64, field_ptr: *const u8, field_len: i32) -> i32;

    /// Get a string field value from the document
    /// Returns the length of the value written, or -1 on error
    fn get_field_string(
        ctx_id: i64,
        field_ptr: *const u8,
        field_len: i32,
        value_ptr: *mut u8,
        value_len_ptr: *mut i32,
    ) -> i32;

    /// Get an integer parameter from the query
    fn get_param_i64(name_ptr: *const u8, name_len: i32, out_ptr: *mut i64) -> i32;

    /// Get a string parameter from the query
    fn get_param_string(
        name_ptr: *const u8,
        name_len: i32,
        value_ptr: *mut u8,
        value_len_ptr: *mut i32,
    ) -> i32;

    /// Log a message (for debugging)
    fn log(level: i32, msg_ptr: *const u8, msg_len: i32);
}

// Memory buffer for string operations
static mut BUFFER: [u8; 1024] = [0; 1024];
static mut TARGET_BUFFER: [u8; 256] = [0; 256];

/// Calculate Levenshtein distance between two strings
fn levenshtein_distance(s1: &str, s2: &str) -> usize {
    let len1 = s1.chars().count();
    let len2 = s2.chars().count();

    if len1 == 0 {
        return len2;
    }
    if len2 == 0 {
        return len1;
    }

    // Use a 2-row approach to save memory
    let mut prev_row: Vec<usize> = (0..=len2).collect();
    let mut curr_row: Vec<usize> = vec![0; len2 + 1];

    for (i, c1) in s1.chars().enumerate() {
        curr_row[0] = i + 1;

        for (j, c2) in s2.chars().enumerate() {
            let cost = if c1 == c2 { 0 } else { 1 };

            curr_row[j + 1] = core::cmp::min(
                core::cmp::min(
                    curr_row[j] + 1,      // Insertion
                    prev_row[j + 1] + 1,  // Deletion
                ),
                prev_row[j] + cost,       // Substitution
            );
        }

        // Swap rows
        core::mem::swap(&mut prev_row, &mut curr_row);
    }

    prev_row[len2]
}

/// Helper to get a string parameter
unsafe fn get_string_param(name: &str, buffer: &mut [u8]) -> Option<&str> {
    let mut len = buffer.len() as i32;
    let result = get_param_string(
        name.as_ptr(),
        name.len() as i32,
        buffer.as_mut_ptr(),
        &mut len,
    );

    if result != 0 || len <= 0 {
        return None;
    }

    core::str::from_utf8(&buffer[..len as usize]).ok()
}

/// Helper to get an i64 parameter
unsafe fn get_i64_param(name: &str) -> Option<i64> {
    let mut value: i64 = 0;
    let result = get_param_i64(
        name.as_ptr(),
        name.len() as i32,
        &mut value,
    );

    if result == 0 {
        Some(value)
    } else {
        None
    }
}

/// Helper to get a field value as string
unsafe fn get_field(ctx_id: i64, field_name: &str, buffer: &mut [u8]) -> Option<&str> {
    // First check if field exists
    let has = has_field(ctx_id, field_name.as_ptr(), field_name.len() as i32);
    if has == 0 {
        return None;
    }

    // Get field value
    let mut len = buffer.len() as i32;
    let result = get_field_string(
        ctx_id,
        field_name.as_ptr(),
        field_name.len() as i32,
        buffer.as_mut_ptr(),
        &mut len,
    );

    if result != 0 || len <= 0 {
        return None;
    }

    core::str::from_utf8(&buffer[..len as usize]).ok()
}

/// Main filter function exported to WASM
///
/// Parameters (from query JSON):
/// - `field`: Name of the field to check (e.g., "product_name")
/// - `target`: Target string to compare against
/// - `max_distance`: Maximum Levenshtein distance to allow
///
/// Returns:
/// - 1 (i32) if the field value is within max_distance of target
/// - 0 (i32) otherwise
#[no_mangle]
pub extern "C" fn filter(ctx_id: i64) -> i32 {
    unsafe {
        // Get parameters
        let field_name = match get_string_param("field", &mut BUFFER[0..256]) {
            Some(s) => s,
            None => {
                // Default field name if not specified
                "name"
            }
        };

        let target = match get_string_param("target", &mut TARGET_BUFFER) {
            Some(s) => s,
            None => {
                // No target specified, can't match
                return 0;
            }
        };

        let max_distance = get_i64_param("max_distance").unwrap_or(2) as usize;

        // Get document field value
        let value = match get_field(ctx_id, field_name, &mut BUFFER[256..]) {
            Some(s) => s,
            None => {
                // Field doesn't exist or is not a string
                return 0;
            }
        };

        // Calculate distance
        let distance = levenshtein_distance(value, target);

        // Return 1 if within threshold, 0 otherwise
        if distance <= max_distance {
            1
        } else {
            0
        }
    }
}

// No-op panic handler for smaller binary
#[panic_handler]
fn panic(_: &core::panic::PanicInfo) -> ! {
    loop {}
}
