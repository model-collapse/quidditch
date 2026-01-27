# PPL Architecture Design: Query Processing Pipeline

## Overview

PPL (Piped Processing Language) query processing follows a multi-stage pipeline that transforms user queries into executable operations. This document details each stage with examples and design decisions.

```
User Query → Tokens → AST → Logical Plan → Physical Plan → Execution → Results
              ↑        ↑         ↑              ↑              ↑
           Lexer   Parser   Analyzer      Optimizer     Executor
```

---

## Stage 1: Lexical Analysis (Tokenization)

### Purpose
Convert raw query text into a stream of tokens that the parser can understand.

### Implementation
**ANTLR4 Lexer** (`PPLLexer.g4`)

### Token Categories

```antlr4
// Keywords
SEARCH      : 'search' ;
WHERE       : 'where' ;
STATS       : 'stats' ;
BY          : 'by' ;
AS          : 'as' ;

// Operators
EQUAL       : '=' ;
NOT_EQUAL   : '!=' | '<>' ;
GREATER     : '>' ;
LESS        : '<' ;
PIPE        : '|' ;
COMMA       : ',' ;

// Literals
INTEGER     : [0-9]+ ;
FLOAT       : [0-9]+ '.' [0-9]+ ;
STRING      : '"' (~["\r\n])* '"' | '\'' (~['\r\n])* '\'' ;
BOOLEAN     : 'true' | 'false' ;

// Identifiers
FIELD_NAME  : [a-zA-Z_] [a-zA-Z0-9_.]* ;
```

### Example: Tokenization

**Input Query**:
```sql
source=logs | where status=500 | stats count() by host
```

**Token Stream**:
```
SEARCH, EQUAL, IDENTIFIER(logs), PIPE,
WHERE, IDENTIFIER(status), EQUAL, INTEGER(500), PIPE,
STATS, IDENTIFIER(count), LPAREN, RPAREN, BY, IDENTIFIER(host)
```

---

## Stage 2: Syntactic Analysis (Parsing)

### Purpose
Build an Abstract Syntax Tree (AST) that represents the hierarchical structure of the query.

### Implementation
**ANTLR4 Parser** (`PPLParser.g4`)

### Grammar Rules

```antlr4
// Top-level rule
pplStatement
    : searchCommand (PIPE pipeCommand)* EOF
    ;

// Search command (always first)
searchCommand
    : SEARCH SOURCE EQUAL sourceName
    ;

// Pipe commands
pipeCommand
    : whereCommand
    | fieldsCommand
    | statsCommand
    | sortCommand
    | headCommand
    | evalCommand
    | joinCommand
    // ... more commands
    ;

// Where clause
whereCommand
    : WHERE expression
    ;

// Stats clause
statsCommand
    : STATS aggregationList (BY groupByList)?
    ;

// Expression (recursive)
expression
    : expression AND expression           # logicalAnd
    | expression OR expression            # logicalOr
    | expression comparisonOp expression  # comparison
    | LPAREN expression RPAREN            # parenthesized
    | functionCall                        # function
    | field                               # fieldRef
    | literal                             # literalValue
    ;
```

### AST Node Types

```go
// Base AST node
type Node interface {
    Accept(visitor Visitor) interface{}
    Type() NodeType
    Position() Position
}

// Command nodes
type SearchCommand struct {
    Source string
    Pos    Position
}

type WhereCommand struct {
    Condition Expression
    Pos       Position
}

type StatsCommand struct {
    Aggregations []Aggregation
    GroupBy      []FieldRef
    Pos          Position
}

// Expression nodes
type BinaryExpression struct {
    Left     Expression
    Operator string
    Right    Expression
    Pos      Position
}

type FunctionCall struct {
    Name      string
    Arguments []Expression
    Pos       Position
}

type FieldReference struct {
    Name string
    Pos  Position
}

type Literal struct {
    Value interface{}
    Type  DataType
    Pos   Position
}
```

### Example: AST Construction

**Query**:
```sql
source=logs | where status=500 AND host='server1' | stats count() by region
```

**AST** (simplified tree):
```
PPLQuery
├── SearchCommand
│   └── Source: "logs"
├── WhereCommand
│   └── BinaryExpression (AND)
│       ├── BinaryExpression (=)
│       │   ├── FieldReference: "status"
│       │   └── Literal: 500 (int)
│       └── BinaryExpression (=)
│           ├── FieldReference: "host"
│           └── Literal: "server1" (string)
└── StatsCommand
    ├── Aggregations
    │   └── FunctionCall: "count()"
    └── GroupBy
        └── FieldReference: "region"
```

---

## Stage 3: Semantic Analysis

### Purpose
Validate the AST for semantic correctness and enrich with type information.

### Analyzer Responsibilities

1. **Type Checking**
   - Verify field existence in schema
   - Validate function argument types
   - Infer expression result types

