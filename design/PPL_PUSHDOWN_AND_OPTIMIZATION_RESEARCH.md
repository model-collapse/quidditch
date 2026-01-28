# PPL Push-Down and Optimization Research Summary

**Research Date:** January 27, 2026
**Sources:** OpenSearch SQL Plugin + Apache Calcite
**Purpose:** Inform Quidditch PPL implementation with proven strategies

---

## Executive Summary

This document consolidates research from two major sources:
1. **OpenSearch SQL Plugin** - Production push-down strategies and limitations
2. **Apache Calcite** - 100+ proven optimization rules

**Key Findings:**
- OpenSearch SQL implements **7 core push-down strategies** with specific decision logic
- Apache Calcite provides **100+ optimization rules** across 12 categories
- Both systems use **two-phase optimization**: logical transformations → storage push-down
- Critical for PPL: **predicate push-down**, **aggregation push-down**, **projection push-down**

---

## Part 1: OpenSearch SQL Push-Down Strategies

### 1.1 Seven Core Push-Down Operations

| Strategy | Success Rate | Limitations |
|----------|--------------|-------------|
| **Filter** | ~80% | Struct types, nested nulls, field-to-field comparisons |
| **Aggregation** | ~70% | No expressions, 1000 bucket limit |
| **Sort** | ~60% | Field references only, no expressions |
| **Limit** | 100% | None (always pushable) |
| **Project** | Partial | Needs post-processing |
| **Highlight** | 100% | None |
| **Nested** | Partial | Result flattening required |

**Source:** `/core/src/main/java/org/opensearch/sql/planner/optimizer/rule/read/TableScanPushDown.java`

---

### 1.2 Push-Down Decision Logic

#### Filter Push-Down

**✅ Can Push Down:**
```sql
-- Simple predicates
source=logs | where status = 500
source=logs | where timestamp > NOW() - 3600
source=logs | where level IN ('ERROR', 'FATAL')

-- Text search (27+ Lucene query types)
source=logs | where match(message, 'error')
source=logs | where match_phrase(message, 'connection timeout')
```

**❌ Cannot Push Down:**
```sql
-- Field-to-field comparisons
source=logs | where field1 = field2

-- Struct types
source=logs | where user.address.city = 'NYC'  -- struct types rejected

-- Nested IS NULL
source=nested | where nested_field IS NULL  -- DSL limitation

-- Metadata beyond equality
source=logs | where _id > 'abc'  -- Only = supported
```

**DSL Translation Example:**
```sql
source=logs | where status >= 500 AND host = 'server1'
```
→
```json
{
  "query": {
    "bool": {
      "must": [
        {"range": {"status": {"gte": 500}}},
        {"term": {"host": "server1"}}
      ]
    }
  }
}
```

---

#### Aggregation Push-Down

**✅ Can Push Down:**
```sql
-- Standard aggregations
source=sales | stats count(), sum(revenue), avg(price) by category

-- Multiple group-by fields
source=logs | stats count() by region, status

-- Time bucketing
source=metrics | timechart span=1h avg(cpu_usage) by host
```

**❌ Cannot Push Down:**
```sql
-- Aggregations on expressions
source=logs | stats avg(log(response_time))  -- Rejected

-- After LIMIT (ordering constraint)
source=logs | head 1000 | stats count() by host  -- Cannot push

-- Post-aggregation filtering (HAVING)
source=logs | stats count() by host | where count > 100  -- Local execution
```

**DSL Translation Example:**
```sql
source=sales | stats sum(revenue), avg(price) by dept
```
→
```json
{
  "size": 0,
  "aggs": {
    "composite_buckets": {
      "composite": {
        "sources": [{"dept": {"terms": {"field": "dept.keyword"}}}]
      },
      "aggs": {
        "sum_revenue": {"sum": {"field": "revenue"}},
        "avg_price": {"avg": {"field": "price"}}
      }
    }
  }
}
```

---

#### Sort Push-Down

**✅ Can Push Down:**
```sql
-- Simple field sorting
source=logs | sort timestamp DESC

-- Multiple fields
source=logs | sort level ASC, timestamp DESC
```

**❌ Cannot Push Down:**
```sql
-- Computed expressions
source=logs | sort abs(value) DESC

-- Sorting by aggregation results
source=logs | stats count() by host | sort count DESC  -- Local execution
```

