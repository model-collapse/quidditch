package executor

import (
	"math"
	"sort"
	"time"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
	"go.uber.org/zap"
)

// aggregateSearchResults merges search results from multiple shards
func (qe *QueryExecutor) aggregateSearchResults(responses []*pb.SearchResponse, from, size int) *SearchResult {
	if len(responses) == 0 {
		return &SearchResult{
			TotalHits: 0,
			MaxScore:  0,
			Hits:      []*SearchHit{},
		}
	}

	// Collect all hits from all shards
	var allHits []*SearchHit
	var totalHits int64
	var maxScore float64

	for _, resp := range responses {
		// Sum total hits
		if resp.Hits != nil && resp.Hits.Total != nil {
			totalHits += resp.Hits.Total.Value
		}

		// Track max score
		if resp.Hits != nil && resp.Hits.MaxScore > maxScore {
			maxScore = resp.Hits.MaxScore
		}

		// Collect hits
		if resp.Hits != nil {
			for _, hit := range resp.Hits.Hits {
				allHits = append(allHits, &SearchHit{
					ID:     hit.Id,
					Score:  hit.Score,
					Source: hit.Source.AsMap(),
				})
			}
		}
	}

	// Sort hits by score (descending)
	sort.Slice(allHits, func(i, j int) bool {
		return allHits[i].Score > allHits[j].Score
	})

	// Apply pagination (from/size)
	start := from
	if start > len(allHits) {
		start = len(allHits)
	}

	end := start + size
	if end > len(allHits) {
		end = len(allHits)
	}

	paginatedHits := allHits[start:end]

	// Merge aggregations from all shards
	aggregations := qe.mergeAggregations(responses)

	return &SearchResult{
		TotalHits:    totalHits,
		MaxScore:     maxScore,
		Hits:         paginatedHits,
		Aggregations: aggregations,
	}
}

// aggregateCountResults sums document counts from multiple shards
func aggregateCountResults(responses []*pb.CountResponse) int64 {
	var total int64
	for _, resp := range responses {
		total += resp.Count
	}
	return total
}

// mergeAggregations merges aggregations from multiple shard responses
func (qe *QueryExecutor) mergeAggregations(responses []*pb.SearchResponse) map[string]*AggregationResult {
	if len(responses) == 0 {
		return nil
	}

	// Group aggregations by name across all shards
	aggsByName := make(map[string][]*pb.AggregationResult)
	for _, resp := range responses {
		if resp.Aggregations == nil {
			continue
		}
		for name, agg := range resp.Aggregations {
			aggsByName[name] = append(aggsByName[name], agg)
		}
	}

	if len(aggsByName) == 0 {
		return nil
	}

	// Merge each aggregation type
	merged := make(map[string]*AggregationResult)
	for name, aggs := range aggsByName {
		if len(aggs) == 0 {
			continue
		}

		aggType := aggs[0].Type // Assume all shards return same type

		// Track aggregation merge time
		mergeStartTime := time.Now()
		var result *AggregationResult

		switch aggType {
		case "terms", "histogram", "date_histogram":
			result = qe.mergeBucketAggregation(aggs)
		case "stats":
			result = qe.mergeStatsAggregation(aggs, false)
		case "extended_stats":
			result = qe.mergeStatsAggregation(aggs, true)
		case "percentiles":
			result = qe.mergePercentilesAggregation(aggs)
		case "cardinality":
			result = qe.mergeCardinalityAggregation(aggs)
		default:
			qe.logger.Warn("Unknown aggregation type, skipping merge",
				zap.String("type", aggType),
				zap.String("name", name))
		}

		if result != nil {
			merged[name] = result
			// Record merge time
			aggregationMergeTime.WithLabelValues(aggType).Observe(time.Since(mergeStartTime).Seconds())
		}
	}

	return merged
}

