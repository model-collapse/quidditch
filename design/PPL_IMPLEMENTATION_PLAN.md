# PPL (Piped Processing Language) Implementation Plan

## Executive Summary

**Reality Check**: PPL is far more complex than initially estimated.

- **Commands**: 44 (not 10!)
- **Functions**: 192 across 13 categories
- **Estimated Effort**: 6-9 months (not 6 weeks!)
- **Team Required**: 3-4 engineers (2 backend, 1 parser specialist, 1 QA)

**Recommendation**: Phased implementation with production-critical features first.

---

## 1. Complete Command Inventory

### Category Breakdown (44 Total Commands)

| Category | Count | Complexity | Priority |
|----------|-------|------------|----------|
| **Data Retrieval** | 3 | Medium | P0 |
| **Filtering & Selection** | 3 | Low | P0 |
| **Data Transformation** | 10 | High | P1 |
| **Aggregation & Statistics** | 7 | High | P0 |
| **Data Combination** | 6 | Very High | P1 |
| **Sorting & Ordering** | 5 | Low | P0 |
| **Machine Learning** | 4 | Very High | P3 |
| **Specialized Operations** | 6 | Medium | P2 |

### Commands by Priority

#### P0 - Critical (15 commands, 34%)
**Must-have for basic PPL queries**

```
search       - Query data from indexes
where        - Filter records
fields       - Select specific fields
stats        - Aggregate metrics
sort         - Order results
head         - Limit rows
top          - Top N values
rare         - Rare values
chart        - Summary tables
timechart    - Time-based aggregations
bin          - Histogram buckets
explain      - Query execution details
describe     - Schema information
showdatasources - List indexes
dedup        - Remove duplicates
```

**Estimated**: 8-10 weeks

#### P1 - Important (17 commands, 39%)
**Essential for real-world analytics**

```
eval         - Calculate fields
rename       - Rename fields
replace      - Substitute values
parse        - Extract structured data
grok         - Pattern-based extraction
rex          - Regex extraction
spath        - Navigate JSON
fillnull     - Handle nulls
join         - Combine datasets (inner, left only)
lookup       - Reference external data
append       - Concatenate results
subquery     - Nested queries
eventstats   - Add stats to events
streamstats  - Running statistics
flatten      - Flatten nested objects
table        - Format output
reverse      - Reverse row order
```

**Estimated**: 12-14 weeks

#### P2 - Nice-to-Have (8 commands, 18%)
**Advanced features for power users**

```
addtotals    - Add summary rows
addcoltotals - Add column totals
transpose    - Pivot rows to columns
expand       - Generate combinations
appendcol    - Add columns from query
appendpipe   - Process results further
trendline    - Trend analysis
multisearch  - Multiple searches
```

**Estimated**: 6-8 weeks

#### P3 - Future (4 commands, 9%)
**Specialized, low usage**

```
patterns     - Identify data patterns
ml           - Machine learning ops
kmeans       - Clustering (deprecated)
ad           - Anomaly detection (deprecated)
```

**Estimated**: 8-10 weeks (if needed)

---

## 2. Function Library (192 Functions)

### By Category

| Category | Count | Priority | Examples |
|----------|-------|----------|----------|
| **Date/Time** | 57 | P0 | DATE_ADD, TIMESTAMPDIFF, EXTRACT, NOW |
| **Math** | 41 | P0 | ABS, SQRT, POW, ROUND, LOG, SIN, COS |
| **Aggregation** | 20 | P0 | COUNT, SUM, AVG, MAX, MIN, PERCENTILE |
| **String** | 17 | P0 | CONCAT, SUBSTRING, UPPER, LOWER, TRIM |
| **Collection** | 15 | P1 | Array operations |
| **Conditional** | 14 | P0 | IF, CASE, COALESCE, ISNULL |
| **JSON** | 11 | P1 | json_extract, json_array, json_keys |
| **Relevance** | 7 | P0 | MATCH, MATCH_PHRASE, MULTI_MATCH |
| **Type Conversion** | 3 | P0 | CAST |
| **IP Address** | 2 | P2 | IP operations |
| **Cryptographic** | 2 | P2 | Hashing |
| **Statistical** | 2 | P3 | Statistical ops |
| **System** | 1 | P2 | System info |