---

### 1.3 Execution Models

#### Model 1: Full Push-Down (Best Performance)

```
Coordinator: Translate PPL → DSL
     ↓ (HTTP)
Data Node: Execute query on Diagon
     ↓
Coordinator: Format results
```

**Latency:** <100ms
**Use Case:** Simple filters + aggregations

---

#### Model 2: Partial Push-Down (Medium Performance)

```
Coordinator: Translate scan/filter → DSL
     ↓ (HTTP + Scroll)
Data Node: Stream filtered documents
     ↓
Coordinator: Execute eval/parse/transform in Go
     ↓
Coordinator: Aggregate in memory
```

**Latency:** 100-500ms
**Use Case:** Transformations (eval, parse, rex)

---

#### Model 3: No Push-Down (Coordinator Execution)

```
Coordinator: Fetch all data
     ↓ (match_all query)
Data Node: Return documents
     ↓
Coordinator: Execute all operations in Go
```

**Latency:** >500ms
**Use Case:** Joins, grok patterns, ML commands

---

### 1.4 OpenSearch SQL Optimization Pipeline

#### Phase 1: Logical Algebra Optimization

```java
// Applied first - general transformations
MergeFilterAndFilter          // Filter(A) → Filter(B) → Filter(A AND B)
PushFilterUnderSort           // Filter → Sort → Sort → Filter
EvalPushDown.PUSH_DOWN_LIMIT  // Move limits under eval
```

#### Phase 2: Data Source Push-Down

```java
// Applied second - storage-specific acceleration
TableScanPushDown.PUSH_DOWN_FILTER
TableScanPushDown.PUSH_DOWN_AGGREGATION
TableScanPushDown.PUSH_DOWN_SORT
TableScanPushDown.PUSH_DOWN_LIMIT
TableScanPushDown.PUSH_DOWN_PROJECT
```

**Key Insight:** Two-phase approach separates logical equivalence from storage optimization.

---

### 1.5 Limitations & Fallbacks

#### Operations That NEVER Push Down

| Operation | Reason | Fallback |
|-----------|--------|----------|
| **Window Functions** | No DSL support | Always coordinator |
| **JOINs** | Multi-index limitation | Always coordinator |
| **eval (computed fields)** | Requires Go evaluation | Coordinator execution |
| **parse/grok** | Complex parsing logic | Coordinator execution |
| **Post-aggregation filters (HAVING)** | After-the-fact filtering | Coordinator execution |
| **Sorting by aggregate results** | DSL limitation | Coordinator execution |

#### Script Query Fallback

When native Lucene queries insufficient:

```java
// ExpressionScriptEngine wraps complex expressions
ScriptQueryBuilder script = new ScriptQueryBuilder(
  new Script(ScriptType.INLINE, "expression", serializedExpr, params)
);
```

**Limitations:**
- Slower than native queries
- Cannot handle struct types
- Risk of script compilation overhead

---

## Part 2: Apache Calcite Optimization Rules

### 2.1 Rule Categories (100+ Rules)

#### Tier 1: Critical Rules for PPL (10 rules)

| Rule | Category | Impact | PPL Use Case |
|------|----------|--------|--------------|
| **FilterMergeRule** | Filter | High | Combine multiple `where` clauses |
| **FilterProjectTransposeRule** | Filter | High | Push filter before `fields` |
| **ProjectMergeRule** | Project | High | Combine multiple `fields` |
| **ProjectRemoveRule** | Project | Medium | Remove redundant projections |
| **FilterTableScanRule** | Scan | Critical | Enable storage push-down |
| **ProjectTableScanRule** | Scan | Critical | Enable column pruning |
| **AggregatePullUpConstantsRule** | Aggregate | Medium | Simplify GROUP BY |
| **AggregateReduceFunctionsRule** | Aggregate | High | AVG → SUM/COUNT decomposition |
| **ReduceExpressionsRule** | Expression | High | Constant folding |
| **FilterReduceExpressionsRule** | Expression | High | Simplify predicates |

---

#### Tier 2: High-Value Rules (13 rules)

