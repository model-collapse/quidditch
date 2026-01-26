package parser

import (
	"encoding/json"
	"fmt"

	"github.com/quidditch/quidditch/pkg/coordination/expressions"
)

// QueryParser parses OpenSearch Query DSL
type QueryParser struct{}

// NewQueryParser creates a new query parser
func NewQueryParser() *QueryParser {
	return &QueryParser{}
}

// ParseSearchRequest parses a complete search request
func (p *QueryParser) ParseSearchRequest(body []byte) (*SearchRequest, error) {
	var req SearchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, fmt.Errorf("failed to parse search request: %w", err)
	}

	// Parse the query if present
	if req.Query != nil {
		parsedQuery, err := p.ParseQuery(req.Query)
		if err != nil {
			return nil, fmt.Errorf("failed to parse query: %w", err)
		}
		req.ParsedQuery = parsedQuery
	}

	return &req, nil
}

// ParseQuery parses a query DSL object into an AST
func (p *QueryParser) ParseQuery(queryMap map[string]interface{}) (Query, error) {
	if len(queryMap) == 0 {
		return nil, fmt.Errorf("empty query")
	}

	// Query should have exactly one key (the query type)
	if len(queryMap) != 1 {
		return nil, fmt.Errorf("query must have exactly one type, got %d", len(queryMap))
	}

	for queryType, queryBody := range queryMap {
		switch queryType {
		case "match":
			return p.parseMatchQuery(queryBody)
		case "match_phrase":
			return p.parseMatchPhraseQuery(queryBody)
		case "multi_match":
			return p.parseMultiMatchQuery(queryBody)
		case "term":
			return p.parseTermQuery(queryBody)
		case "terms":
			return p.parseTermsQuery(queryBody)
		case "range":
			return p.parseRangeQuery(queryBody)
		case "bool":
			return p.parseBoolQuery(queryBody)
		case "match_all":
			return p.parseMatchAllQuery(queryBody)
		case "exists":
			return p.parseExistsQuery(queryBody)
		case "prefix":
			return p.parsePrefixQuery(queryBody)
		case "wildcard":
			return p.parseWildcardQuery(queryBody)
		case "fuzzy":
			return p.parseFuzzyQuery(queryBody)
		case "query_string":
			return p.parseQueryStringQuery(queryBody)
		case "expr":
			return p.parseExpressionQuery(queryBody)
		case "wasm_udf":
			return p.parseWasmUDFQuery(queryBody)
		default:
			return nil, fmt.Errorf("unsupported query type: %s", queryType)
		}
	}

	return nil, fmt.Errorf("failed to parse query")
}

// parseMatchQuery parses a match query
func (p *QueryParser) parseMatchQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("match query body must be an object")
	}

	// Match query has one field
	for field, value := range bodyMap {
		query := &MatchQuery{
			Field: field,
		}

		switch v := value.(type) {
		case string:
			query.Query = v
		case map[string]interface{}:
			// Extended match query with options
			if q, ok := v["query"].(string); ok {
				query.Query = q
			}
			if operator, ok := v["operator"].(string); ok {
				query.Operator = operator
			}
			if boost, ok := v["boost"].(float64); ok {
				query.Boost = boost
			}
			if analyzer, ok := v["analyzer"].(string); ok {
				query.Analyzer = analyzer
			}
		default:
			return nil, fmt.Errorf("invalid match query value type")
		}

		return query, nil
	}

	return nil, fmt.Errorf("match query must have a field")
}

// parseMatchPhraseQuery parses a match_phrase query
func (p *QueryParser) parseMatchPhraseQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("match_phrase query body must be an object")
	}

	for field, value := range bodyMap {
		query := &MatchPhraseQuery{
			Field: field,
		}

		switch v := value.(type) {
		case string:
			query.Query = v
		case map[string]interface{}:
			if q, ok := v["query"].(string); ok {
				query.Query = q
			}
			if slop, ok := v["slop"].(float64); ok {
				query.Slop = int(slop)
			}
		default:
			return nil, fmt.Errorf("invalid match_phrase query value type")
		}

		return query, nil
	}

	return nil, fmt.Errorf("match_phrase query must have a field")
}

