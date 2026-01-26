;; Custom Score UDF - WebAssembly Text Format Example
;;
;; This is a simple scoring UDF that demonstrates:
;; 1. Accessing document fields
;; 2. Getting query parameters
;; 3. Performing calculations
;; 4. Returning a score-based filter decision
;;
;; Business Logic:
;; - Get document's base_score field
;; - Get document's boost field (optional, default 1.0)
;; - Calculate: final_score = base_score * boost
;; - Return 1 if final_score >= min_score parameter, 0 otherwise
;;
;; Usage:
;; {
;;   "wasm_udf": {
;;     "name": "custom_score",
;;     "version": "1.0.0",
;;     "parameters": {
;;       "min_score": 0.7
;;     }
;;   }
;; }

(module
  ;; Import host functions
  (import "env" "has_field"
    (func $has_field (param i64 i32 i32) (result i32)))

  (import "env" "get_field_f64"
    (func $get_field_f64 (param i64 i32 i32 i32) (result i32)))

  (import "env" "get_param_f64"
    (func $get_param_f64 (param i32 i32 i32) (result i32)))

  ;; Export memory
  (memory (export "memory") 1)

  ;; Field names stored in data section
  (data (i32.const 0) "base_score")     ;; 10 bytes at offset 0
  (data (i32.const 16) "boost")         ;; 5 bytes at offset 16
  (data (i32.const 32) "min_score")     ;; 9 bytes at offset 32

  ;; Global variables for temporary storage
  (global $base_score (mut f64) (f64.const 0.0))
  (global $boost (mut f64) (f64.const 1.0))
  (global $min_score (mut f64) (f64.const 0.5))
  (global $final_score (mut f64) (f64.const 0.0))

  ;; Main filter function
  ;; Takes document context ID (i64), returns 1 or 0 (i32)
  (func (export "filter") (param $ctx_id i64) (result i32)
    (local $result i32)
    (local $has_boost i32)

    ;; Get min_score parameter (default 0.5)
    (call $get_param_f64
      (i32.const 32)    ;; "min_score" pointer
      (i32.const 9)     ;; length
      (i32.const 64))   ;; output pointer

    ;; If parameter fetch succeeded, update global
    (if (i32.eq (local.get $result) (i32.const 0))
      (then
        (global.set $min_score (f64.load (i32.const 64)))
      )
    )

    ;; Get base_score field (required)
    (local.set $result
      (call $get_field_f64
        (local.get $ctx_id)
        (i32.const 0)     ;; "base_score" pointer
        (i32.const 10)    ;; length
        (i32.const 80)))  ;; output pointer

    ;; If base_score missing or error, return 0
    (if (i32.ne (local.get $result) (i32.const 0))
      (then
        (return (i32.const 0))
      )
    )

    ;; Load base_score into global
    (global.set $base_score (f64.load (i32.const 80)))

    ;; Check if boost field exists
    (local.set $has_boost
      (call $has_field
        (local.get $ctx_id)
        (i32.const 16)    ;; "boost" pointer
        (i32.const 5)))   ;; length

    ;; If boost field exists, get its value
    (if (i32.ne (local.get $has_boost) (i32.const 0))
      (then
        (local.set $result
          (call $get_field_f64
            (local.get $ctx_id)
            (i32.const 16)    ;; "boost" pointer
            (i32.const 5)     ;; length
            (i32.const 96)))  ;; output pointer

        ;; If successful, update boost global
        (if (i32.eq (local.get $result) (i32.const 0))
          (then
            (global.set $boost (f64.load (i32.const 96)))
          )
        )
      )
    )

    ;; Calculate final_score = base_score * boost
    (global.set $final_score
      (f64.mul
        (global.get $base_score)
        (global.get $boost)))

    ;; Return 1 if final_score >= min_score, else 0
    (if (result i32)
      (f64.ge
        (global.get $final_score)
        (global.get $min_score))
      (then (i32.const 1))
      (else (i32.const 0))
    )
  )
)