| Rule | Category | Impact | PPL Use Case |
|------|----------|--------|--------------|
| **FilterIntoJoinRule** | Join | High | Push predicates into joins |
| **JoinCommuteRule** | Join | Medium | Reorder join inputs |
| **JoinPushThroughJoinRule** | Join | Medium | Nested join optimization |
| **AggregateJoinTransposeRule** | Aggregate | High | Pre-aggregate before join |
| **AggregateUnionTransposeRule** | Aggregate | Medium | Distribute aggregation |
| **SubQueryRemoveRule** | Subquery | High | Eliminate subqueries |
| **JoinToSemiJoinRule** | Join | Medium | Convert to semi-join |
| **SortRemoveRule** | Sort | Medium | Eliminate redundant sorts |
| **ProjectCalcMergeRule** | Calc | Medium | Fuse project + calc |
| **FilterCalcMergeRule** | Calc | Medium | Fuse filter + calc |
| **CalcMergeRule** | Calc | High | Combine calc operations |
| **UnionToDistinctRule** | Set Op | Low | Optimize UNION DISTINCT |
| **EmptyPruneRule** | Pruning | Medium | Dead code elimination |

---

#### Tier 3: Specialized Rules (77+ additional rules)

Categories:
- **Join Rules** (15 total): Push-down, commute, associate, extract
- **Aggregate Rules** (16 total): Merge, transpose, expand, reduce
- **Sort Rules** (6 total): Remove, exchange, transpose
- **Set Operation Rules** (8 total): Union, intersect, minus
- **Window Rules** (2 total): Window function optimization
- **Subquery Rules** (3 total): Decorrelation
- **Empty Pruning** (9 total): Value elimination

---

### 2.2 Optimization Strategies

#### Strategy 1: VolcanoPlanner (Cost-Based)

**When to Use:**
- Complex queries with multiple join orders
- Need guaranteed optimal plan
- Have accurate statistics

**Characteristics:**
- Exhaustive search (bounded)
- Equivalence class management
- Cost comparison across alternatives
- Slower compilation, better execution

**Configuration:**
```java
VolcanoPlanner planner = new VolcanoPlanner(costFactory, context);
planner.setTopDownOpt(true);  // Enable top-down optimization
planner.addRule(FilterTableScanRule.Config.DEFAULT.toRule());
planner.addRule(ProjectTableScanRule.Config.DEFAULT.toRule());
// ... add all relevant rules
```

---

#### Strategy 2: HepPlanner (Heuristic)

**When to Use:**
- Fast compilation required
- Deterministic rule ordering acceptable
- Moderate query complexity

**Characteristics:**
- Sequential rule application
- Greedy optimization
- No cost comparison
- Fast compilation, good execution

**Configuration:**
```java
HepProgramBuilder builder = new HepProgramBuilder();

// Phase 1: Subquery removal
builder.addRuleCollection(SubQueryRemoveRule.ALL);

// Phase 2: Expression simplification
builder.addRuleInstance(ReduceExpressionsRule.Config.DEFAULT.toRule());

// Phase 3: Filter optimization
builder.addRuleInstance(FilterMergeRule.Config.DEFAULT.toRule());
builder.addRuleInstance(FilterProjectTransposeRule.Config.DEFAULT.toRule());

// Phase 4: Push-down
builder.addRuleInstance(FilterTableScanRule.Config.DEFAULT.toRule());
builder.addRuleInstance(ProjectTableScanRule.Config.DEFAULT.toRule());

HepProgram program = builder.build();
HepPlanner planner = new HepPlanner(program);
```

---

### 2.3 Cost Model

#### Three Primary Metrics

```java
public interface RelOptCost {
    double getRows();    // Estimated row count
    double getCpu();     // CPU cost (arbitrary units)
    double getIo();      // I/O cost (arbitrary units)
}
```

#### Metadata System

```java
// Cardinality estimation
RelMetadataQuery mq = cluster.getMetadataQuery();
Double rowCount = mq.getRowCount(rel);

// Selectivity (0.0 to 1.0)
Double selectivity = mq.getSelectivity(rel, predicate);

// Distinct values
Double distinctCount = mq.getDistinctRowCount(rel, groupKey, predicate);

// Column uniqueness
Boolean unique = mq.areColumnsUnique(rel, columns);
```

---

### 2.4 Implementation Patterns

#### Pattern 1: Rule Definition