// parseMultiMatchQuery parses a multi_match query
func (p *QueryParser) parseMultiMatchQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("multi_match query body must be an object")
	}

	query := &MultiMatchQuery{}

	if q, ok := bodyMap["query"].(string); ok {
		query.Query = q
	} else {
		return nil, fmt.Errorf("multi_match query must have a query string")
	}

	if fields, ok := bodyMap["fields"].([]interface{}); ok {
		query.Fields = make([]string, len(fields))
		for i, f := range fields {
			if field, ok := f.(string); ok {
				query.Fields[i] = field
			}
		}
	} else {
		return nil, fmt.Errorf("multi_match query must have fields")
	}

	if matchType, ok := bodyMap["type"].(string); ok {
		query.Type = matchType
	}

	return query, nil
}

// parseTermQuery parses a term query
func (p *QueryParser) parseTermQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("term query body must be an object")
	}

	for field, value := range bodyMap {
		query := &TermQuery{
			Field: field,
		}

		switch v := value.(type) {
		case string:
			query.Value = v
		case float64:
			query.Value = v
		case bool:
			query.Value = v
		case map[string]interface{}:
			if val, ok := v["value"]; ok {
				query.Value = val
			}
			if boost, ok := v["boost"].(float64); ok {
				query.Boost = boost
			}
		default:
			query.Value = v
		}

		return query, nil
	}

	return nil, fmt.Errorf("term query must have a field")
}

// parseTermsQuery parses a terms query
func (p *QueryParser) parseTermsQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("terms query body must be an object")
	}

	for field, value := range bodyMap {
		query := &TermsQuery{
			Field: field,
		}

		if values, ok := value.([]interface{}); ok {
			query.Values = values
		} else {
			return nil, fmt.Errorf("terms query values must be an array")
		}

		return query, nil
	}

	return nil, fmt.Errorf("terms query must have a field")
}

// parseRangeQuery parses a range query
func (p *QueryParser) parseRangeQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("range query body must be an object")
	}

	for field, value := range bodyMap {
		query := &RangeQuery{
			Field: field,
		}

		rangeMap, ok := value.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("range query value must be an object")
		}

		if gte, ok := rangeMap["gte"]; ok {
			query.Gte = gte
		}
		if gt, ok := rangeMap["gt"]; ok {
			query.Gt = gt
		}
		if lte, ok := rangeMap["lte"]; ok {
			query.Lte = lte
		}
		if lt, ok := rangeMap["lt"]; ok {
			query.Lt = lt
		}
		if boost, ok := rangeMap["boost"].(float64); ok {
			query.Boost = boost
		}

		return query, nil
	}

	return nil, fmt.Errorf("range query must have a field")
}

// parseBoolQuery parses a bool query
func (p *QueryParser) parseBoolQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("bool query body must be an object")
	}

	query := &BoolQuery{}

	// Parse must clauses
	if must, ok := bodyMap["must"]; ok {
		queries, err := p.parseQueryArray(must)
		if err != nil {
			return nil, fmt.Errorf("failed to parse must clauses: %w", err)
		}
		query.Must = queries
	}

	// Parse should clauses
	if should, ok := bodyMap["should"]; ok {
		queries, err := p.parseQueryArray(should)
		if err != nil {
			return nil, fmt.Errorf("failed to parse should clauses: %w", err)
		}
		query.Should = queries
	}

	// Parse must_not clauses
	if mustNot, ok := bodyMap["must_not"]; ok {
		queries, err := p.parseQueryArray(mustNot)
		if err != nil {
			return nil, fmt.Errorf("failed to parse must_not clauses: %w", err)
		}
		query.MustNot = queries
	}

	// Parse filter clauses
	if filter, ok := bodyMap["filter"]; ok {
		queries, err := p.parseQueryArray(filter)
		if err != nil {
			return nil, fmt.Errorf("failed to parse filter clauses: %w", err)
		}
		query.Filter = queries
	}

	// Parse minimum_should_match
	if minMatch, ok := bodyMap["minimum_should_match"]; ok {
		switch v := minMatch.(type) {
		case float64:
			query.MinimumShouldMatch = int(v)
		case string:
			query.MinimumShouldMatchStr = v
		}
	}

	return query, nil
}