// mergeBucketAggregation merges bucket-based aggregations (terms, histogram, date_histogram)
func (qe *QueryExecutor) mergeBucketAggregation(aggs []*pb.AggregationResult) *AggregationResult {
	if len(aggs) == 0 {
		return nil
	}

	// Sum bucket counts across all shards
	bucketCounts := make(map[string]int64)      // for string keys (terms, date_histogram)
	numericBucketCounts := make(map[float64]int64) // for numeric keys (histogram)

	aggType := aggs[0].Type
	isNumeric := aggType == "histogram"

	for _, agg := range aggs {
		for _, bucket := range agg.Buckets {
			if isNumeric {
				numericBucketCounts[bucket.NumericKey] += bucket.DocCount
			} else {
				bucketCounts[bucket.Key] += bucket.DocCount
			}
		}
	}

	// Convert to result buckets
	var buckets []*AggregationBucket

	if isNumeric {
		// Numeric buckets (histogram)
		for key, count := range numericBucketCounts {
			buckets = append(buckets, &AggregationBucket{
				NumericKey: key,
				DocCount:   count,
			})
		}
		// Sort by numeric key
		sort.Slice(buckets, func(i, j int) bool {
			return buckets[i].NumericKey < buckets[j].NumericKey
		})
	} else {
		// String buckets (terms, date_histogram)
		for key, count := range bucketCounts {
			buckets = append(buckets, &AggregationBucket{
				Key:      key,
				DocCount: count,
			})
		}
		// Sort by doc_count descending
		sort.Slice(buckets, func(i, j int) bool {
			return buckets[i].DocCount > buckets[j].DocCount
		})
	}

	return &AggregationResult{
		Type:    aggType,
		Buckets: buckets,
	}
}

// mergeStatsAggregation merges stats and extended_stats aggregations
func (qe *QueryExecutor) mergeStatsAggregation(aggs []*pb.AggregationResult, extended bool) *AggregationResult {
	if len(aggs) == 0 {
		return nil
	}

	result := &AggregationResult{
		Type: aggs[0].Type,
		Min:  aggs[0].Min,
		Max:  aggs[0].Max,
	}

	var totalCount int64
	var totalSum float64
	var totalSumOfSquares float64

	for _, agg := range aggs {
		totalCount += agg.Count
		totalSum += agg.Sum

		// Track global min/max
		if agg.Min < result.Min {
			result.Min = agg.Min
		}
		if agg.Max > result.Max {
			result.Max = agg.Max
		}

		if extended {
			totalSumOfSquares += agg.SumOfSquares
		}
	}

	result.Count = totalCount
	result.Sum = totalSum
	if totalCount > 0 {
		result.Avg = totalSum / float64(totalCount)
	}

	if extended {
		result.SumOfSquares = totalSumOfSquares
		// Calculate variance: Var(X) = E[X²] - E[X]²
		if totalCount > 0 {
			result.Variance = (totalSumOfSquares / float64(totalCount)) - (result.Avg * result.Avg)
			if result.Variance > 0 {
				result.StdDeviation = math.Sqrt(result.Variance)
				result.StdDeviationBoundsUpper = result.Avg + 2.0*result.StdDeviation
				result.StdDeviationBoundsLower = result.Avg - 2.0*result.StdDeviation
			}
		}
	}

	return result
}

// mergePercentilesAggregation merges percentiles aggregations
// Note: This is approximate - collecting all values would be expensive
// For now, we average the percentile values from each shard
func (qe *QueryExecutor) mergePercentilesAggregation(aggs []*pb.AggregationResult) *AggregationResult {
	if len(aggs) == 0 {
		return nil
	}

	result := &AggregationResult{
		Type:   "percentiles",
		Values: make(map[string]float64),
	}

	// Track percentile sums and counts for averaging
	percentileSums := make(map[string]float64)
	percentileCounts := make(map[string]int)

	for _, agg := range aggs {
		for percentile, value := range agg.Values {
			percentileSums[percentile] += value
			percentileCounts[percentile]++
		}
	}

	// Average the percentiles
	for percentile, sum := range percentileSums {
		count := percentileCounts[percentile]
		if count > 0 {
			result.Values[percentile] = sum / float64(count)
		}
	}

	return result
}

// mergeCardinalityAggregation merges cardinality aggregations
// Note: This is approximate - true cardinality would require HyperLogLog
// For now, we sum the cardinalities (which may overcount)
func (qe *QueryExecutor) mergeCardinalityAggregation(aggs []*pb.AggregationResult) *AggregationResult {
	if len(aggs) == 0 {
		return nil
	}

	result := &AggregationResult{
		Type: "cardinality",
	}

	// Sum cardinalities from all shards
	// Note: This overcounts if same values appear on multiple shards
	var total int64
	for _, agg := range aggs {
		total += agg.Value
	}

	result.Value = total

	return result
}