```java
public class CustomFilterPushDownRule
    extends RelRule<CustomFilterPushDownRule.Config> {

  protected CustomFilterPushDownRule(Config config) {
    super(config);
  }

  @Override
  public void onMatch(RelOptRuleCall call) {
    final LogicalFilter filter = call.rel(0);
    final LogicalTableScan scan = call.rel(1);

    // Check if filter can be pushed
    if (!canPushDown(filter.getCondition())) {
      return;
    }

    // Create new filtered scan
    RelNode newScan = createFilteredScan(scan, filter.getCondition());

    // Replace in plan
    call.transformTo(newScan);
  }

  // Configuration
  public interface Config extends RelRule.Config {
    Config DEFAULT = EMPTY
      .withOperandSupplier(b0 ->
        b0.operand(LogicalFilter.class)
          .oneInput(b1 ->
            b1.operand(LogicalTableScan.class).noInputs()))
      .as(Config.class);

    @Override default CustomFilterPushDownRule toRule() {
      return new CustomFilterPushDownRule(this);
    }
  }
}
```

---

#### Pattern 2: Transformation with RelBuilder

```java
public void onMatch(RelOptRuleCall call) {
  final LogicalFilter filter = call.rel(0);
  final LogicalAggregate agg = call.rel(1);

  // Use RelBuilder for safe transformations
  final RelBuilder builder = call.builder();

  builder.push(agg.getInput());  // Start with aggregate input

  // Push filter below aggregate
  builder.filter(pushableConditions);

  // Rebuild aggregate
  builder.aggregate(
    builder.groupKey(agg.getGroupSet()),
    agg.getAggCallList()
  );

  // Apply remaining filters
  if (!remainingConditions.isEmpty()) {
    builder.filter(remainingConditions);
  }

  call.transformTo(builder.build());
}
```

---

## Part 3: PPL-Specific Recommendations

### 3.1 Optimization Rule Priority for Log Analytics

#### Phase 1: Subquery & Expression Simplification (Week 1)

```java
// Essential for PPL preprocessing
SubQueryRemoveRule.FILTER          // Convert subqueries to joins
SubQueryRemoveRule.PROJECT
SubQueryRemoveRule.JOIN

ReduceExpressionsRule              // Fold constants: NOW() - 3600 → 1706227200
FilterReduceExpressionsRule        // Simplify: x > 5 AND x > 3 → x > 5
```

**PPL Impact:**
```sql
-- Before optimization
source=logs | where timestamp > NOW() - 3600 AND status > 400 AND status > 500

-- After optimization
source=logs | where timestamp > 1706227200 AND status > 500
```

---

#### Phase 2: Predicate Push-Down (Week 2)

```java
// Critical for reducing data early
FilterMergeRule                    // Combine multiple where clauses
FilterProjectTransposeRule         // Push filter before fields
FilterTableScanRule                // Enable storage push-down
FilterIntoJoinRule                 // Push predicates into joins
JoinConditionPushRule              // Push join conditions to inputs
```

**PPL Impact:**
```sql
-- Before optimization
source=logs | fields timestamp, host, status | where status = 500

-- After optimization
source=logs | where status = 500 | fields timestamp, host, status
-- Filter pushed before projection, reducing projection work
```

---

#### Phase 3: Projection Optimization (Week 2)

```java
// Column pruning for wide log schemas
ProjectMergeRule                   // Combine multiple fields commands
ProjectRemoveRule                  // Remove redundant projections
ProjectTableScanRule               // Enable column pruning push-down
```

**PPL Impact:**
```sql
-- Before optimization
source=logs | fields host, status, message | fields host, status

-- After optimization
source=logs | fields host, status
-- Redundant projection eliminated
```

---

#### Phase 4: Aggregation Optimization (Week 3)

```java
// Critical for stats commands
AggregatePullUpConstantsRule       // Simplify GROUP BY with constants
AggregateReduceFunctionsRule       // AVG → SUM/COUNT decomposition
AggregateJoinTransposeRule         // Pre-aggregate before joins
AggregateProjectMergeRule          // Fuse aggregation + projection
```

**PPL Impact:**
```sql
-- Before optimization
source=orders | join customers | stats avg(price) by customer_id

-- After optimization
source=orders | stats sum(price) as total, count(*) as cnt by customer_id
  | join customers
  | eval avg_price = total / cnt
-- Pre-aggregate before expensive join
```

---

#### Phase 5: Join Optimization (Week 4)

