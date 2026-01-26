package executor

import (
	"sort"

	pb "github.com/quidditch/quidditch/pkg/common/proto"
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

	return &SearchResult{
		TotalHits: totalHits,
		MaxScore:  maxScore,
		Hits:      paginatedHits,
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