2. **Name Resolution**
   - Resolve field aliases
   - Handle nested field references (e.g., `user.address.city`)
   - Validate aggregation function names

3. **Semantic Validation**
   - Ensure GROUP BY fields exist in aggregation
   - Check WHERE clause has boolean type
   - Validate HAVING clause (must come after GROUP BY)

### Example: Type Inference

**Query**:
```sql
source=products | where price > 100 | eval discount = price * 0.1
```

**Type Analysis**:
```
Field 'price': float64 (from schema)
Literal 100: int → coerce to float64
Comparison (price > 100): boolean ✓
Expression (price * 0.1): float64 * float64 → float64 ✓
New field 'discount': float64
```

### Semantic Errors

```sql
-- Error: Field 'unknownField' not found
source=logs | where unknownField=500

-- Error: Cannot compare string with number
source=logs | where status > 'ERROR'

-- Error: COUNT() requires 0 or 1 arguments
source=logs | stats count(field1, field2)

-- Error: GROUP BY field must be selected or in aggregation
source=logs | stats count() by region | fields count
```

---

## Stage 4: Logical Plan Construction

### Purpose
Transform the validated AST into a logical execution plan - a tree of logical operators.

### Logical Operators

```go
// Base logical operator
type LogicalOperator interface {
    Schema() Schema
    Children() []LogicalOperator
    Accept(visitor LogicalVisitor) LogicalOperator
}

// Logical operators (database-style)
type LogicalScan struct {
    Source string
    Schema Schema
}

type LogicalFilter struct {
    Condition Expression
    Input     LogicalOperator
}

type LogicalProject struct {
    Expressions []NamedExpression
    Input       LogicalOperator
}

type LogicalAggregate struct {
    GroupBy      []Expression
    Aggregations []AggregateExpression
    Input        LogicalOperator
}

type LogicalSort struct {
    SortKeys []SortKey
    Input    LogicalOperator
}

type LogicalLimit struct {
    Count int
    Input LogicalOperator
}

type LogicalJoin struct {
    JoinType  JoinType
    Left      LogicalOperator
    Right     LogicalOperator
    Condition Expression
}
```

### AST → Logical Plan Transformation

**Query**:
```sql
source=logs | where status=500 | stats count() as total by host | sort total DESC | head 10
```

**Logical Plan** (bottom-up tree):
```
LogicalLimit(10)
  └── LogicalSort(total DESC)
      └── LogicalAggregate(
            groupBy=[host],
            aggs=[count() as total]
          )
          └── LogicalFilter(status = 500)
              └── LogicalScan(logs)
```

**Tree Representation**:
```
        Limit(10)
            |
        Sort(total DESC)
            |
     Aggregate(group=[host], aggs=[count() as total])
            |
     Filter(status = 500)
            |
       Scan(logs)
```

### Logical Plan Properties

Each operator has:
- **Schema**: Output fields and types
- **Statistics**: Estimated row count, data size
- **Cost**: Estimated computation cost

```go
type LogicalPlanNode struct {
    Operator   LogicalOperator
    Schema     Schema
    Statistics Statistics
    Cost       Cost
}

type Schema struct {
    Fields []FieldSchema
}

type FieldSchema struct {
    Name string
    Type DataType
}

type Statistics struct {
    RowCount    int64
    DataSizeBytes int64
}

type Cost struct {
    CPUCost float64
    IOCost  float64
}
```

---

## Stage 5: Query Optimization

### Purpose
Transform the logical plan into an optimized equivalent using rule-based and cost-based techniques.

### Optimization Rules

#### Rule 1: Predicate Push-Down
Push filters down to the scan level to reduce data early.

**Before**:
```
Aggregate(count())
  └── Filter(status = 500)
      └── Scan(logs)
```

**After**:
```
Aggregate(count())
  └── Scan(logs, filter: status = 500)  // Push filter to scan
```

**Why**: Reduce data flowing through pipeline, leverage index filtering.

#### Rule 2: Projection Push-Down
Only read required fields from storage.

**Before**:
```
Project(host, status)
  └── Filter(status = 500)
      └── Scan(logs)  // Reads all fields
```

**After**:
```
Project(host, status)
  └── Filter(status = 500)
      └── Scan(logs, columns: [host, status, timestamp])  // Only needed fields
```

#### Rule 3: Aggregation Push-Down
Push aggregations to OpenSearch when possible.

**Before**:
```
Aggregate(count(), sum(bytes) by host)
  └── Scan(logs)
```

**After** (if pushable):
```
ScanWithAggregation(
  index: logs,
  aggs: {
    "host_groups": {
      "terms": {"field": "host"},
      "aggs": {
        "count": {"value_count": {"field": "*"}},
        "total_bytes": {"sum": {"field": "bytes"}}
      }
    }
  }
)
```