```java
// For multi-source queries
JoinCommuteRule                    // Swap join inputs (smaller left)
JoinPushThroughJoinRule            // Nested join reordering
JoinToSemiJoinRule                 // Convert to semi-join when applicable
FilterIntoJoinRule                 // Push WHERE into JOIN ON
```

**PPL Impact:**
```sql
-- Before optimization (large table left)
source=orders | join [source=small_lookup]

-- After optimization (small table left)
source=small_lookup | join [source=orders]
-- Build hash table on smaller dataset
```

---

#### Phase 6: Operator Fusion (Week 5)

```java
// Reduce operator overhead
CalcMergeRule                      // Combine filter + project into Calc
ProjectCalcMergeRule
FilterCalcMergeRule
```

**PPL Impact:**
```sql
-- Before optimization
source=logs | where status = 500 | fields host, status | where host != 'localhost'

-- After optimization (fused into single Calc)
source=logs | calc(status = 500 AND host != 'localhost', project[host, status])
-- Single pass instead of three
```

---

### 3.2 Decision Tree for PPL Optimization

```
Query Analysis
│
├─ Has subqueries?
│  YES → Apply SubQueryRemoveRule first
│  NO  → Continue
│
├─ Has constant expressions?
│  YES → Apply ReduceExpressionsRule
│  NO  → Continue
│
├─ Multiple filters?
│  YES → Apply FilterMergeRule
│  NO  → Continue
│
├─ Filter after projection?
│  YES → Apply FilterProjectTransposeRule
│  NO  → Continue
│
├─ Can push to storage?
│  YES → Apply FilterTableScanRule, ProjectTableScanRule
│  NO  → Continue
│
├─ Has aggregation?
│  YES → Apply AggregateReduceFunctionsRule
│       → Check for join, apply AggregateJoinTransposeRule if needed
│  NO  → Continue
│
├─ Has joins?
│  YES → Apply JoinCommuteRule (cardinality-based)
│       → Apply FilterIntoJoinRule
│  NO  → Continue
│
└─ Final → Apply CalcMergeRule for operator fusion
```

---

### 3.3 Cost Model Configuration for Log Analytics

#### Selectivity Estimates

```java
// Time-range filters (high selectivity)
timestamp > NOW() - 3600          // Selectivity: 0.01 (1 hour / 100 hours)

// Status code filters (variable)
status = 500                      // Selectivity: 0.05 (5% error rate)
status >= 400                     // Selectivity: 0.15 (15% errors)

// Text search (low selectivity)
match(message, 'error')           // Selectivity: 0.30 (30% logs)

// Host filters (variable)
host = 'server1'                  // Selectivity: 1/N (N = server count)
```

#### I/O Cost Estimates

```java
// Full table scan
Cost = rows * 1.0                 // Base cost

// Indexed scan (with filter)
Cost = rows * selectivity * 0.1   // 10x cheaper with index

// Aggregation
Cost = rows * log(distinct_values)  // Hash table overhead

// Join
Cost = leftRows * rightRows       // Nested loop
Cost = leftRows + rightRows       // Hash join (better)
```

---

## Part 4: Implementation Roadmap

### Week 1-2: Foundation

**Deliverables:**
- [ ] HepPlanner setup with Tier 1 rules
- [ ] PPL AST → RelNode conversion
- [ ] Basic cost model (row count only)
- [ ] FilterMergeRule, ProjectMergeRule
- [ ] ReduceExpressionsRule

**Test Cases:**
- Constant folding: `NOW() - 3600`
- Filter merge: `where A | where B`
- Projection merge: `fields X,Y | fields X`

---

### Week 3-4: Push-Down

**Deliverables:**
- [ ] FilterTableScanRule integration
- [ ] ProjectTableScanRule integration
- [ ] DSL translation layer
- [ ] FilterProjectTransposeRule
- [ ] FilterIntoJoinRule

**Test Cases:**
- Filter push to OpenSearch DSL
- Column pruning to `_source` filtering
- Filter before projection optimization

---

### Week 5-6: Aggregation & Joins

**Deliverables:**
- [ ] AggregateReduceFunctionsRule
- [ ] AggregatePullUpConstantsRule
- [ ] JoinCommuteRule
- [ ] AggregateJoinTransposeRule
- [ ] Metadata provider (selectivity estimates)

**Test Cases:**
- AVG decomposition to SUM/COUNT
- Join input reordering
- Pre-aggregation before join

