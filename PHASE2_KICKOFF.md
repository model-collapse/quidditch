# Phase 2: Query Optimization & UDFs - Kickoff

**Date**: 2026-01-26
**Status**: 🚀 **PHASE 2 STARTED**
**Timeline**: Months 6-8 (3 months)

---

## 🎯 Phase 2 Goals

Transform Quidditch from a basic distributed search engine into an **intelligent query optimizer** with **programmable UDFs**.

**Key Objectives**:
1. Build Go-based query planner (no Java/Calcite dependency)
2. Complete WASM UDF runtime for programmability  
3. Enable query optimizations (10-30% speedup)
4. Support advanced scripting use cases
5. Maintain <200ms p99 for complex queries

---

## 📊 Current Status  

### What's Already Complete from Phase 2 ✅

1. **DSL Parser** (100%) ✅
   - Location: pkg/coordination/parser/
   - 13 query types supported
   - 1,591 lines of code
   - 15+ tests passing

2. **Expression Trees** (100%) ✅
   - Location: pkg/data/diagon/ (C++ side)
   - Native C++ evaluation (~5ns per call)
   - Filter pushdown working

3. **WASM UDF Runtime** (50%) ✅
   - Location: pkg/wasm/
   - Wasmtime integration complete
   - 50+ tests passing

---

## 🔨 What Needs to Be Built

### Priority 1: Query Planner (6 weeks)

Build custom Go-based query planner inspired by Apache Calcite.

**Components**:
1. Logical Plan Representation
2. Rule-Based Optimizer (10+ rules)
3. Cost Model
4. Physical Plan Generation

### Priority 2: Complete WASM Runtime (2 weeks)

1. Python UDF compilation
2. Advanced host functions
3. Resource limits
4. UDF deployment API

### Priority 3: Physical Plan Execution (4 weeks)

1. Execution engine
2. Task scheduler
3. Result streaming
4. Aggregation merge

---

## 📋 Week 1 Tasks

### Task 1: Design Query Planner API (2 days)
- Define LogicalPlan interface
- Design optimization rule interface
- Plan PhysicalPlan interface

### Task 2: Implement Basic Plan Nodes (3 days)
- LogicalScan
- LogicalFilter
- LogicalProject
- LogicalAggregate

### Task 3: Build AST Converter (2 days)
- Convert DSL AST → Logical Plan
- Handle all 13 query types

---

## 🚀 Quick Start

```bash
# Create planner package
mkdir -p pkg/coordination/planner/rules

# Run existing tests
go test ./pkg/coordination/parser/...
go test ./pkg/wasm/...
```

---

## 📈 Timeline

- Week 1-2: Query Planner Foundation
- Week 3-4: Optimization Rules  
- Week 5-6: Cost Model & Physical Plans
- Week 7-8: WASM Completion
- Week 9-12: Physical Execution

**Expected Completion**: End of Month 8

---

**Status**: ✅ READY TO START
**Next**: Design logical plan API