### Implementation Priorities

**Phase 1** (P0 - 135 functions):
- Date/Time: 57 functions
- Math: 41 functions
- Aggregation: 20 functions
- String: 17 functions

**Phase 2** (P0/P1 - 32 functions):
- Conditional: 14 functions
- JSON: 11 functions
- Relevance: 7 functions

**Phase 3** (P1/P2 - 25 functions):
- Collections: 15 functions
- Type conversion: 3 functions
- IP: 2 functions
- Crypto: 2 functions
- Statistical: 2 functions
- System: 1 function

---

## 3. Architecture Design

### 3.1 Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    PPL Query Processing                      │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   Lexer      │───▶│   Parser     │───▶│  Analyzer    │  │
│  │  (Tokens)    │    │    (AST)     │    │ (Validation) │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│         │                    │                    │          │
│         ▼                    ▼                    ▼          │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐  │
│  │   Grammar    │    │ Logical Plan │    │  Optimizer   │  │
│  │  (ANTLR4)    │    │   Builder    │    │   (Rules)    │  │
│  └──────────────┘    └──────────────┘    └──────────────┘  │
│                             │                    │          │
│                             ▼                    ▼          │
│                      ┌──────────────┐    ┌──────────────┐  │
│                      │Physical Plan │───▶│   Executor   │  │
│                      │  (DSL/Go)    │    │  (Push/Pull) │  │
│                      └──────────────┘    └──────────────┘  │
│                                                 │            │
│                                                 ▼            │
│                                          ┌──────────────┐   │
│                                          │   Results    │   │
│                                          │  (Formatter) │   │
│                                          └──────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### 3.2 Parser Implementation

**Technology Choice**: ANTLR4 (proven, used by OpenSearch)

**Grammar Files**:
1. `PPLLexer.g4` - Token definitions (100+ tokens)
2. `PPLParser.g4` - Syntax rules (40+ commands)

**Generated Code**:
- Go target (`antlr4-go-runtime`)
- Visitor pattern for AST traversal
- Error recovery and reporting

### 3.3 Query Translation

**Two Execution Models**:

#### Push-Down (Preferred)
Converts PPL to OpenSearch DSL for data node execution:

```
source=logs | where status=500 | stats count() by host
                    ↓
{
  "query": {"term": {"status": 500}},
  "aggs": {
    "host": {
      "terms": {"field": "host"},
      "aggs": {"count": {"value_count": {"field": "*"}}}
    }
  }
}
```

**Push-able commands**: search, where, stats, sort, head, top, rare, chart, timechart

#### Coordinator Execution (Fallback)
Executes on Go coordinator node:

```
eval new_field = field1 + field2
       ↓
Go code iterates results, computes expression
```

**Non-pushable commands**: eval, transpose, patterns, ml

### 3.4 Expression Engine

**AST Node Types**:
- Logical: AND, OR, NOT, XOR
- Comparison: =, !=, <, >, <=, >=, IN, BETWEEN, LIKE
- Arithmetic: +, -, *, /, %
- Function calls: Built-in + UDFs
- Field references: Simple, nested (dot notation)

**Evaluator**:
- Type inference and coercion
- Short-circuit evaluation
- Null handling (SQL semantics)
- Integration with existing Quidditch expression evaluator

---

## 4. Implementation Phases

### Phase 1: Foundation (8-10 weeks)

**Milestone**: Basic PPL queries working end-to-end

**Components**:
1. **Parser Infrastructure** (3 weeks)
   - ANTLR4 grammar (lexer + parser)
   - AST generation and visitor
   - Error handling and recovery
   - Unit tests for grammar