#### Rule 4: Constant Folding
Evaluate constant expressions at compile time.

**Before**:
```
Filter(timestamp > DATE_SUB(NOW(), INTERVAL 1 DAY))
```

**After**:
```
Filter(timestamp > 1706227200000)  // Precomputed
```

#### Rule 5: Join Reordering
Reorder joins based on cardinality estimates.

**Before**:
```
Join(large_table, small_table)  // Wrong order
```

**After**:
```
Join(small_table, large_table)  // Better: build hash table on smaller side
```

### Optimization Example

**Original Query**:
```sql
source=logs | where timestamp > NOW() - 3600 | where status >= 400 | stats count() by host
```

**Logical Plan** (before optimization):
```
Aggregate(count() by host)
  └── Filter(status >= 400)
      └── Filter(timestamp > NOW() - 3600)
          └── Scan(logs)
```

**Optimized Logical Plan**:
```
Aggregate(count() by host)
  └── Scan(logs,
        filter: timestamp > 1706227200 AND status >= 400,  // Combined filters
        columns: [host, status, timestamp]                  // Only needed columns
      )
```

**Optimizations Applied**:
1. Constant folding: `NOW() - 3600` → `1706227200`
2. Filter merge: Combined two filters into one
3. Predicate push-down: Pushed filter to scan
4. Column pruning: Only read necessary columns

---

## Stage 6: Physical Plan Generation

### Purpose
Convert the optimized logical plan into a concrete execution plan with specific algorithms and data sources.

### Physical Operators

```go
// Physical operators
type PhysicalOperator interface {
    Execute(ctx Context) Iterator
    Schema() Schema
    Cost() Cost
}

// Scan implementations
type OpenSearchScan struct {
    Index      string
    Query      *OpenSearchQuery  // DSL query
    Columns    []string
    Aggregations *Aggregations
}

type LocalScan struct {
    Data []Row  // In-memory data (for testing)
}

// Execution implementations
type HashAggregate struct {
    GroupBy      []Expression
    Aggregations []AggregateFunc
    Input        PhysicalOperator
    HashTable    *HashMap  // Execution state
}

type StreamAggregate struct {
    // Requires sorted input
    GroupBy      []Expression
    Aggregations []AggregateFunc
    Input        PhysicalOperator
}

type HashJoin struct {
    Left       PhysicalOperator
    Right      PhysicalOperator
    JoinKeys   []Expression
    BuildSide  Side  // LEFT or RIGHT
    HashTable  *HashMap
}

type NestedLoopJoin struct {
    // Fallback for non-equi joins
    Left      PhysicalOperator
    Right     PhysicalOperator
    Condition Expression
}
```

### Physical Plan Selection

**Logical Plan**:
```
Aggregate(count() by host)
  └── Scan(logs, filter: status = 500)
```

**Physical Plan Options**:

#### Option A: Full Push-Down (Best)
```
OpenSearchScan(
  index: "logs",
  query: {"term": {"status": 500}},
  aggs: {
    "host_groups": {
      "terms": {"field": "host"},
      "aggs": {"count": {"value_count": {"field": "*"}}}
    }
  }
)
```
**Cost**: Low IO, low CPU, high efficiency

#### Option B: Partial Push-Down
```
HashAggregate(group=[host], aggs=[count()])
  └── OpenSearchScan(
        index: "logs",
        query: {"term": {"status": 500}},
        columns: ["host"]
      )
```
**Cost**: Medium IO, medium CPU

#### Option C: No Push-Down (Worst)
```
HashAggregate(group=[host], aggs=[count()])
  └── Filter(status = 500)
      └── OpenSearchScan(
            index: "logs",
            columns: ["host", "status"]
          )
```
**Cost**: High IO, high CPU

**Selection**: Choose Option A (full push-down) when possible.

---

## Stage 7: Push-Down Strategy

### Push-Down Decision Tree

```
Can push to OpenSearch?
│
├─ YES → Generate DSL
│   │
│   ├─ Pure scan/filter? → Use query DSL only
│   ├─ Has aggregation? → Use aggs DSL
│   ├─ Has sorting? → Use sort DSL
│   └─ Has limit? → Use size/from DSL
│
└─ NO → Execute on coordinator
    │
    ├─ Needs transformation (eval, parse)? → Go evaluation
    ├─ Needs join? → Hash join or nested loop
    └─ Needs complex logic? → Custom operator
```

### Push-Down Mapping: PPL → OpenSearch DSL

#### Example 1: Simple Filter

**PPL**:
```sql
source=logs | where status=500 AND host='server1'
```

**OpenSearch DSL**:
```json
{
  "query": {
    "bool": {
      "must": [
        {"term": {"status": 500}},
        {"term": {"host": "server1"}}
      ]
    }
  }
}
```

