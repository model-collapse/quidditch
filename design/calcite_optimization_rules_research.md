# Apache Calcite Optimization Rules Research for PPL Query Planning

**Research Date:** January 27, 2026
**Repository:** https://github.com/apache/calcite
**Primary Focus:** Optimization rules applicable to PPL (Piped Processing Language) query planning

---

## Executive Summary

Apache Calcite provides a comprehensive rule-based query optimizer with 100+ optimization rules across multiple categories. The optimizer uses two primary planners:
- **VolcanoPlanner**: Cost-based dynamic programming for exhaustive optimization
- **HepPlanner**: Heuristic-based optimization for faster compilation

This research identifies key optimization rules relevant to PPL queries, particularly for log analytics workloads involving filtering, aggregation, joins, and time-series patterns.

---

## Table of Contents

1. [Core Optimization Architecture](#core-optimization-architecture)
2. [Categorized Rule Catalog (100+ Rules)](#categorized-rule-catalog)
3. [Rule-Based Optimizer Details](#rule-based-optimizer-details)
4. [Cost Model and Metadata System](#cost-model-and-metadata-system)
5. [PPL-Specific Insights](#ppl-specific-insights)
6. [Implementation Patterns](#implementation-patterns)
7. [Best Practices for Rule Ordering](#best-practices-for-rule-ordering)
8. [Transformation Examples](#transformation-examples)

---

## Core Optimization Architecture

### Relational Algebra Foundation

Calcite represents queries as trees of relational operators (RelNode). The optimizer transforms these trees through:

1. **Pattern Matching**: Rules define operand patterns to match against expression trees
2. **Transformation**: Matched patterns are rewritten into equivalent but more efficient forms
3. **Cost Evaluation**: Alternative plans are compared using a cost model
4. **Trait Propagation**: Properties like collation, distribution, and convention guide optimization

### Query Optimization Process

```
SQL/PPL Query → Parse → Validate → SQL-to-Rel Conversion →
  Rule Application (Multiple Phases) → Cost-Based Selection →
  Physical Plan Generation → Execution
```

**Key Phases:**
1. **Initial conversion**: SQL/PPL to logical relational algebra
2. **Logical optimization**: Rule-based transformations (filter pushdown, projection merge, etc.)
3. **Subquery removal**: Decorrelation and join conversion
4. **Physical planning**: Convention conversion and trait enforcement
5. **Final optimization**: Cost-based selection of optimal plan

---

## Categorized Rule Catalog

### 1. Filter Rules (Predicate Optimization)

#### Core Filter Rules
| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **FilterMergeRule** | Combines consecutive filters into a single filter with AND condition | High - PPL chains filters | `\| where x > 5 \| where y < 10` → single filter `x > 5 AND y < 10` |
| **FilterProjectTransposeRule** | Pushes filter below projection to apply earlier | High - Earlier filtering reduces data | `\| fields a, b \| where a > 5` → `\| where a > 5 \| fields a, b` |
| **FilterAggregateTransposeRule** | Pushes filters above aggregations when possible | High - Pre-aggregate filtering | Filter on grouped column → push before GROUP BY |
| **FilterTableScanRule** | Pushes filters into table scans | Critical - Reduces I/O | Converts filter on scan to filtered scan |
| **FilterRemoveIsNotDistinctFromRule** | Simplifies IS NOT DISTINCT FROM predicates | Medium | Optimizes null-safe comparisons |
| **FilterToCalcRule** | Converts Filter to Calc operator | Medium | Enables Calc fusion optimizations |
| **FilterCalcMergeRule** | Merges Filter with Calc operations | Medium | Reduces operator count |
| **FilterCorrelateRule** | Optimizes filters on correlated operations | Low - Complex subqueries | Handles correlated subquery filters |
| **FilterSortTransposeRule** | Moves filters past sorts when safe | Medium | Reduces data before sorting |

#### Join-Related Filter Rules
| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **FilterJoinRule** (Abstract) | Base for pushing filters into/through joins | Critical - Join optimization | - |
| **FilterIntoJoinRule** | Pushes filters from WHERE into JOIN conditions | High | `\| join ... \| where t1.x = 5` → join with filter |
| **JoinConditionPushRule** | Pushes join conditions into join inputs | High | Reduces join input sizes |
| **JoinDeriveIsNotNullFilterRule** | Infers NOT NULL constraints from join conditions | Medium | Enables additional optimizations |

**Key Configuration:**
- `bloat`: Controls complexity threshold for pushing filters (default: 100)
- `copyFilter`/`copyProject`: Whether to use builder or copy operators
- `smart`: Enables intelligent outer join simplification

**Implementation Details:**
- Uses `RelOptUtil.classifyFilters()` to categorize predicates
- Preserves join semantics (INNER vs OUTER)
- Checks for determinism and correlation variables
- Applies predicate inference for transitive equalities

---

### 2. Projection Rules (Column Selection)

| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **ProjectMergeRule** | Combines consecutive projections | High - PPL chains fields commands | `\| fields a, b \| fields a` → single projection |
| **ProjectRemoveRule** | Eliminates identity projections | High - Removes redundant ops | Projection of all columns unchanged → removed |
| **ProjectFilterTransposeRule** | Reorders projections and filters | High - Optimization flexibility | Enables better filter pushdown |
| **ProjectJoinTransposeRule** | Pushes projections past joins | High - Reduces join width | Projects only needed columns before join |
| **ProjectAggregateMergeRule** | Merges projections with aggregates | Medium | Removes unused aggregate calls |
| **ProjectJoinRemoveRule** | Eliminates unnecessary join projections | Medium | Simplifies join output |
| **ProjectTableScanRule** | Pushes projections into table scans | Critical - Column pruning | Only reads required columns |
| **ProjectToCalcRule** | Converts Project to Calc operator | Medium | Enables Calc fusion |
| **ProjectCalcMergeRule** | Merges Project with Calc operations | Medium | Reduces operator count |

**Key Configuration:**
- `bloat`: Limits complexity increase during merging (default: 100)
- `force`: Always merge vs. skip if already identical (default: true)

**Implementation Details:**
- Detects permutation patterns and computes their product
- Validates against bloat thresholds using `pushPastProjectUnlessBloat()`
- Eliminates identity operations when merged result matches input
- Preserves correlation variables to maintain query semantics

---

### 3. Aggregate Rules (GROUP BY and Aggregation Functions)

#### Core Aggregate Rules
| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **AggregateMergeRule** | Combines consecutive aggregates | High - Nested stats commands | Two GROUP BYs → single if compatible |
| **AggregateRemoveRule** | Eliminates redundant aggregates | High - Unnecessary grouping | GROUP BY on unique key → removed |
| **AggregateReduceFunctionsRule** | Decomposes complex aggregate functions | Critical - Function simplification | `AVG(x)` → `SUM(x)/COUNT(x)` |
| **AggregateProjectMergeRule** | Merges projections into aggregates | High - Combined operations | Removes unused aggregate results |
| **AggregateProjectPullUpConstantsRule** | Removes constant grouping keys | High - Reduces cardinality | GROUP BY x, 5 → GROUP BY x with const projection |
| **AggregateFilterTransposeRule** | Moves filters above aggregates | High - Post-aggregation filtering | Filter on aggregate result → HAVING equivalent |
| **AggregateExpandDistinctAggregatesRule** | Expands DISTINCT aggregates | Medium | `COUNT(DISTINCT x)` → subquery pattern |
| **AggregateExpandWithinDistinctRule** | Handles WITHIN DISTINCT clauses | Low | Advanced DISTINCT handling |
| **AggregateCaseToFilterRule** | Converts CASE in aggregates to filters | Medium | `SUM(CASE WHEN x>5 THEN 1 ELSE 0)` → filtered sum |
| **AggregateFilterToCaseRule** | Transforms aggregate filters to CASE | Medium | Inverse of above |
| **AggregateMinMaxToLimitRule** | Optimizes MIN/MAX using LIMIT | Medium | `SELECT MAX(x)` → `ORDER BY x DESC LIMIT 1` |

#### Aggregate Transpose Rules
| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **AggregateJoinTransposeRule** | Pushes aggregates below joins | High - Pre-join aggregation | Aggregate before join reduces cardinality |
| **AggregateUnionTransposeRule** | Pushes aggregates past unions | Medium | Aggregate each union branch |
| **AggregateJoinRemoveRule** | Eliminates unnecessary joins in aggregates | Medium | Removes joins on unique keys |
| **AggregateExtractProjectRule** | Extracts projections from aggregates | Medium | Separates projection logic |

**Function Decompositions (AggregateReduceFunctionsRule):**
- `AVG(x)` → `SUM(x) / COUNT(x)`
- `STDDEV_POP(x)` → `SQRT((SUM(x²) - SUM(x)² / COUNT(x)) / COUNT(x))`
- `VAR_POP(x)` → `(SUM(x²) - SUM(x)² / COUNT(x)) / COUNT(x)`
- `COVAR_POP(x,y)` → Variance-based decomposition
- `REGR_SXX(x,y)`, `REGR_SYY(x,y)` → Regression decompositions

**Key Constraints:**
- Merge requires subset group keys and splittable functions
- No DISTINCT, FILTER clauses, or multi-argument functions
- Blocks COUNT without SUM0 when top has empty groups
- Transpose requires INNER joins for safety

---

### 4. Join Rules (Multi-Table Operations)

#### Join Reordering Rules
| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **JoinCommuteRule** | Swaps join operands (left ↔ right) | High - Join order exploration | `A JOIN B` → `B JOIN A` |
| **JoinAssociateRule** | Reorders nested joins through associativity | High - Join tree restructuring | `(A JOIN B) JOIN C` → `A JOIN (B JOIN C)` |
| **JoinPushThroughJoinRule** | Reorders multi-way joins | High - Complex join optimization | Enables better predicate application |
| **JoinToMultiJoinRule** | Flattens join trees for N-way optimization | High - Multi-table queries | Binary joins → MultiJoin structure |

#### Join Transformation Rules
| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **JoinExtractFilterRule** | Separates join conditions from filters | High - Cleaner join logic | Splits ON and WHERE clauses |
| **JoinProjectTransposeRule** | Moves projections across joins | High - Reduces join width | Projects only needed columns |
| **JoinUnionTransposeRule** | Reorders unions relative to joins | Medium | Distributes join over union |
| **JoinToCorrelateRule** | Converts joins to correlate operations | Low - Subquery handling | Enables decorrelation |

#### Semi-Join and Special Rules
| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **SemiJoinRule** (Multiple variants) | Creates semi-joins from joins + aggregates | High - Cardinality reduction | JOIN with DISTINCT → semi-join |
| **JoinOnUniqueToSemiJoinRule** | Exploits unique key constraints | Medium | Optimizes joins on primary keys |
| **IntersectToSemiJoinRule** | Converts INTERSECT to semi-join | Low | Set operation optimization |
| **MinusToAntiJoinRule** | Converts MINUS/EXCEPT to anti-join | Low | Set operation optimization |

**Key Transformations (JoinPushThroughJoinRule):**
```
(sales JOIN product_class) JOIN product
  ON s.product_id = p.product_id AND p.class_id = pc.class_id
→
(sales JOIN product ON s.product_id = p.product_id)
  JOIN product_class ON p.class_id = pc.class_id
```

**Configuration:**
- `swapOuter`: Whether to swap outer joins (default: depends on variant)
- `isRight`: Push direction (right vs left variant)
- `allowAlwaysTrueCondition`: Permits cartesian products

**Safety Constraints:**
- JoinAssociateRule: Only INNER joins, matching conventions
- JoinPushThroughJoinRule: Analyzes condition dependencies
- Preserves outer join semantics (LEFT → RIGHT when swapped)
- Field reference remapping ensures correctness

---

### 5. Sort and Limit Rules

| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **SortRemoveRule** | Eliminates redundant sorts | High - Removes unnecessary ordering | Sort on already sorted data → removed |
| **SortRemoveConstantKeysRule** | Removes constant sort keys | High - Simplifies ordering | ORDER BY x, 5 → ORDER BY x |
| **SortMergeRule** | Combines consecutive sorts | Medium | Two sorts → single sort |
| **SortProjectTransposeRule** | Reorders sorts and projections | Medium | Optimization flexibility |
| **SortJoinTransposeRule** | Moves sorts relative to joins | Low | Push sort before/after join |
| **SortUnionTransposeRule** | Reorders sorts and unions | Medium | Sort union branches |

**Key Implementation (SortRemoveRule):**
- Leverages collation trait system to detect ordering
- Preserves OFFSET/LIMIT operations (cannot remove if present)
- Replaces sort with input when redundant
- Propagates collation traits through plan

---

### 6. Set Operation Rules (UNION, INTERSECT, MINUS)

| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **UnionMergeRule** | Combines consecutive unions | Medium - Multi-source queries | `UNION(UNION(a,b), UNION(c,d))` → `UNION(a,b,c,d)` |
| **UnionRemoveRule** | Eliminates single-input unions | High - Simplification | `UNION(a)` → `a` |
| **IntersectMergeRule** | Flattens nested intersects | Low | Combines intersect operations |
| **MinusMergeRule** | Flattens nested minus (carefully) | Low | Only left-side flattening |
| **IntersectToDistinctRule** | Converts INTERSECT to DISTINCT | Low | Alternative implementation |
| **MinusToDistinctRule** | Converts MINUS to DISTINCT | Low | Alternative implementation |
| **UnionToDistinctRule** | Converts UNION to DISTINCT | Low | Set operation optimization |
| **AggregateGroupingSetsToUnionRule** | Converts GROUPING SETS to unions | Medium | Handles complex grouping |

**Key Constraints:**
- Only merges operations with matching distinctness (ALL vs DISTINCT)
- Preserves operation semantics (MINUS cannot be fully flattened)
- Reduces tree depth for simpler execution plans

---

### 7. Calc Rules (Fused Filter + Project)

| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **CalcMergeRule** | Fuses consecutive Calc operations | High - Reduces passes | Two Calc ops → single Calc |
| **CalcRemoveRule** | Eliminates trivial Calc operations | High - Dead code elimination | Identity Calc → removed |
| **FilterToCalcRule** | Converts Filter to Calc | Medium | Enables fusion |
| **ProjectToCalcRule** | Converts Project to Calc | Medium | Enables fusion |
| **FilterCalcMergeRule** | Merges Filter into Calc | Medium | Combines operations |
| **ProjectCalcMergeRule** | Merges Project into Calc | Medium | Combines operations |

**What is Calc?**
Calc is a fused operator that combines filtering and projection into a single operation using a `RexProgram`. Benefits:
- Single data pass instead of multiple
- Reduced intermediate materialization
- Simplified execution plans
- Better memory efficiency

**Implementation:**
- Uses `RexProgramBuilder.mergePrograms()` to combine logic
- Avoids merging when windowed aggregates present
- Rewires input references through combined program

---

### 8. Expression Simplification Rules

| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **ReduceExpressionsRule** (Multiple variants) | Evaluates constants at compile time | Critical - Constant folding | `1 + 2` → `3`, removes casts |
| **FilterReduceExpressionsRule** | Simplifies filter expressions | High | `WHERE TRUE` → removed, `WHERE FALSE` → empty |
| **ProjectReduceExpressionsRule** | Simplifies projection expressions | High | Constant expressions computed |
| **JoinReduceExpressionsRule** | Simplifies join conditions | High | Optimizes predicates |
| **CalcReduceExpressionsRule** | Optimizes Calc programs | High | Reduces nested expressions |
| **WindowReduceExpressionsRule** | Eliminates constant window keys | Medium | Simplifies window specs |
| **ReduceDecimalsRule** | Simplifies decimal expressions | Medium | Optimizes numeric operations |

**RexSimplify Capabilities:**
- Boolean logic optimization: `x = 1 OR NOT x = 1 OR x IS NULL` → `TRUE`
- Null handling: Multiple strategies via `RexUnknownAs` enum
- Nullability cast removal: `CAST(1 = 0 AS BOOLEAN)` → `1 = 0`
- Predicate combination: Combines multiple predicates with AND
- Type preservation: Maintains original type and nullability

---

### 9. Subquery and Correlation Rules

| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **SubQueryRemoveRule** | Decorrelates subqueries to joins | High - Subquery optimization | `WHERE x IN (SELECT...)` → join |
| **JoinToCorrelateRule** | Converts joins to correlate | Low | Enables decorrelation patterns |
| **FilterCorrelateRule** | Optimizes correlated filters | Low | Complex subquery handling |

**SubQueryRemoveRule Patterns:**
- **Scalar subqueries**: → LEFT JOIN + aggregate with SINGLE_VALUE
- **EXISTS**: → INNER/LEFT JOIN with constant projection
- **IN predicates**: → LEFT JOIN with CASE for three-valued logic
- **SOME/Quantified**: → Aggregate min/max with CASE expressions
- **Collections** (ARRAY, MAP, MULTISET): → Collect operators with INNER JOIN

**Decorrelation Strategy:**
- Wrapped RelNode contains `RexCorrelVariable` references
- Transformation produces `Correlate` node
- `RelDecorrelator` eliminates correlation dependencies
- Manages field offsets and join conditions carefully

---

### 10. Empty Relation Pruning Rules

| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **PruneEmptyRules** (Collection) | Eliminates dead code producing no rows | High - Early termination | Query on empty table → empty result |
| **UnionEmptyPruneRule** | Removes empty union inputs | Medium | `UNION(Rel, Empty)` → `Rel` |
| **JoinEmptyPruneRule** | Handles empty join inputs | Medium | INNER JOIN with empty side → empty |
| **ProjectEmptyPruneRule** | Propagates emptiness through projects | Medium | `Project(Empty)` → Empty |
| **FilterEmptyPruneRule** | Propagates emptiness through filters | Medium | `Filter(Empty)` → Empty |
| **AggregateEmptyPruneRule** | Handles empty aggregates | Medium | May produce single row with NULLs |
| **SortEmptyPruneRule** | Propagates emptiness through sorts | Medium | `Sort(Empty)` → Empty |

**Strategy:**
- Leverages metadata queries to detect empty relations
- Empty represented as `Values` with no tuples
- Different handling for inner vs outer joins
- Aggregates may produce rows even with empty input

---

### 11. Window Function Rules

| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **ProjectToWindowRule** | Converts projects with window functions to Window operator | Low - Advanced analytics | Windowed calculations |
| **WindowReduceExpressionsRule** | Eliminates constant window keys | Low | Simplifies window specs |

**Note:** PPL currently has limited window function support. These rules become relevant as window capabilities expand.

---

### 12. Table Scan and Physical Rules

| Rule Name | Description | PPL Relevance | Example |
|-----------|-------------|---------------|---------|
| **FilterTableScanRule** | Pushes filters into table scans | Critical - I/O reduction | Filtered scan vs full scan + filter |
| **ProjectTableScanRule** | Pushes projections into table scans | Critical - Column pruning | Only read needed columns |
| **AggregateStarTableRule** | Matches aggregates on star tables | Low - Schema-specific | Star schema optimization |
| **TableScanRule** | Generic table scan conversion | Medium | Logical → physical conversion |

---

## Rule-Based Optimizer Details

### VolcanoPlanner (Cost-Based Optimization)

**Architecture:**
- Dynamic programming strategy
- Maintains equivalence classes (RelSet/RelSubset)
- Explores plan space exhaustively within bounds
- Selects optimal plan using cost model

**Rule Application Strategy:**
1. **Transformation Rules**: Generate new relational expressions
2. **Substitution Rules**: Replace with equivalent alternatives
3. **Top-Down Optimization**: Optional mode via `setTopDownOpt()`
4. **Cost Bounds**: Uses `getCost()`, `getLowerBound()`, `upperBoundForInputs()`

**Key Methods:**
- `register()`: Adds expressions to equivalence sets
- `findBestExp()`: Discovers optimal implementation
- `ensureRegistered()`: Conditionally registers expressions

**When to Use:**
- Complex queries requiring optimal plans
- Accurate cardinality estimates available
- Execution time matters more than compilation time
- Multiple join orderings need evaluation

---

### HepPlanner (Heuristic Optimization)

**Architecture:**
- Greedy sequential rule application
- Directed acyclic graph (DAG) representation
- No exhaustive search, no cost comparison
- Mark-and-sweep garbage collection

**Match Ordering Options:**
- ARBITRARY
- DEPTH_FIRST
- TOP_DOWN
- BOTTOM_UP

**Rule Application:**
- Sequential processing based on HepProgram
- Early termination at fixed point or match limits
- Simple and fast compilation

**When to Use:**
- Moderate query complexity
- Fast "good enough" solutions needed
- Memory constraints limit exhaustive search
- Domain expertise for rule ordering available

---

## Cost Model and Metadata System

### RelOptCost Interface

**Three Primary Metrics:**
1. **Row Count** (`getRows()`): Number of rows processed
2. **CPU Usage** (`getCpu()`): Computational resource consumption
3. **I/O Usage** (`getIo()`): Data access overhead

**Comparison Methods:**
- `equals()`: Exact match
- `isEqWithEpsilon()`: Match with rounding tolerance
- `isLe()`: Less than or equal
- `isLt()`: Strictly less than
- `isInfinite()`: Unimplementable expression

**Arithmetic Operations:**
- `plus()`, `minus()`, `multiplyBy()`, `divideBy()`

**Design Philosophy:**
- Units are intentionally vague for flexibility
- Implementations can define custom semantics
- Can be extended with additional metrics (memory, network)

---

### RelMetadataQuery System

**Cardinality Estimation:**
- `getRowCount()`: Estimated row count
- `getMaxRowCount()` / `getMinRowCount()`: Boundary estimates
- `getDistinctRowCount()`: Distinct rows for grouped columns

**Selectivity Estimates:**
- `getSelectivity()`: Predicate selectivity (0.0 to 1.0)
- Helps determine filter effectiveness

**Cost Support:**
- `getCumulativeCost()` / `getNonCumulativeCost()`: Operator costs
- `getPercentageOriginalRows()`: Data reduction tracking
- `getColumnOrigins()` / `getTableOrigin()`: Data lineage

**Purpose:**
Enables cost-based decisions by providing statistics and estimates for comparing execution strategies.

---

## PPL-Specific Insights

### Most Relevant Rules for Log Analytics

#### Tier 1: Critical (Must Implement)
1. **FilterMergeRule** - PPL chains filters frequently
2. **FilterProjectTransposeRule** - Early filtering reduces pipeline data
3. **ProjectMergeRule** - PPL chains fields/eval commands
4. **ProjectRemoveRule** - Eliminates redundant field selections
5. **FilterTableScanRule** - Critical for I/O reduction in log scans
6. **ProjectTableScanRule** - Column pruning for wide log schemas
7. **AggregatePullUpConstantsRule** - Common in time-bucketed aggregations
8. **AggregateReduceFunctionsRule** - Simplifies stats commands
9. **ReduceExpressionsRule** (all variants) - Constant folding and simplification
10. **FilterReduceExpressionsRule** - Simplifies filter predicates

#### Tier 2: High Value (Strong Impact)
11. **FilterIntoJoinRule** - Optimizes join + where patterns
12. **JoinCommuteRule** - Enables join order exploration
13. **JoinPushThroughJoinRule** - Multi-table join optimization
14. **AggregateJoinTransposeRule** - Pre-join aggregation for large datasets
15. **AggregateMergeRule** - Nested stats commands
16. **AggregateProjectMergeRule** - Combines stats with field operations
17. **ProjectJoinTransposeRule** - Reduces join width
18. **CalcMergeRule** - Fuses filter + project for efficiency
19. **SortRemoveRule** - Eliminates redundant sorts
20. **UnionMergeRule** - Multi-source log queries
21. **SubQueryRemoveRule** - Decorrelates subqueries
22. **SemiJoinRule** - Cardinality reduction for joins
23. **PruneEmptyRules** - Early termination detection

#### Tier 3: Medium Value (Situational)
24. **FilterAggregateTransposeRule** - Post-aggregation filtering
25. **JoinAssociateRule** - Join tree restructuring
26. **JoinExtractFilterRule** - Cleaner join semantics
27. **SortRemoveConstantKeysRule** - Sort optimization
28. **AggregateExpandDistinctAggregatesRule** - DISTINCT handling
29. **ProjectFilterTransposeRule** - Reordering flexibility
30. **JoinToMultiJoinRule** - N-way join planning

---

### Rules for Time-Series Queries

**Temporal Patterns:**
1. **Window functions** (limited in current PPL):
   - TUMBLE, HOP, SLIDING windows (from streaming SQL)
   - Time-based aggregation bucketing

2. **Time-range filters:**
   - FilterMergeRule: Combines time range with other filters
   - FilterTableScanRule: Pushes time filters to storage layer

3. **Time-bucketed aggregations:**
   - AggregatePullUpConstantsRule: Removes constant time expressions
   - AggregateReduceFunctionsRule: Simplifies temporal statistics

**Streaming Capabilities (Future):**
- Monotonicity constraints for progress tracking
- Watermarks for out-of-order tolerance
- Tumbling, hopping, sliding window aggregations
- Stream-to-table joins for enrichment

---

### Rules for Aggregation-Heavy Workloads

**Log Analytics Patterns:**
1. **Multiple aggregation levels:**
   - AggregateMergeRule: Combines compatible groupings
   - AggregateJoinTransposeRule: Pre-join aggregation

2. **Distinct counts:**
   - AggregateExpandDistinctAggregatesRule: Handles COUNT(DISTINCT)
   - SemiJoinRule: Alternative for distinct joins

3. **Statistical functions:**
   - AggregateReduceFunctionsRule: Decomposes AVG, STDDEV, VAR

4. **Post-aggregation filtering:**
   - FilterAggregateTransposeRule: Optimizes HAVING equivalent

---

### Rules for Join Optimization

**Multi-Source Log Correlation:**
1. **Join order exploration:**
   - JoinCommuteRule: Swaps join operands
   - JoinAssociateRule: Restructures join trees
   - JoinPushThroughJoinRule: Reorders multi-way joins

2. **Join cardinality reduction:**
   - SemiJoinRule: When only checking existence
   - FilterIntoJoinRule: Pushes predicates into joins
   - AggregateJoinTransposeRule: Aggregates before joining

3. **Join width reduction:**
   - ProjectJoinTransposeRule: Only projects needed columns
   - ProjectTableScanRule: Column pruning at source

---

## Implementation Patterns

### 1. Rule Definition Pattern

```java
public class MyOptimizationRule extends RelRule<MyOptimizationRule.Config> {

  protected MyOptimizationRule(Config config) {
    super(config);
  }

  @Override
  public void onMatch(RelOptRuleCall call) {
    // Get matched relational expressions
    final RelNode input = call.rel(0);

    // Perform transformation logic
    final RelNode transformed = transform(input);

    // Register transformed expression
    call.transformTo(transformed);
  }

  // Configuration interface
  public interface Config extends RelRule.Config {
    Config DEFAULT = EMPTY
        .withOperandSupplier(b ->
            b.operand(LogicalFilter.class)
             .oneInput(b2 -> b2.operand(LogicalProject.class).anyInputs()))
        .as(Config.class);

    @Override default MyOptimizationRule toRule() {
      return new MyOptimizationRule(this);
    }
  }
}
```

**Key Components:**
- **Operand pattern**: Defines what to match (class, traits, predicates)
- **onMatch()**: Transformation logic
- **Config interface**: Rule configuration and factory
- **transformTo()**: Registers equivalent expression

---

### 2. Operand Matching Patterns

**Simple Single-Input:**
```java
b.operand(LogicalFilter.class)
  .oneInput(b2 -> b2.operand(LogicalProject.class).anyInputs())
```

**Multi-Input (Join):**
```java
b.operand(LogicalJoin.class)
  .inputs(
    b2 -> b2.operand(LogicalFilter.class).anyInputs(),
    b3 -> b3.operand(LogicalTableScan.class).noInputs())
```

**With Predicates:**
```java
b.operand(LogicalFilter.class)
  .predicate(filter -> !filter.containsCorrelation())
  .oneInput(...)
```

**With Traits:**
```java
b.operand(LogicalProject.class)
  .trait(Convention.NONE)
  .anyInputs()
```

---

### 3. Expression Transformation Patterns

**Using RelBuilder (Recommended):**
```java
@Override
public void onMatch(RelOptRuleCall call) {
  final Filter filter = call.rel(0);
  final Project project = call.rel(1);

  final RelBuilder builder = call.builder();
  final RelNode transformed = builder
      .push(project.getInput())
      .filter(RexUtil.shift(filter.getCondition(), -project.getProjects().size()))
      .project(project.getProjects())
      .build();

  call.transformTo(transformed);
}
```

**Direct Construction:**
```java
@Override
public void onMatch(RelOptRuleCall call) {
  final Filter filter = call.rel(0);
  final Project project = call.rel(1);

  final RexNode newCondition = adjustCondition(filter.getCondition(), project);
  final Filter newFilter = filter.copy(
      filter.getTraitSet(),
      project.getInput(),
      newCondition);

  final Project newProject = project.copy(
      project.getTraitSet(),
      newFilter,
      project.getProjects(),
      project.getRowType());

  call.transformTo(newProject);
}
```

---

### 4. Trait Handling Patterns

**Preserving Traits:**
```java
final RelNode newRel = oldRel.copy(
    oldRel.getTraitSet(),  // Preserve traits
    newInputs);
```

**Converting Traits:**
```java
final RelTraitSet traits = rel.getTraitSet()
    .replace(Convention.PHYSICAL)
    .replace(RelCollations.of(sortKeys));

final RelNode converted = rel.copy(traits, inputs);
```

**Trait Propagation:**
```java
call.transformTo(newRel, ImmutableMap.of(), RelHintsPropagator.DEFAULT);
```

---

### 5. Cost Estimation Patterns

**Computing Cost:**
```java
@Override
public RelOptCost computeSelfCost(RelOptPlanner planner, RelMetadataQuery mq) {
  final double rowCount = mq.getRowCount(this);
  final double cpu = rowCount * getProjects().size();
  final double io = 0; // No I/O for in-memory operation

  return planner.getCostFactory().makeCost(rowCount, cpu, io);
}
```

**Using Metadata:**
```java
final RelMetadataQuery mq = call.getMetadataQuery();
final Double selectivity = mq.getSelectivity(filter, condition);
final Double inputRowCount = mq.getRowCount(input);
final double outputRowCount = inputRowCount * selectivity;
```

---

## Best Practices for Rule Ordering

### Phase-Based Optimization Strategy

#### Phase 1: Subquery Removal and Decorrelation
**Goal:** Eliminate subqueries and correlation
```
Rules:
- SubQueryRemoveRule
- JoinToCorrelateRule (if needed)
- FilterCorrelateRule
```

**Why First:**
- Exposes join opportunities
- Enables broader optimization
- Simplifies subsequent rule matching

---

#### Phase 2: Expression Simplification
**Goal:** Constant folding and expression reduction
```
Rules:
- ReduceExpressionsRule (all variants)
- FilterReduceExpressionsRule
- ProjectReduceExpressionsRule
- JoinReduceExpressionsRule
- ReduceDecimalsRule
```

**Why Early:**
- Reduces expression complexity
- Enables better cost estimation
- May eliminate entire operators

---

#### Phase 3: Predicate Pushdown and Filter Optimization
**Goal:** Push filters as early as possible
```
Rules:
- FilterMergeRule
- FilterProjectTransposeRule
- FilterTableScanRule
- FilterIntoJoinRule
- JoinConditionPushRule
- FilterAggregateTransposeRule (selective)
```

**Why After Simplification:**
- Simplified predicates push better
- Cleaner join conditions
- Better selectivity estimates

---

#### Phase 4: Projection Optimization
**Goal:** Column pruning and projection elimination
```
Rules:
- ProjectMergeRule
- ProjectRemoveRule
- ProjectTableScanRule
- ProjectJoinTransposeRule
- ProjectFilterTransposeRule
```

**Why After Filters:**
- Filters may eliminate needed columns
- Cleaner projection patterns
- Better column pruning opportunities

---

#### Phase 5: Join Optimization
**Goal:** Optimal join ordering and type selection
```
Rules:
- JoinToMultiJoinRule (flatten joins first)
- JoinCommuteRule
- JoinAssociateRule
- JoinPushThroughJoinRule
- SemiJoinRule
- JoinExtractFilterRule
```

**Why Mid-Pipeline:**
- Requires clean predicates and projections
- Before aggregation to enable pre-join aggregation
- Explores multiple join orders

---

#### Phase 6: Aggregation Optimization
**Goal:** Optimize grouping and aggregate functions
```
Rules:
- AggregatePullUpConstantsRule
- AggregateReduceFunctionsRule
- AggregateMergeRule
- AggregateProjectMergeRule
- AggregateJoinTransposeRule
- AggregateRemoveRule
```

**Why After Joins:**
- May push aggregates before joins
- Clean join results
- Better cardinality estimates

---

#### Phase 7: Operator Fusion
**Goal:** Combine operators for efficiency
```
Rules:
- CalcMergeRule
- FilterToCalcRule + ProjectToCalcRule (enable fusion)
- FilterCalcMergeRule
- ProjectCalcMergeRule
```

**Why Late:**
- After all logical optimizations
- Combines stable operators
- Reduces operator count

---

#### Phase 8: Sort and Limit Optimization
**Goal:** Eliminate redundant orderings
```
Rules:
- SortRemoveRule
- SortRemoveConstantKeysRule
- SortMergeRule
- SortProjectTransposeRule
```

**Why Late:**
- After trait propagation
- Collation information stable
- Before physical conversion

---

#### Phase 9: Dead Code Elimination
**Goal:** Remove empty relations and unused operations
```
Rules:
- PruneEmptyRules (all variants)
- AggregateRemoveRule
- ProjectRemoveRule
- UnionRemoveRule
```

**Why Last (Logical):**
- After all optimizations
- May expose new opportunities
- Final cleanup

---

#### Phase 10: Physical Planning
**Goal:** Convert to physical operators
```
Rules:
- Convention conversion rules
- Physical implementation rules
- Trait enforcement rules
```

**Why Last:**
- After logical optimization complete
- Selects physical implementations
- Cost-based selection

---

### Rule Interaction Guidelines

**1. Enable Then Optimize:**
- Enable rules (e.g., FilterToCalcRule) before merge rules (CalcMergeRule)
- Flatten structures (JoinToMultiJoinRule) before reordering

**2. Top-Down for Pushdown:**
- Predicate pushdown works better top-down
- Eliminates data early in pipeline

**3. Bottom-Up for Elimination:**
- Dead code elimination works better bottom-up
- Removes unused leaf operations

**4. Iterate Until Stable:**
- Some rules enable others
- Run phases until fixed point
- HepPlanner handles this automatically

**5. Cost-Based Selection:**
- Use VolcanoPlanner for multiple alternatives
- HepPlanner for deterministic sequences
- Combine both: HepPlanner for logical, VolcanoPlanner for physical

---

### Example Rule Ordering for PPL

```java
// Phase 1: Subquery removal
HepProgramBuilder builder = new HepProgramBuilder();
builder.addRuleInstance(CoreRules.SUB_QUERY_REMOVE);

// Phase 2: Expression simplification
builder.addRuleInstance(CoreRules.FILTER_REDUCE_EXPRESSIONS);
builder.addRuleInstance(CoreRules.PROJECT_REDUCE_EXPRESSIONS);
builder.addRuleInstance(CoreRules.JOIN_REDUCE_EXPRESSIONS);

// Phase 3: Filter optimization
builder.addRuleInstance(CoreRules.FILTER_MERGE);
builder.addRuleInstance(CoreRules.FILTER_PROJECT_TRANSPOSE);
builder.addRuleInstance(CoreRules.FILTER_INTO_JOIN);
builder.addRuleInstance(CoreRules.FILTER_TABLE_SCAN);

// Phase 4: Projection optimization
builder.addRuleInstance(CoreRules.PROJECT_MERGE);
builder.addRuleInstance(CoreRules.PROJECT_REMOVE);
builder.addRuleInstance(CoreRules.PROJECT_TABLE_SCAN);
builder.addRuleInstance(CoreRules.PROJECT_JOIN_TRANSPOSE);

// Phase 5: Join optimization
builder.addRuleInstance(CoreRules.JOIN_TO_MULTI_JOIN);
builder.addRuleInstance(CoreRules.JOIN_COMMUTE);
builder.addRuleInstance(CoreRules.JOIN_ASSOCIATE);
builder.addRuleInstance(CoreRules.SEMI_JOIN);

// Phase 6: Aggregation optimization
builder.addRuleInstance(CoreRules.AGGREGATE_PROJECT_PULL_UP_CONSTANTS);
builder.addRuleInstance(CoreRules.AGGREGATE_REDUCE_FUNCTIONS);
builder.addRuleInstance(CoreRules.AGGREGATE_MERGE);
builder.addRuleInstance(CoreRules.AGGREGATE_PROJECT_MERGE);
builder.addRuleInstance(CoreRules.AGGREGATE_JOIN_TRANSPOSE);
builder.addRuleInstance(CoreRules.AGGREGATE_REMOVE);

// Phase 7: Operator fusion
builder.addRuleInstance(CoreRules.FILTER_TO_CALC);
builder.addRuleInstance(CoreRules.PROJECT_TO_CALC);
builder.addRuleInstance(CoreRules.CALC_MERGE);

// Phase 8: Sort optimization
builder.addRuleInstance(CoreRules.SORT_REMOVE);
builder.addRuleInstance(CoreRules.SORT_REMOVE_CONSTANT_KEYS);

// Phase 9: Cleanup
builder.addRuleInstance(CoreRules.UNION_REMOVE);
// Add PruneEmptyRules as needed

HepPlanner planner = new HepPlanner(builder.build());
```

---

## Transformation Examples

### Example 1: Filter Pushdown

**Original PPL Query:**
```
source = logs
| fields user_id, action, timestamp
| where action = "login"
| where timestamp > "2024-01-01"
```

**Initial Relational Algebra:**
```
Project(user_id, action, timestamp)
  Filter(timestamp > '2024-01-01')
    Filter(action = 'login')
      Project(user_id, action, timestamp)
        TableScan(logs)
```

**After FilterMergeRule:**
```
Project(user_id, action, timestamp)
  Filter(action = 'login' AND timestamp > '2024-01-01')
    Project(user_id, action, timestamp)
      TableScan(logs)
```

**After ProjectMergeRule:**
```
Project(user_id, action, timestamp)
  Filter(action = 'login' AND timestamp > '2024-01-01')
    TableScan(logs)
```

**After FilterProjectTransposeRule:**
```
Project(user_id, action, timestamp)
  Filter(action = 'login' AND timestamp > '2024-01-01')
    TableScan(logs)
// (No change - filter already optimal position)
```

**After FilterTableScanRule:**
```
Project(user_id, action, timestamp)
  TableScan(logs, filter=[action = 'login' AND timestamp > '2024-01-01'])
```

**After ProjectTableScanRule:**
```
TableScan(logs,
          filter=[action = 'login' AND timestamp > '2024-01-01'],
          project=[user_id, action, timestamp])
```

**Result:** Single operator with pushed filter and projection.

---

### Example 2: Aggregation Optimization

**Original PPL Query:**
```
source = logs
| stats count() by user_id, "SUCCESS" as status
| stats sum(count) by user_id
```

**Initial Relational Algebra:**
```
Aggregate(group=[user_id], agg=[SUM(count)])
  Project(user_id, count, status)
    Aggregate(group=[user_id, 'SUCCESS'], agg=[COUNT()])
      TableScan(logs)
```

**After AggregatePullUpConstantsRule (lower aggregate):**
```
Aggregate(group=[user_id], agg=[SUM(count)])
  Project(user_id, count, 'SUCCESS' as status)
    Aggregate(group=[user_id], agg=[COUNT()])  // Constant removed from group
      TableScan(logs)
```

**After AggregateMergeRule:**
```
// Check if compatible: Both group by user_id, top is subset
// Both use splittable functions (COUNT → SUM)
Aggregate(group=[user_id], agg=[COUNT()])
  TableScan(logs)
// Single aggregate replaces two
```

**Result:** Single aggregation instead of nested aggregations.

---

### Example 3: Join Optimization

**Original PPL Query:**
```
source = logs
| join type=inner users on user_id
| where logs.timestamp > "2024-01-01"
| where users.status = "active"
| fields logs.action, users.name
```

**Initial Relational Algebra:**
```
Project(action, name)
  Filter(users.status = 'active')
    Filter(logs.timestamp > '2024-01-01')
      Join(logs.user_id = users.user_id)
        TableScan(logs)
        TableScan(users)
```

**After FilterMergeRule:**
```
Project(action, name)
  Filter(logs.timestamp > '2024-01-01' AND users.status = 'active')
    Join(logs.user_id = users.user_id)
      TableScan(logs)
      TableScan(users)
```

**After FilterIntoJoinRule:**
```
Project(action, name)
  Join(logs.user_id = users.user_id)
    Filter(logs.timestamp > '2024-01-01')
      TableScan(logs)
    Filter(users.status = 'active')
      TableScan(users)
```

**After FilterTableScanRule:**
```
Project(action, name)
  Join(logs.user_id = users.user_id)
    TableScan(logs, filter=[timestamp > '2024-01-01'])
    TableScan(users, filter=[status = 'active'])
```

**After ProjectJoinTransposeRule:**
```
Join(logs.user_id = users.user_id)
  Project(user_id, action)  // Only needed columns from logs
    TableScan(logs, filter=[timestamp > '2024-01-01'])
  Project(user_id, name)    // Only needed columns from users
    TableScan(users, filter=[status = 'active'])
```

**After ProjectTableScanRule:**
```
Join(logs.user_id = users.user_id)
  TableScan(logs,
           filter=[timestamp > '2024-01-01'],
           project=[user_id, action])
  TableScan(users,
           filter=[status = 'active'],
           project=[user_id, name])
```

**Result:** Filters pushed to scans, only needed columns projected.

---

### Example 4: Complex Stats with Function Reduction

**Original PPL Query:**
```
source = logs
| stats avg(response_time) by service
```

**Initial Relational Algebra:**
```
Aggregate(group=[service], agg=[AVG(response_time)])
  TableScan(logs)
```

**After AggregateReduceFunctionsRule:**
```
Project(service, SUM(response_time) / COUNT(response_time) as avg_response_time)
  Aggregate(group=[service],
           agg=[SUM(response_time), COUNT(response_time)])
    TableScan(logs)
```

**Result:** AVG decomposed to SUM/COUNT for better parallelization and merging.

---

### Example 5: Semi-Join Optimization

**Original PPL Query:**
```
source = logs
| join type=inner active_users on user_id
| fields DISTINCT logs.*
```

**Initial Relational Algebra:**
```
Project(logs.*)
  Aggregate(group=[all logs columns])  // DISTINCT
    Join(logs.user_id = active_users.user_id)
      TableScan(logs)
      TableScan(active_users)
```

**After SemiJoinRule:**
```
// Detected: Join + Aggregate with empty aggregate on right side
// Right side only used for filtering (semi-join pattern)
Project(logs.*)
  SemiJoin(logs.user_id = active_users.user_id)
    TableScan(logs)
    Project(user_id)
      TableScan(active_users)
```

**Result:** SemiJoin instead of full join + aggregate (more efficient).

---

## Conclusion

### Summary of Key Findings

1. **100+ Optimization Rules**: Calcite provides comprehensive coverage across all query patterns
2. **Two-Tier Planning**: VolcanoPlanner for cost-based, HepPlanner for heuristic optimization
3. **Sophisticated Cost Model**: Row count, CPU, I/O metrics with extensibility
4. **Rich Metadata System**: Cardinality, selectivity, and lineage tracking
5. **Phase-Based Optimization**: 10 phases from subquery removal to physical planning

### Top Recommendations for PPL Implementation

**1. Start with Core Rules (Tier 1):**
- Focus on filter and projection optimization first
- Implement predicate pushdown and column pruning
- Add expression simplification early

**2. Use HepPlanner for Initial Implementation:**
- Simpler to understand and implement
- Deterministic rule ordering
- Good enough for most queries
- Upgrade to VolcanoPlanner for complex joins

**3. Implement in Phases:**
- Phase 1: Filter + Projection rules
- Phase 2: Expression simplification
- Phase 3: Aggregation rules
- Phase 4: Join optimization
- Phase 5: Advanced rules (semi-join, subqueries)

**4. Leverage Existing Infrastructure:**
- Use `CoreRules` constants instead of creating rule instances
- Utilize `RelBuilder` for transformations (cleaner API)
- Leverage `RelOptUtil` for common operations
- Use `RexSimplify` for expression optimization

**5. Focus on Log Analytics Patterns:**
- Time-range filter optimization (critical for logs)
- Pre-join aggregation (common pattern)
- Column pruning (wide log schemas)
- Constant folding (time buckets, static filters)

### Next Steps for PPL Integration

1. **Map PPL Commands to Relational Algebra:**
   - Define RelNode equivalents for PPL operators
   - Create PPL-specific conventions if needed

2. **Implement Rule Registration:**
   - Start with Tier 1 rules
   - Add rules incrementally based on query patterns
   - Monitor performance impact

3. **Configure Planners:**
   - Set up HepProgram with phased rule application
   - Define cost factory for PPL-specific costs
   - Implement metadata providers for log-specific statistics

4. **Add Custom Rules:**
   - Time-series optimizations
   - Log-specific patterns (e.g., span commands)
   - PPL-specific aggregate functions

5. **Testing and Validation:**
   - Compare optimized vs non-optimized plans
   - Measure query performance improvements
   - Validate semantic equivalence

### Resources

- **Apache Calcite Documentation**: https://calcite.apache.org/docs/
- **Calcite Repository**: https://github.com/apache/calcite
- **Rule Source Code**: `/core/src/main/java/org/apache/calcite/rel/rules/`
- **CoreRules Reference**: `/core/src/main/java/org/apache/calcite/rel/rules/CoreRules.java`
- **Algebra Documentation**: https://calcite.apache.org/docs/algebra.html

---

**End of Research Document**