2. **Core Commands** (3 weeks)
   - search, where, fields (P0)
   - sort, head (P0)
   - Basic stats (count, sum, avg, min, max)

3. **Expression Engine** (2 weeks)
   - Logical and comparison operators
   - Arithmetic operations
   - Field references
   - Integration with existing evaluator

4. **Query Translation** (2 weeks)
   - AST → Logical plan
   - Logical plan → DSL
   - Push-down optimizer
   - Integration with query executor

**Deliverables**:
- Parse 15 basic PPL commands
- Execute simple queries (source, where, fields, stats)
- 100+ parser tests
- 50+ integration tests
- Documentation: PPL Syntax Guide

**Example Working Queries**:
```
source=products | where price > 100 | fields name, price
source=logs | where status=500 | stats count() by host
source=metrics | where timestamp > "2024-01-01" | sort timestamp | head 100
```

---

### Phase 2: Aggregations & Transformations (10-12 weeks)

**Milestone**: Production-ready analytics queries

**Components**:
1. **Advanced Aggregations** (4 weeks)
   - stats: All 20 aggregation functions
   - chart: Multi-dimensional aggregations
   - timechart: Time-series bucketing with span
   - bin: Histogram bucketing
   - eventstats: Window aggregations
   - streamstats: Running aggregations

2. **Data Transformations** (4 weeks)
   - eval: Expression evaluation
   - rename: Field renaming
   - replace: Value substitution
   - parse: Structured extraction
   - grok: Pattern-based parsing
   - rex: Regex extraction

3. **Function Library** (4 weeks)
   - Math functions (41): ABS, SQRT, POW, etc.
   - String functions (17): CONCAT, SUBSTRING, etc.
   - Date/Time functions (57): DATE_ADD, EXTRACT, etc.
   - Conditional functions (14): IF, CASE, COALESCE, etc.

**Deliverables**:
- 135+ functions implemented
- 22 commands working (P0 complete)
- 200+ function tests
- 100+ command tests
- Documentation: Function Reference

**Example Working Queries**:
```
source=sales | eval revenue = price * quantity | stats sum(revenue) by category
source=logs | parse message "%{TIMESTAMP:ts} %{LOGLEVEL:level} %{GREEDYDATA:msg}"
source=metrics | timechart span=1h avg(cpu_usage) by host
source=events | eval hour = DATE_EXTRACT(timestamp, "hour") | chart count() by hour
```

---

### Phase 3: Joins & Advanced Features (12-14 weeks)

**Milestone**: Enterprise-grade analytics capabilities

**Components**:
1. **Join Operations** (6 weeks)
   - Inner join (basic)
   - Left join (outer)
   - Semi join (filter)
   - Anti join (exclusion)
   - Subsearch limits (10K default, 50K max)
   - Memory management
   - Performance optimization

2. **Subqueries** (4 weeks)
   - IN subquery
   - EXISTS subquery (correlated + uncorrelated)
   - Scalar subquery
   - Relation subquery (join right-side)

3. **Advanced Commands** (4 weeks)
   - lookup: External data enrichment
   - append: Result concatenation
   - flatten: Nested object flattening
   - spath: JSON navigation
   - fillnull: Null handling
   - dedup: Deduplication

**Deliverables**:
- 39 commands working (P0 + P1 complete)
- Join performance < 500ms for 10K rows
- 150+ integration tests
- Documentation: Join & Subquery Guide

**Example Working Queries**:
```
source=orders | join left=o right=c where o.customer_id = c.id [search source=customers]
source=logs | where severity IN [search source=alerts | fields severity]
source=events | lookup user_info.csv user_id AS id OUTPUT username, email
source=raw | parse json_field | spath path="user.address.city" | fillnull value="Unknown"
```

---

### Phase 4: Production Hardening (8-10 weeks)

**Milestone**: Production-ready with observability and optimization