// parseQueryArray parses an array of queries or a single query
func (p *QueryParser) parseQueryArray(value interface{}) ([]Query, error) {
	switch v := value.(type) {
	case []interface{}:
		queries := make([]Query, 0, len(v))
		for _, item := range v {
			queryMap, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("query must be an object")
			}
			query, err := p.ParseQuery(queryMap)
			if err != nil {
				return nil, err
			}
			queries = append(queries, query)
		}
		return queries, nil
	case map[string]interface{}:
		// Single query
		query, err := p.ParseQuery(v)
		if err != nil {
			return nil, err
		}
		return []Query{query}, nil
	default:
		return nil, fmt.Errorf("query must be an object or array")
	}
}

// parseMatchAllQuery parses a match_all query
func (p *QueryParser) parseMatchAllQuery(body interface{}) (Query, error) {
	query := &MatchAllQuery{}

	if bodyMap, ok := body.(map[string]interface{}); ok {
		if boost, ok := bodyMap["boost"].(float64); ok {
			query.Boost = boost
		}
	}

	return query, nil
}

// parseExistsQuery parses an exists query
func (p *QueryParser) parseExistsQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("exists query body must be an object")
	}

	query := &ExistsQuery{}

	if field, ok := bodyMap["field"].(string); ok {
		query.Field = field
	} else {
		return nil, fmt.Errorf("exists query must have a field")
	}

	return query, nil
}

// parsePrefixQuery parses a prefix query
func (p *QueryParser) parsePrefixQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("prefix query body must be an object")
	}

	for field, value := range bodyMap {
		query := &PrefixQuery{
			Field: field,
		}

		switch v := value.(type) {
		case string:
			query.Value = v
		case map[string]interface{}:
			if val, ok := v["value"].(string); ok {
				query.Value = val
			}
		default:
			return nil, fmt.Errorf("invalid prefix query value type")
		}

		return query, nil
	}

	return nil, fmt.Errorf("prefix query must have a field")
}

// parseWildcardQuery parses a wildcard query
func (p *QueryParser) parseWildcardQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("wildcard query body must be an object")
	}

	for field, value := range bodyMap {
		query := &WildcardQuery{
			Field: field,
		}

		switch v := value.(type) {
		case string:
			query.Value = v
		case map[string]interface{}:
			if val, ok := v["value"].(string); ok {
				query.Value = val
			}
		default:
			return nil, fmt.Errorf("invalid wildcard query value type")
		}

		return query, nil
	}

	return nil, fmt.Errorf("wildcard query must have a field")
}

// parseFuzzyQuery parses a fuzzy query
func (p *QueryParser) parseFuzzyQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("fuzzy query body must be an object")
	}

	for field, value := range bodyMap {
		query := &FuzzyQuery{
			Field: field,
		}

		switch v := value.(type) {
		case string:
			query.Value = v
		case map[string]interface{}:
			if val, ok := v["value"].(string); ok {
				query.Value = val
			}
			if fuzziness, ok := v["fuzziness"].(string); ok {
				query.Fuzziness = fuzziness
			}
		default:
			return nil, fmt.Errorf("invalid fuzzy query value type")
		}

		return query, nil
	}

	return nil, fmt.Errorf("fuzzy query must have a field")
}

// parseQueryStringQuery parses a query_string query
func (p *QueryParser) parseQueryStringQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("query_string query body must be an object")
	}

	query := &QueryStringQuery{}

	if q, ok := bodyMap["query"].(string); ok {
		query.Query = q
	} else {
		return nil, fmt.Errorf("query_string query must have a query string")
	}

	if defaultField, ok := bodyMap["default_field"].(string); ok {
		query.DefaultField = defaultField
	}

	if fields, ok := bodyMap["fields"].([]interface{}); ok {
		query.Fields = make([]string, len(fields))
		for i, f := range fields {
			if field, ok := f.(string); ok {
				query.Fields[i] = field
			}
		}
	}

	return query, nil
}