#### Example 2: Aggregation

**PPL**:
```sql
source=logs | stats count() as total, avg(latency) by region, status
```

**OpenSearch DSL**:
```json
{
  "size": 0,
  "aggs": {
    "region_groups": {
      "terms": {"field": "region"},
      "aggs": {
        "status_groups": {
          "terms": {"field": "status"},
          "aggs": {
            "total": {"value_count": {"field": "*"}},
            "avg_latency": {"avg": {"field": "latency"}}
          }
        }
      }
    }
  }
}
```

#### Example 3: Time-Series

**PPL**:
```sql
source=metrics | timechart span=1h avg(cpu_usage) by host
```

**OpenSearch DSL**:
```json
{
  "size": 0,
  "aggs": {
    "time_buckets": {
      "date_histogram": {
        "field": "timestamp",
        "fixed_interval": "1h"
      },
      "aggs": {
        "host_groups": {
          "terms": {"field": "host"},
          "aggs": {
            "avg_cpu": {"avg": {"field": "cpu_usage"}}
          }
        }
      }
    }
  }
}
```

### Push-Down Compatibility Matrix

| PPL Command | Pushable? | OpenSearch DSL | Notes |
|-------------|-----------|----------------|-------|
| `search` | ✅ Yes | `query` | Full push-down |
| `where` | ✅ Yes | `query.bool` | Boolean combinations |
| `fields` | ✅ Yes | `_source` | Column pruning |
| `stats` | ✅ Yes | `aggs` | Most aggregations |
| `timechart` | ✅ Yes | `date_histogram` | Time bucketing |
| `sort` | ✅ Yes | `sort` | Sort order |
| `head` | ✅ Yes | `size` | Result limit |
| `top` | ✅ Yes | `terms.order` | Top N values |
| `rare` | ⚠️ Partial | Custom script | Need post-processing |
| `eval` | ❌ No | - | Coordinator execution |
| `parse` | ❌ No | - | Coordinator execution |
| `grok` | ❌ No | - | Coordinator execution |
| `join` | ❌ No | - | Coordinator execution |
| `lookup` | ❌ No | - | Coordinator execution |

### Push-Down Code Example

```go
// Push-down translator
type PushDownTranslator struct {
    schema *IndexSchema
}

func (t *PushDownTranslator) Translate(plan PhysicalPlan) (*OpenSearchQuery, error) {
    switch node := plan.(type) {
    case *PhysicalScan:
        return t.translateScan(node)
    case *PhysicalFilter:
        return t.translateFilter(node)
    case *PhysicalAggregate:
        return t.translateAggregate(node)
    default:
        return nil, ErrNotPushable
    }
}

func (t *PushDownTranslator) translateFilter(filter *PhysicalFilter) (*OpenSearchQuery, error) {
    query := &OpenSearchQuery{}

    // Translate condition to DSL
    switch expr := filter.Condition.(type) {
    case *BinaryExpression:
        switch expr.Operator {
        case "=":
            query.Query = map[string]interface{}{
                "term": map[string]interface{}{
                    expr.Left.(*FieldRef).Name: expr.Right.(*Literal).Value,
                },
            }
        case "AND":
            left, _ := t.translateExpression(expr.Left)
            right, _ := t.translateExpression(expr.Right)
            query.Query = map[string]interface{}{
                "bool": map[string]interface{}{
                    "must": []interface{}{left, right},
                },
            }
        // ... more operators
        }
    }

    return query, nil
}

func (t *PushDownTranslator) translateAggregate(agg *PhysicalAggregate) (*OpenSearchQuery, error) {
    query := &OpenSearchQuery{Size: 0}  // No documents, only aggregations

    // Build aggregation DSL
    aggs := make(map[string]interface{})

    // Group by fields → terms aggregation
    if len(agg.GroupBy) > 0 {
        for i, groupField := range agg.GroupBy {
            fieldName := groupField.(*FieldRef).Name
            aggName := fmt.Sprintf("group_%d", i)

            aggs[aggName] = map[string]interface{}{
                "terms": map[string]interface{}{
                    "field": fieldName,
                    "size":  10000,  // Max buckets
                },
            }

            // Nested aggregations for metrics
            if i == len(agg.GroupBy)-1 {
                metricAggs := t.translateMetrics(agg.Aggregations)
                aggs[aggName].(map[string]interface{})["aggs"] = metricAggs
            }
        }
    }

    query.Aggs = aggs
    return query, nil
}
```

---

## Stage 8: Execution

### Execution Models

#### Model 1: Full Push-Down (Data Node Execution)