**Components**:
1. **Performance Optimization** (4 weeks)
   - Query plan caching
   - Expression compilation
   - Result streaming (avoid buffering)
   - Memory pooling
   - Benchmarking suite

2. **Observability** (2 weeks)
   - explain command (query plan visualization)
   - Execution metrics (latency, memory)
   - Slow query logging
   - Query profiling

3. **Error Handling** (2 weeks)
   - User-friendly error messages
   - Syntax error recovery
   - Type mismatch detection
   - Resource limit enforcement

4. **Testing & Documentation** (2 weeks)
   - 500+ test cases
   - Performance benchmarks
   - User guide (100+ examples)
   - Migration guide (from OpenSearch)

**Deliverables**:
- 47 commands working (P0 + P1 + P2 complete)
- Query latency < 100ms (95th percentile)
- Memory usage < 500MB per query
- Complete PPL documentation
- Migration tooling

---

### Phase 5 (Optional): Advanced Features (8-10 weeks)

**Machine Learning Integration**:
- patterns: Automatic pattern detection
- ml: RCF anomaly detection, k-means clustering
- Integration with Python pipeline ML stages

**Advanced Operations**:
- transpose: Pivot transformations
- expand: Combination generation
- trendline: Statistical trend analysis
- multisearch: Parallel query execution

---

## 5. Resource Requirements

### Team Composition

**Core Team** (3-4 engineers):
1. **Parser Specialist** (1 FTE)
   - ANTLR4 grammar development
   - AST design and implementation
   - Error handling

2. **Backend Engineers** (2 FTE)
   - Query translation
   - Function library
   - Join implementation
   - Integration with Quidditch

3. **QA Engineer** (0.5-1 FTE)
   - Test plan development
   - Automated testing
   - Performance benchmarking
   - Documentation

**Skill Requirements**:
- Strong Go experience
- Parser/compiler background (ANTLR, YACC, etc.)
- Query optimization knowledge
- OpenSearch/Elasticsearch familiarity

### Timeline

| Phase | Duration | Engineers | Total Weeks |
|-------|----------|-----------|-------------|
| Phase 1: Foundation | 8-10 weeks | 3 | 24-30 |
| Phase 2: Aggregations | 10-12 weeks | 3 | 30-36 |
| Phase 3: Joins | 12-14 weeks | 3 | 36-42 |
| Phase 4: Hardening | 8-10 weeks | 3 | 24-30 |
| **Total** | **38-46 weeks** | **3** | **114-138 engineer-weeks** |

**Calendar Time**: 9-12 months with 3-person team

---

## 6. Technical Risks & Mitigations

### High Risk

**1. Grammar Complexity**
- **Risk**: 44 commands, 192 functions → large grammar
- **Mitigation**: Use proven OpenSearch grammar as reference, incremental development

**2. Join Performance**
- **Risk**: Memory exhaustion on large joins
- **Mitigation**: Row limits (10K default, 50K max), memory monitoring, streaming execution

**3. Function Library Scope**
- **Risk**: 192 functions is massive undertaking
- **Mitigation**: Prioritize by usage (P0: 135 functions), reuse existing evaluator

### Medium Risk

**4. Push-Down Translation**
- **Risk**: Not all PPL commands map to DSL
- **Mitigation**: Hybrid model (push-down + coordinator execution)

**5. Type System Complexity**
- **Risk**: 16 data types, implicit coercion rules
- **Mitigation**: Explicit type mappings, comprehensive tests

**6. Backwards Compatibility**
- **Risk**: Breaking changes as PPL evolves
- **Mitigation**: Versioned API, deprecation warnings

---

## 7. Success Metrics

### Functional Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Command Coverage | 90% (40/44) | Commands implemented |
| Function Coverage | 70% (135/192) | Functions implemented |
| OpenSearch Compatibility | 85% | Query compatibility tests |
| Test Coverage | >80% | Unit + integration tests |