// parseExpressionQuery parses an expression query
func (p *QueryParser) parseExpressionQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expr query body must be an object")
	}

	// Create expression parser, validator, and serializer
	exprParser := expressions.NewParser()
	validator := expressions.NewValidator()
	serializer := expressions.NewSerializer()

	// Parse expression AST
	expr, err := exprParser.Parse(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expression: %w", err)
	}

	// Validate expression
	if err := validator.Validate(expr); err != nil {
		return nil, fmt.Errorf("invalid expression: %w", err)
	}

	// Serialize for C++ evaluation
	data, err := serializer.Serialize(expr)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize expression: %w", err)
	}

	return &ExpressionQuery{
		Expression:           expr,
		SerializedExpression: data,
	}, nil
}

// parseWasmUDFQuery parses a WASM UDF query
func (p *QueryParser) parseWasmUDFQuery(body interface{}) (Query, error) {
	bodyMap, ok := body.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("wasm_udf query body must be an object")
	}

	query := &WasmUDFQuery{
		Parameters: make(map[string]interface{}),
	}

	// Extract UDF name (required)
	if name, ok := bodyMap["name"].(string); ok {
		query.Name = name
	} else {
		return nil, fmt.Errorf("wasm_udf query must have a 'name' field")
	}

	// Extract version (optional)
	if version, ok := bodyMap["version"].(string); ok {
		query.Version = version
	}

	// Extract parameters (optional)
	if params, ok := bodyMap["parameters"].(map[string]interface{}); ok {
		query.Parameters = params
	} else if params, ok := bodyMap["params"].(map[string]interface{}); ok {
		// Support "params" as alias
		query.Parameters = params
	}

	return query, nil
}

// Validate validates the parsed query
func (p *QueryParser) Validate(query Query) error {
	if query == nil {
		return fmt.Errorf("query is nil")
	}

	// Recursively validate based on query type
	switch q := query.(type) {
	case *MatchQuery:
		if q.Field == "" {
			return fmt.Errorf("match query field is empty")
		}
		if q.Query == "" {
			return fmt.Errorf("match query text is empty")
		}
	case *TermQuery:
		if q.Field == "" {
			return fmt.Errorf("term query field is empty")
		}
		if q.Value == nil {
			return fmt.Errorf("term query value is nil")
		}
	case *BoolQuery:
		if len(q.Must) == 0 && len(q.Should) == 0 && len(q.MustNot) == 0 && len(q.Filter) == 0 {
			return fmt.Errorf("bool query has no clauses")
		}
		// Validate nested queries
		for _, subQuery := range q.Must {
			if err := p.Validate(subQuery); err != nil {
				return err
			}
		}
		for _, subQuery := range q.Should {
			if err := p.Validate(subQuery); err != nil {
				return err
			}
		}
		for _, subQuery := range q.MustNot {
			if err := p.Validate(subQuery); err != nil {
				return err
			}
		}
		for _, subQuery := range q.Filter {
			if err := p.Validate(subQuery); err != nil {
				return err
			}
		}
	case *RangeQuery:
		if q.Field == "" {
			return fmt.Errorf("range query field is empty")
		}
		if q.Gt == nil && q.Gte == nil && q.Lt == nil && q.Lte == nil {
			return fmt.Errorf("range query has no range conditions")
		}
	case *ExpressionQuery:
		if q.Expression == nil {
			return fmt.Errorf("expression query has no expression")
		}
		if len(q.SerializedExpression) == 0 {
			return fmt.Errorf("expression query has no serialized expression")
		}
	case *WasmUDFQuery:
		if q.Name == "" {
			return fmt.Errorf("wasm_udf query has no name")
		}
	}

	return nil
}