```
┌─────────────────────────────────────┐
│   Coordinator Node                  │
│                                     │
│   1. Translate PPL → DSL           │
│   2. Send DSL to data nodes        │
│   3. Receive aggregated results    │
│   4. Format and return             │
└─────────────────────────────────────┘
                 │
                 │ HTTP (DSL)
                 ▼
┌─────────────────────────────────────┐
│   Data Node (OpenSearch)            │
│                                     │
│   1. Parse DSL query               │
│   2. Execute on Diagon index       │
│   3. Return results                │
└─────────────────────────────────────┘
```

**Query**:
```sql
source=logs | where status=500 | stats count() by host
```

**Execution Flow**:
1. Coordinator translates to DSL
2. Sends to data nodes: `POST /logs/_search` with aggregation DSL
3. Data nodes execute aggregation on Diagon
4. Returns aggregated buckets
5. Coordinator formats as PPL result

**Latency**: Low (single network hop)

#### Model 2: Partial Push-Down (Hybrid Execution)

```
┌─────────────────────────────────────┐
│   Coordinator Node                  │
│                                     │
│   1. Translate scan/filter → DSL   │
│   2. Fetch data from data nodes    │
│   3. Execute eval in Go            │
│   4. Execute aggregation in Go     │
│   5. Return results                │
└─────────────────────────────────────┘
                 │
                 │ HTTP (DSL with scroll)
                 ▼
┌─────────────────────────────────────┐
│   Data Node (OpenSearch)            │
│                                     │
│   1. Parse DSL query               │
│   2. Filter on Diagon              │
│   3. Stream matching docs          │
└─────────────────────────────────────┘
```

**Query**:
```sql
source=logs | where status=500 | eval latency_ms = latency * 1000 | stats avg(latency_ms) by host
```

**Execution Flow**:
1. Push filter to data nodes: `status=500`
2. Stream matching documents to coordinator
3. Coordinator evaluates `latency_ms = latency * 1000` in Go
4. Coordinator aggregates `avg(latency_ms)` in memory
5. Returns results

**Latency**: Medium (data streaming overhead)

#### Model 3: No Push-Down (Coordinator Execution)

```
┌─────────────────────────────────────┐
│   Coordinator Node                  │
│                                     │
│   1. Fetch all data (scan)         │
│   2. Execute parse/grok in Go      │
│   3. Execute join in Go            │
│   4. Execute aggregation in Go     │
│   5. Return results                │
└─────────────────────────────────────┘
                 │
                 │ HTTP (match_all query)
                 ▼
┌─────────────────────────────────────┐
│   Data Node (OpenSearch)            │
│                                     │
│   1. Return all documents          │
└─────────────────────────────────────┘
```

**Query**:
```sql
source=logs | parse message "%{TIMESTAMP} %{LOGLEVEL:level} %{GREEDYDATA:msg}" | where level='ERROR'
```

**Execution Flow**:
1. Fetch all documents from data nodes
2. Coordinator applies grok pattern parsing in Go
3. Coordinator filters `level='ERROR'` in memory
4. Returns results

**Latency**: High (full data transfer)

### Iterator-Based Execution

Physical operators produce iterators for streaming execution:

```go
// Iterator interface
type Iterator interface {
    Next() (Row, error)
    Close() error
}

// Row represents a result row
type Row struct {
    Fields map[string]interface{}
}

// Example: Hash aggregate iterator
type HashAggregateIterator struct {
    input      Iterator
    groupKeys  []string
    aggFuncs   []AggregateFunc
    hashTable  map[string]*AggregateState
    resultChan chan Row
    done       bool
}

func (it *HashAggregateIterator) Next() (Row, error) {
    if !it.done {
        // Phase 1: Build hash table
        for {
            row, err := it.input.Next()
            if err == io.EOF {
                break
            }
            if err != nil {
                return Row{}, err
            }

            // Extract group key
            key := it.extractGroupKey(row)

            // Get or create aggregate state
            state, exists := it.hashTable[key]
            if !exists {
                state = it.createAggregateState()
                it.hashTable[key] = state
            }

            // Update aggregates
            it.updateAggregates(state, row)
        }

        // Phase 2: Emit results
        it.emitResults()
        it.done = true
    }

    // Return next result row
    select {
    case row, ok := <-it.resultChan:
        if !ok {
            return Row{}, io.EOF
        }
        return row, nil
    }
}
```

### Execution Example

**Query**:
```sql
source=logs | where status >= 500 | stats count() as errors, avg(latency) by host | sort errors DESC | head 5
```

**Physical Plan**:
```
Limit(5)
  └── Sort(errors DESC)
      └── HashAggregate(
            group=[host],
            aggs=[count() as errors, avg(latency)]
          )
          └── OpenSearchScan(
                index: logs,
                filter: status >= 500,
                columns: [host, latency, status]
              )
```

**Execution Steps**:

1. **OpenSearchScan Iterator**:
   ```go
   // Translates to OpenSearch DSL
   POST /logs/_search
   {
     "query": {"range": {"status": {"gte": 500}}},
     "_source": ["host", "latency"],
     "size": 10000,
     "scroll": "1m"
   }

   // Streams documents
   for doc := range scrollResults {
       yield Row{
           "host": doc.Host,
           "latency": doc.Latency,
       }
   }
   ```

2. **HashAggregate Iterator**:
   ```go
   hashTable := make(map[string]*AggState)

   for row := range scanIterator {
       key := row["host"].(string)

       if _, exists := hashTable[key]; !exists {
           hashTable[key] = &AggState{
               count: 0,
               sum: 0.0,
           }
       }

       hashTable[key].count++
       hashTable[key].sum += row["latency"].(float64)
   }

   // Emit results
   for host, state := range hashTable {
       yield Row{
           "host": host,
           "errors": state.count,
           "avg(latency)": state.sum / float64(state.count),
       }
   }
   ```

3. **Sort Iterator**:
   ```go
   rows := []Row{}
   for row := range aggregateIterator {
       rows = append(rows, row)
   }

   sort.Slice(rows, func(i, j int) bool {
       return rows[i]["errors"].(int) > rows[j]["errors"].(int)
   })

   for _, row := range rows {
       yield row
   }
   ```

4. **Limit Iterator**:
   ```go
   count := 0
   for row := range sortIterator {
       if count >= 5 {
           break
       }
       yield row
       count++
   }
   ```

---

## Stage 9: Result Formatting

### Purpose
Convert internal row format to user-facing JSON response.

### Output Format

**PPL Result**:
```json
{
  "took": 45,
  "timed_out": false,
  "hits": {
    "total": {
      "value": 1234,
      "relation": "eq"
    }
  },
  "aggregations": {
    "host": [
      {"key": "server1", "errors": 156, "avg(latency)": 234.5},
      {"key": "server2", "errors": 89, "avg(latency)": 189.2},
      {"key": "server3", "errors": 45, "avg(latency)": 301.8}
    ]
  }
}
```

**Alternative: Tabular Format**:
```
host      | errors | avg(latency)
----------|--------|-------------
server1   | 156    | 234.5
server2   | 89     | 189.2
server3   | 45     | 301.8
```

---

## Complete Example: End-to-End

### Query
```sql
source=access_logs
| where status >= 400 AND timestamp > NOW() - 3600
| eval error_type = IF(status >= 500, 'server_error', 'client_error')
| stats count() as total, avg(response_time) as avg_time by error_type, path
| where total > 10
| sort total DESC
| head 20
```

### Stage 1: Tokenization
```
[SEARCH, SOURCE, EQUAL, IDENTIFIER(access_logs), PIPE,
 WHERE, IDENTIFIER(status), GREATER_EQUAL, INTEGER(400), AND, ...]
```

### Stage 2: AST
```
PPLQuery
├── SearchCommand(source=access_logs)
├── WhereCommand(status >= 400 AND timestamp > NOW() - 3600)
├── EvalCommand(error_type = IF(status >= 500, 'server_error', 'client_error'))
├── StatsCommand(
│     aggs=[count() as total, avg(response_time) as avg_time],
│     groupBy=[error_type, path]
│   )
├── WhereCommand(total > 10)  // HAVING clause
├── SortCommand(total DESC)
└── HeadCommand(20)
```

### Stage 3: Semantic Analysis
- ✓ Field 'status' exists (type: int)
- ✓ Field 'timestamp' exists (type: timestamp)
- ✓ Field 'response_time' exists (type: float)
- ✓ Field 'path' exists (type: string)
- ✓ Expression type: IF(boolean, string, string) → string
- ✓ HAVING clause after GROUP BY

### Stage 4: Logical Plan
```
LogicalLimit(20)
  └── LogicalSort(total DESC)
      └── LogicalFilter(total > 10)
          └── LogicalAggregate(
                groupBy=[error_type, path],
                aggs=[count() as total, avg(response_time) as avg_time]
              )
              └── LogicalProject(
                    fields=[status, timestamp, response_time, path,
                            IF(status >= 500, 'server_error', 'client_error') as error_type]
                  )
                  └── LogicalFilter(status >= 400 AND timestamp > NOW() - 3600)
                      └── LogicalScan(access_logs)
```

### Stage 5: Optimization
```
LogicalLimit(20)
  └── LogicalSort(total DESC)
      └── LogicalFilter(total > 10)  // HAVING - can't push down
          └── LogicalAggregate(
                groupBy=[error_type, path],
                aggs=[count() as total, avg(response_time) as avg_time]
              )
              └── LogicalProject(
                    fields=[status, response_time, path,
                            IF(status >= 500, 'server_error', 'client_error') as error_type]
                  )
                  └── LogicalScan(
                        source=access_logs,
                        filter=status >= 400 AND timestamp > 1706223600,  // Pushed + folded
                        columns=[status, timestamp, response_time, path]   // Pruned
                      )
```