### Performance Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Query Latency (p95) | <100ms | Simple queries |
| Query Latency (p95) | <500ms | Complex queries (joins) |
| Memory Usage | <500MB | Per query |
| Throughput | >1000 qps | Simple queries |
| Throughput | >100 qps | Complex queries |

### Quality Metrics

| Metric | Target | Measurement |
|--------|--------|-------------|
| Parser Error Rate | <1% | Valid queries failing |
| Translation Accuracy | >95% | Correct DSL generation |
| Documentation Coverage | 100% | All commands documented |
| Example Coverage | 3+ per command | Working examples |

---

## 8. Dependencies

### External Dependencies

1. **ANTLR4 Go Runtime**
   - Repository: github.com/antlr/antlr4/runtime/Go/antlr
   - Version: v4.13.0+
   - License: BSD-3

2. **OpenSearch SQL Grammar** (Reference)
   - Repository: github.com/opensearch-project/sql
   - Files: `OpenSearchPPLLexer.g4`, `OpenSearchPPLParser.g4`
   - License: Apache 2.0

### Internal Dependencies

1. **Quidditch Query Executor**
   - DSL query execution
   - Aggregation framework
   - Expression evaluator

2. **Diagon Core**
   - Index search
   - Relevance scoring
   - Field storage

3. **Pipeline Framework**
   - UDF execution
   - Python integration
   - Result transformation

---

## 9. Alternative Approaches

### Option A: Full Implementation (Current Plan)
- **Pros**: Complete feature set, OpenSearch compatible
- **Cons**: 9-12 months, resource intensive
- **Recommendation**: Phased approach with P0/P1 first

### Option B: SQL-to-DSL Translation Only
- **Pros**: Faster (3-4 months), simpler
- **Cons**: Limited PPL support, missing advanced features
- **Recommendation**: Not sufficient for analytics workloads

### Option C: Proxy to OpenSearch SQL Plugin
- **Pros**: Zero implementation effort
- **Cons**: Requires OpenSearch cluster, no control, latency overhead
- **Recommendation**: Only for MVP/prototype

### Option D: PPL Subset (Recommended)
- **Scope**: P0 commands only (15/44 = 34%)
- **Timeline**: 4-5 months with 2 engineers
- **Coverage**: 80% of real-world queries
- **Recommendation**: **Best balance** of effort and value

---

## 10. Recommended Approach

### Phase 0: Decision Point (2 weeks)

**Evaluate Options**:
1. Full PPL (9-12 months, 3 engineers)
2. PPL Subset (4-5 months, 2 engineers)
3. SQL-only (3-4 months, 1 engineer)

**Decision Criteria**:
- Customer demand (log analytics vs OLAP)
- Competitive requirements
- Resource availability
- Time to market

### Recommended: PPL Subset (Phase 1 + Phase 2)

**Scope**: 22 commands (50%), 135 functions (70%)

**Timeline**: 18-22 weeks with 2 engineers

**Coverage**:
- ✅ All basic queries (search, where, fields, sort, head)
- ✅ All aggregations (stats, chart, timechart, bin)
- ✅ Data transformations (eval, rename, parse, grok, rex)
- ✅ Core functions (math, string, date, aggregation)
- ❌ Joins (defer to Phase 3)
- ❌ ML (defer to Phase 5)

**Rationale**:
- Covers 80% of real-world log analytics queries
- Manageable scope and timeline
- Can add joins later based on demand
- Focuses on Quidditch strengths (fast aggregations)

---

## 11. Next Steps

### Immediate (Week 1-2)

1. **Stakeholder Alignment**
   - Review this plan with leadership
   - Decide: Full PPL vs Subset
   - Approve resource allocation

2. **Technical Preparation**
   - Clone OpenSearch SQL plugin grammar
   - Set up ANTLR4 Go development environment
   - Design AST node structures

3. **Team Formation**
   - Hire/assign parser specialist
   - Allocate backend engineers
   - Define QA responsibilities

### Short Term (Month 1)