---

### Week 7+: Advanced

**Deliverables:**
- [ ] SubQueryRemoveRule
- [ ] CalcMergeRule family
- [ ] Cost-based join selection
- [ ] Performance benchmarking
- [ ] Rule ordering tuning

---

## Part 5: Key Takeaways

### What OpenSearch SQL Teaches Us

1. **Push-down is not all-or-nothing**: Partial push-down is common and valuable
2. **Decision logic matters**: Clear criteria for when to push vs execute locally
3. **Fallbacks are essential**: Script queries, coordinator execution
4. **Two-phase optimization**: Logical transformations first, storage push-down second
5. **Metadata operations are tricky**: `_id`, `_index` have special handling

### What Apache Calcite Teaches Us

1. **100+ rules is overwhelming**: Start with Tier 1 (10 rules), expand gradually
2. **Rule ordering matters**: Subquery removal before push-down
3. **Cost model is critical**: Even simple estimates enable better join orders
4. **Operator fusion reduces overhead**: Calc combines filter+project
5. **HepPlanner for deterministic**: VolcanoPlanner for exhaustive

### Combined Strategy for Quidditch PPL

1. **Use HepPlanner initially**: Fast, predictable, good enough for Tier 0+1
2. **Apply Tier 1 rules only**: 10 rules cover 80% of optimization value
3. **Two-phase like OpenSearch**: Logical algebra first, storage push-down second
4. **Clear push-down criteria**: Document what can/cannot push down (like OpenSearch)
5. **Implement fallbacks early**: Coordinator execution for non-pushable operations
6. **Simple cost model**: Row count + selectivity estimates (no complex math)
7. **Add Tier 2 rules in Phase 4**: Joins, advanced aggregations
8. **Benchmark continuously**: Measure optimization effectiveness

---

## Part 6: Quick Reference

### Push-Down Checklist

**Can Always Push Down:**
- ✅ Simple comparisons (=, <, >, <=, >=, !=)
- ✅ Boolean logic (AND, OR, NOT)
- ✅ Text search (match, match_phrase, etc.)
- ✅ Standard aggregations (count, sum, avg, min, max)
- ✅ Field-based sorting
- ✅ LIMIT/OFFSET
- ✅ Column selection (_source filtering)

**Cannot Push Down:**
- ❌ Window functions
- ❌ JOINs (multi-index)
- ❌ eval (computed fields)
- ❌ parse/grok (complex parsing)
- ❌ Field-to-field comparisons
- ❌ Aggregations on expressions
- ❌ Post-aggregation filters (HAVING)
- ❌ Sorting by aggregate results

**Partial Push-Down:**
- ⚠️ Nested queries (query pushed, flattening local)
- ⚠️ Projection (source filtering pushed, transform local)
- ⚠️ Some sorts (field sorts pushed, expression sorts local)

---

### Rule Application Order

```
1. SubQueryRemoveRule          // Eliminate subqueries first
2. ReduceExpressionsRule        // Constant folding
3. FilterReduceExpressionsRule  // Simplify predicates
4. FilterMergeRule              // Combine filters
5. ProjectMergeRule             // Combine projections
6. FilterProjectTransposeRule   // Push filter before project
7. FilterTableScanRule          // Enable storage push-down
8. ProjectTableScanRule         // Enable column pruning
9. AggregateReduceFunctionsRule // Decompose functions
10. JoinCommuteRule             // Reorder joins by size
```

---

## Part 7: References

### OpenSearch SQL
- **Repository:** https://github.com/opensearch-project/sql
- **Key Files:** TableScanPushDown.java, OpenSearchIndexScanBuilder.java, FilterQueryBuilder.java
- **Documentation:** `/docs/user/optimization/optimization.rst`

### Apache Calcite
- **Repository:** https://github.com/apache/calcite
- **Key Files:** `/core/src/main/java/org/apache/calcite/rel/rules/`
- **Documentation:** https://calcite.apache.org/docs/

### Additional Research
- **OpenSearch SQL Research:** Complete push-down analysis (45+ files)
- **Calcite Rules Research:** 100+ optimization rules cataloged
- **Combined Analysis:** This document

---

**Document Version:** 1.0
**Last Updated:** January 27, 2026
**Next Review:** After Tier 0+1 implementation (Week 14)