### Stage 6: Physical Plan
```
Limit(20)
  └── Sort(total DESC)  // In-memory sort
      └── Filter(total > 10)  // Coordinator filter (HAVING)
          └── HashAggregate(  // Coordinator aggregation (eval involved)
                group=[error_type, path],
                aggs=[count(), avg(response_time)]
              )
              └── Project(  // Coordinator evaluation
                    error_type = IF(status >= 500, 'server_error', 'client_error')
                  )
                  └── OpenSearchScan(  // Push-down
                        index: access_logs,
                        query: {
                          "bool": {
                            "must": [
                              {"range": {"status": {"gte": 400}}},
                              {"range": {"timestamp": {"gt": 1706223600}}}
                            ]
                          }
                        },
                        _source: [status, timestamp, response_time, path],
                        size: 10000,
                        scroll: "1m"
                      )
```

### Stage 7: Push-Down Decision

**Pushable**:
- ✓ Filter: `status >= 400 AND timestamp > 1706223600`
- ✓ Column pruning: `[status, timestamp, response_time, path]`

**Not Pushable** (coordinator execution):
- ✗ eval: `IF(status >= 500, ...)` - requires Go evaluation
- ✗ Aggregation: Depends on eval field `error_type`
- ✗ HAVING: `total > 10` - post-aggregation filter
- ✗ Sort: In-memory sort

**OpenSearch DSL**:
```json
POST /access_logs/_search?scroll=1m
{
  "query": {
    "bool": {
      "must": [
        {"range": {"status": {"gte": 400}}},
        {"range": {"timestamp": {"gt": 1706223600}}}
      ]
    }
  },
  "_source": ["status", "timestamp", "response_time", "path"],
  "size": 10000
}
```

### Stage 8: Execution

```
┌─────────────────────────────────────────────────────┐
│ Coordinator Node                                    │
│                                                     │
│ 1. Scan: Fetch filtered docs from OpenSearch      │
│    (status>=400, timestamp>1706223600)             │
│                                                     │
│ 2. Eval: Compute error_type in Go                 │
│    IF(status >= 500, 'server_error', 'client_error')│
│                                                     │
│ 3. Aggregate: Group by (error_type, path) in Go   │
│    count(), avg(response_time)                     │
│                                                     │
│ 4. Filter: Apply HAVING total > 10                │
│                                                     │
│ 5. Sort: Order by total DESC                      │
│                                                     │
│ 6. Limit: Take top 20                             │
└─────────────────────────────────────────────────────┘
                        │
                        ▼
┌─────────────────────────────────────────────────────┐
│ Data Node                                           │
│                                                     │
│ Execute: status>=400 AND timestamp>1706223600      │
│ Return: Stream of matching documents               │
└─────────────────────────────────────────────────────┘
```

### Stage 9: Results

```json
{
  "took": 234,
  "timed_out": false,
  "hits": {
    "total": {"value": 4521, "relation": "eq"}
  },
  "aggregations": {
    "groups": [
      {
        "error_type": "server_error",
        "path": "/api/users",
        "total": 156,
        "avg_time": 1234.5
      },
      {
        "error_type": "client_error",
        "path": "/checkout",
        "total": 89,
        "avg_time": 234.2
      },
      // ... 18 more rows
    ]
  }
}
```

---

## Performance Considerations

### Push-Down Benefits

| Metric | Full Push-Down | Partial Push-Down | No Push-Down |
|--------|----------------|-------------------|--------------|
| **Network Transfer** | Minimal (only aggregated results) | Medium (filtered docs) | High (all docs) |
| **Coordinator CPU** | Low | Medium | High |
| **Coordinator Memory** | Low | Medium | High |
| **Latency** | <100ms | 100-500ms | >500ms |
| **Scalability** | Excellent | Good | Poor |

### Memory Management

```go
// Aggregation memory limits
type AggregateMemoryTracker struct {
    maxMemoryBytes int64
    currentMemory  int64
    groupCount     int
}

func (t *AggregateMemoryTracker) CheckMemory() error {
    if t.currentMemory > t.maxMemoryBytes {
        return ErrMemoryLimitExceeded
    }
    return nil
}

// Configuration
const (
    MaxAggregateMemory = 500 * 1024 * 1024  // 500MB
    MaxGroupCount      = 100000              // 100K groups
)
```

### Query Timeouts

```go
type ExecutionContext struct {
    ctx        context.Context
    timeout    time.Duration
    cancelFunc context.CancelFunc
}

func (e *Executor) Execute(plan PhysicalPlan) (Iterator, error) {
    ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
    defer cancel()

    execCtx := &ExecutionContext{
        ctx:        ctx,
        timeout:    e.timeout,
        cancelFunc: cancel,
    }

    return plan.Execute(execCtx)
}
```