4. **Phase 1 Kickoff**
   - Implement ANTLR4 grammar (lexer + parser)
   - Build AST visitor framework
   - Create first working query (source | where | fields)

5. **Infrastructure**
   - Set up parser test harness
   - Create benchmark suite
   - Build query examples library

---

## Appendix A: Command Reference Matrix

| Command | Category | Priority | Complexity | Est. Weeks | Dependencies |
|---------|----------|----------|------------|-----------|--------------|
| search | Retrieval | P0 | Low | 1 | None |
| where | Filtering | P0 | Low | 1 | Expression engine |
| fields | Selection | P0 | Low | 0.5 | None |
| stats | Aggregation | P0 | High | 3 | Aggregation framework |
| sort | Sorting | P0 | Low | 0.5 | None |
| head | Limiting | P0 | Low | 0.5 | None |
| top | Sorting | P0 | Low | 1 | Aggregation |
| rare | Sorting | P0 | Low | 1 | Aggregation |
| chart | Aggregation | P0 | High | 2 | Multi-agg |
| timechart | Aggregation | P0 | High | 2 | Time bucketing |
| bin | Bucketing | P0 | Medium | 1 | Histogram |
| explain | Diagnostic | P0 | Low | 1 | Query planner |
| describe | Schema | P0 | Low | 1 | Metadata |
| showdatasources | Schema | P0 | Low | 0.5 | Cluster state |
| dedup | Selection | P0 | Medium | 1 | Distinct |
| eval | Transformation | P1 | Medium | 2 | Expression engine |
| rename | Transformation | P1 | Low | 0.5 | None |
| replace | Transformation | P1 | Low | 1 | Pattern matching |
| parse | Transformation | P1 | High | 2 | Parser library |
| grok | Transformation | P1 | High | 3 | Grok patterns |
| rex | Transformation | P1 | Medium | 2 | Regex engine |
| spath | Transformation | P1 | Medium | 1 | JSON parser |
| fillnull | Transformation | P1 | Low | 1 | Null handling |
| join | Combination | P1 | Very High | 6 | Join executor |
| lookup | Combination | P1 | High | 2 | External data |
| append | Combination | P1 | Medium | 1 | Union |
| subquery | Combination | P1 | High | 4 | Nested execution |
| eventstats | Aggregation | P1 | High | 2 | Window functions |
| streamstats | Aggregation | P1 | High | 2 | Running aggregations |
| flatten | Transformation | P1 | Medium | 1 | Nested objects |
| table | Output | P1 | Low | 1 | Formatter |
| reverse | Sorting | P1 | Low | 0.5 | None |
| addtotals | Aggregation | P2 | Low | 1 | Aggregation |
| addcoltotals | Aggregation | P2 | Low | 1 | Aggregation |
| transpose | Transformation | P2 | High | 2 | Pivot |
| expand | Transformation | P2 | Medium | 1 | Cartesian product |
| appendcol | Combination | P2 | Medium | 2 | Column join |
| appendpipe | Combination | P2 | Medium | 2 | Pipeline chaining |
| trendline | Analysis | P2 | High | 2 | Statistical models |
| multisearch | Combination | P2 | Medium | 2 | Parallel execution |
| patterns | ML | P3 | Very High | 3 | Pattern detection |
| ml | ML | P3 | Very High | 5 | ML framework |
| kmeans | ML | P3 | High | 2 | Clustering |
| ad | ML | P3 | High | 2 | Anomaly detection |

**Totals**:
- P0: 15 commands (34%), 19.5 weeks
- P1: 17 commands (39%), 39 weeks
- P2: 8 commands (18%), 15 weeks
- P3: 4 commands (9%), 12 weeks

**Critical Path**: P0 + P1 = 32 commands (73%), 58.5 weeks

---

## Appendix B: Function Categories

### Date and Time (57 functions)

**Date Arithmetic**:
- ADDDATE, DATE_ADD, DATE_SUB, SUBDATE, DATEDIFF, TIMESTAMPDIFF
- ADDTIME, SUBTIME, TIMEDIFF