---

## Error Handling

### Error Types

```go
// Parse errors
type ParseError struct {
    Message  string
    Position Position
}

// Semantic errors
type SemanticError struct {
    Message string
    Node    Node
}

// Execution errors
type ExecutionError struct {
    Message   string
    Operator  string
    Cause     error
}

// Timeout errors
type TimeoutError struct {
    Duration time.Duration
}

// Memory errors
type MemoryLimitError struct {
    Limit   int64
    Current int64
}
```

### Error Recovery

```go
// Graceful degradation
func (e *Executor) ExecuteWithFallback(plan PhysicalPlan) (Iterator, error) {
    // Try push-down execution
    it, err := e.executePushDown(plan)
    if err == nil {
        return it, nil
    }

    // Log degradation
    log.Warn("Push-down failed, falling back to coordinator execution",
        "error", err,
        "query", plan.String())

    // Fallback to coordinator execution
    return e.executeCoordinator(plan)
}
```

---

## Testing Strategy

### Unit Tests

```go
// Lexer tests
func TestLexer(t *testing.T) {
    tests := []struct {
        input  string
        tokens []Token
    }{
        {
            input: "source=logs | where status=500",
            tokens: []Token{
                {Type: SEARCH, Value: "source"},
                {Type: EQUAL, Value: "="},
                // ...
            },
        },
    }
}

// Parser tests
func TestParser(t *testing.T) {
    tests := []struct {
        input string
        ast   Node
        err   error
    }{
        {
            input: "source=logs | where status=500",
            ast: &PPLQuery{
                Commands: []Command{
                    &SearchCommand{Source: "logs"},
                    &WhereCommand{/* ... */},
                },
            },
        },
    }
}

// Optimizer tests
func TestOptimizer(t *testing.T) {
    tests := []struct {
        input    LogicalPlan
        expected LogicalPlan
    }{
        {
            name: "predicate_pushdown",
            input: /* unoptimized plan */,
            expected: /* optimized plan */,
        },
    }
}
```

### Integration Tests

```go
func TestEndToEnd(t *testing.T) {
    // Setup test index
    index := setupTestIndex(t)
    defer index.Cleanup()

    // Index test data
    index.IndexDocuments(testData)

    // Execute PPL query
    query := "source=test | where value > 100 | stats count() by category"
    results, err := executor.Execute(query)
    require.NoError(t, err)

    // Verify results
    assert.Equal(t, 3, len(results.Rows))
    assert.Equal(t, 150, results.Rows[0]["count"])
}
```

---

## Observability

### Metrics

```go
// Query metrics
type QueryMetrics struct {
    ParseTimeMs      float64
    PlanTimeMs       float64
    ExecutionTimeMs  float64
    RowsScanned      int64
    RowsReturned     int64
    BytesProcessed   int64
    PushDownRatio    float64
}

// Prometheus metrics
var (
    queryDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "ppl_query_duration_seconds",
            Help: "PPL query execution duration",
        },
        []string{"stage"},
    )

    pushDownRate = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "ppl_pushdown_rate",
            Help: "Percentage of queries using push-down",
        },
    )
)
```

### Query Profiling

```go
// Explain output
type ExplainResult struct {
    Query          string
    LogicalPlan    string
    PhysicalPlan   string
    PushDownInfo   PushDownInfo
    EstimatedCost  Cost
}

type PushDownInfo struct {
    Pushable     []string  // Commands pushed to OpenSearch
    NotPushable  []string  // Commands executed on coordinator
    Reason       map[string]string
}
```

---

## Summary

### Key Design Principles

1. **Separation of Concerns**: Each stage has clear responsibility
2. **Composability**: Operators compose into complex plans
3. **Optimization**: Rule-based and cost-based optimization
4. **Push-Down First**: Maximize work on data nodes
5. **Graceful Degradation**: Fallback to coordinator when needed
6. **Streaming Execution**: Iterator-based for memory efficiency
7. **Observability**: Metrics and profiling built-in

### Architecture Benefits

- ✅ **Performance**: Push-down minimizes data transfer
- ✅ **Scalability**: Parallel execution on data nodes
- ✅ **Flexibility**: Hybrid execution model
- ✅ **Maintainability**: Clear separation of stages
- ✅ **Extensibility**: Easy to add new operators
- ✅ **Debuggability**: Explain plans and metrics

### Next Steps

1. Implement ANTLR4 grammar (Tier 0)
2. Build AST visitor and logical plan builder
3. Create physical plan generator
4. Implement push-down translator
5. Build iterator-based execution engine
6. Add optimization rules
7. Comprehensive testing

---

**Document Version**: 1.0
**Last Updated**: January 27, 2026
**Author**: Quidditch Team
**Status**: Design Specification