**Date Extraction**:
- EXTRACT, DATE_PART, DATE_FORMAT, TIME_FORMAT
- DAYOFWEEK, DAYOFMONTH, DAYOFYEAR, WEEKOFYEAR, WEEK
- DAY, MONTH, QUARTER, YEAR, HOUR, MINUTE, SECOND, MICROSECOND
- DAYNAME, MONTHNAME

**Date Construction**:
- MAKEDATE, MAKETIME, DATE, TIME, TIMESTAMP
- FROM_DAYS, FROM_UNIXTIME, TO_DAYS, TO_SECONDS, UNIX_TIMESTAMP

**Date Utilities**:
- CURDATE, CURTIME, NOW, SYSDATE, UTC_DATE, UTC_TIME, UTC_TIMESTAMP
- LAST_DAY, PERIOD_ADD, PERIOD_DIFF, QUARTER, WEEK

**Time Zones**:
- CONVERT_TZ

### Mathematical (41 functions)

**Basic Arithmetic**:
- ABS, SIGN, CEIL, CEILING, FLOOR, ROUND, TRUNCATE

**Exponential & Logarithmic**:
- EXP, LN, LOG, LOG2, LOG10, POW, POWER, SQRT

**Trigonometric**:
- SIN, COS, TAN, ASIN, ACOS, ATAN, ATAN2
- COT, DEGREES, RADIANS

**Rounding & Modulo**:
- MOD, RAND, RANDOM

**Bitwise**:
- BITWISE_AND, BITWISE_OR, BITWISE_XOR, BITWISE_NOT

**Special**:
- PI, E

### String (17 functions)

**Manipulation**:
- CONCAT, CONCAT_WS, SUBSTRING, SUBSTR, LEFT, RIGHT, TRIM, LTRIM, RTRIM
- UPPER, UCASE, LOWER, LCASE, REVERSE

**Pattern Matching**:
- LIKE, REGEXP, REPLACE, LOCATE, POSITION

**Information**:
- LENGTH, CHAR_LENGTH, CHARACTER_LENGTH

### Aggregation (20 functions)

**Basic Aggregates**:
- COUNT, SUM, AVG, MIN, MAX

**Statistical**:
- STDDEV, STDDEV_POP, STDDEV_SAMP, VAR_POP, VAR_SAMP, VARIANCE

**Advanced**:
- PERCENTILE, MEDIAN, MODE, PERCENTILE_APPROX

**Utilities**:
- DISTINCT, COLLECT_LIST, COLLECT_SET

### Conditional (14 functions)

**Branching**:
- IF, IFNULL, NULLIF, COALESCE, NVL

**Case Expression**:
- CASE, WHEN, THEN, ELSE, END

**Null Checks**:
- ISNULL, ISNOTNULL

### JSON (11 functions)

**Extraction**:
- JSON_EXTRACT, JSON_EXTRACT_SCALAR, GET_JSON_OBJECT

**Construction**:
- JSON_ARRAY, JSON_OBJECT, JSON_ARRAY_LENGTH

**Utilities**:
- JSON_KEYS, JSON_VALID, JSON_TYPE

### Relevance (7 functions)

**Search Functions**:
- MATCH, MATCH_PHRASE, MATCH_PHRASE_PREFIX, MULTI_MATCH
- QUERY_STRING, SIMPLE_QUERY_STRING, MATCH_BOOL_PREFIX

### Type Conversion (3 functions)

**Casting**:
- CAST, CONVERT, TRY_CAST

### Collection (15 functions)

**Array Operations**:
- ARRAY, ARRAY_CONTAINS, ARRAY_LENGTH, ARRAY_DISTINCT
- ARRAY_UNION, ARRAY_INTERSECT, ARRAY_EXCEPT
- ARRAY_JOIN, ARRAY_SORT, ARRAY_REVERSE

**Map Operations**:
- MAP, MAP_KEYS, MAP_VALUES, MAP_CONTAINS_KEY, MAP_SIZE

### IP Address (2 functions)

**IP Operations**:
- INET_ATON, INET_NTOA

### Cryptographic (2 functions)

**Hashing**:
- MD5, SHA1

### Statistical (2 functions)

**Statistics**:
- CORR, COVAR_POP

### System (1 function)

**System Info**:
- VERSION

---

## Appendix C: Grammar Complexity

### Token Count: 100+ tokens

**Categories**:
- Keywords: 50+ (SELECT, FROM, WHERE, GROUP, BY, etc.)
- Commands: 44 (search, stats, join, etc.)
- Operators: 20+ (+, -, *, /, =, !=, <, >, AND, OR, etc.)
- Literals: 10+ (string, number, boolean, null)
- Identifiers: Field names, aliases

### Parser Rules: 150+ rules

**Major Rules**:
- pplStatement: Top-level query
- commandClause: Individual command
- expression: Recursive expression tree
- functionCall: Function invocation
- fieldList: Field selection
- whereClause: Filter conditions
- statsClause: Aggregation specification
- joinClause: Join specification
- subquery: Nested query

**Complexity**:
- Left-recursive expressions (operator precedence)
- Ambiguous grammar (requires disambiguation)
- Error recovery (continue parsing after errors)

---

## Appendix D: Testing Strategy

### Test Pyramid

```
                    ┌──────────────┐
                    │  E2E Tests   │  50 tests (10%)
                    │  (Full PPL)  │
                    └──────────────┘
                   ┌────────────────┐
                   │ Integration    │  150 tests (30%)
                   │   (Commands)   │
                   └────────────────┘
                  ┌──────────────────┐
                  │  Component Tests │  200 tests (40%)
                  │  (Translation)   │
                  └──────────────────┘
                 ┌────────────────────┐
                 │    Unit Tests      │  100 tests (20%)
                 │ (Parser, Functions)│
                 └────────────────────┘
```

### Test Categories

**1. Grammar Tests** (100 tests)
- Valid syntax parsing
- Error recovery
- Ambiguous cases
- Edge cases

**2. Function Tests** (135 tests - one per P0 function)
- Correct computation
- Type coercion
- Null handling
- Edge cases

**3. Command Tests** (150 tests)
- Basic functionality
- Parameter variations
- Error handling
- Integration with functions

**4. Translation Tests** (50 tests)
- PPL → DSL correctness
- Optimization verification
- Push-down eligibility
- Query plan validation

**5. Performance Tests** (15 tests)
- Latency benchmarks
- Memory profiling
- Throughput testing
- Scalability tests

**6. Compatibility Tests** (50 tests)
- OpenSearch query parity
- Result format matching
- Error message consistency

---

## Appendix E: References

### OpenSearch SQL Plugin

**Repository**: https://github.com/opensearch-project/sql

**Key Files**:
- `/core/src/main/antlr4/OpenSearchPPLLexer.g4` - Lexer grammar
- `/core/src/main/antlr4/OpenSearchPPLParser.g4` - Parser grammar
- `/docs/user/ppl/` - Command documentation (47 files)
- `/docs/dev/` - Architecture documentation

**Version**: 2.x (OpenSearch 2.x compatible)

### Documentation

**Official Docs**: https://opensearch.org/docs/latest/search-plugins/sql/ppl/

**Command Reference**: https://opensearch.org/docs/latest/search-plugins/sql/ppl/commands/

**Function Reference**: https://opensearch.org/docs/latest/search-plugins/sql/ppl/functions/

### Related Projects

**Splunk SPL**: Inspiration for PPL design

**Apache Calcite**: SQL query optimizer (used by OpenSearch)

**ANTLR4**: Parser generator framework

---

**Document Version**: 1.0
**Last Updated**: January 27, 2026
**Author**: Quidditch Team
**Status**: Draft - Awaiting Review
