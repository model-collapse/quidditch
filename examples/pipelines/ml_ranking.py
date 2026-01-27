"""
ML Ranking Pipeline Stage

This stage re-ranks search results using a machine learning model.
Combines multiple signals: BM25 score, click-through rate, recency, etc.

Usage:
    POST /api/v1/pipelines/ml_ranker
    Body: {
        "name": "ml_ranker",
        "version": "1.0.0",
        "type": "result",
        "description": "Re-ranks results using ML model",
        "stages": [{
            "name": "apply_ml_ranking",
            "type": "python",
            "config": {
                "udf_name": "ml_ranking",
                "udf_version": "1.0.0",
                "model": "learning_to_rank_v1"
            }
        }]
    }
"""

import math


# Simulated ML model weights (in production, load from trained model)
# Features: [bm25_score, ctr, recency_days, has_image, price_range]
MODEL_WEIGHTS = [0.4, 0.3, 0.15, 0.10, 0.05]
MODEL_BIAS = 0.5


def normalize_score(value, min_val, max_val):
    """
    Normalize value to [0, 1] range.

    Args:
        value: Value to normalize
        min_val: Minimum possible value
        max_val: Maximum possible value

    Returns:
        Normalized value in [0, 1]
    """
    if max_val == min_val:
        return 0.5

    return (value - min_val) / (max_val - min_val)


def extract_features(hit, query_context):
    """
    Extract features from a search hit for ML model.

    Args:
        hit: Search result hit
            {
                "score": 1.5,
                "_id": "doc123",
                "_source": {...}
            }
        query_context: Query metadata (e.g., user preferences)

    Returns:
        Feature vector (list of floats)
    """
    # Feature 1: BM25 score (already normalized by search engine)
    bm25_score = hit.get("score", 0.0)

    # Feature 2: Click-through rate (from analytics)
    # In production, look up from analytics database
    doc_id = hit.get("_id", "")
    ctr = hash(doc_id) % 100 / 100.0  # Simulated CTR [0, 1]

    # Feature 3: Recency (days since publication)
    source = hit.get("_source", {})
    published_timestamp = source.get("published_at", 0)
    now_timestamp = 1706400000  # Example: 2024-01-28 in epoch seconds
    days_old = (now_timestamp - published_timestamp) / 86400.0
    # Normalize: recent = 1.0, old = 0.0
    recency_score = 1.0 / (1.0 + math.log(max(days_old, 1)))

    # Feature 4: Has image (binary)
    has_image = 1.0 if source.get("image_url") else 0.0

    # Feature 5: Price range (if applicable)
    price = source.get("price", 0.0)
    # Normalize price to preference (cheaper = better in this example)
    price_score = 1.0 / (1.0 + math.log(max(price, 1)))

    return [bm25_score, ctr, recency_score, has_image, price_score]


def predict_score(features):
    """
    Predict relevance score using ML model.

    Args:
        features: Feature vector

    Returns:
        Predicted relevance score
    """
    # Simple linear model: score = w1*f1 + w2*f2 + ... + bias
    score = MODEL_BIAS

    for weight, feature in zip(MODEL_WEIGHTS, features):
        score += weight * feature

    return score


def udf_main(result):
    """
    Main UDF entry point.

    Receives search results and re-ranks them using ML model.

    Args:
        result: Dictionary containing search results
            {
                "total": 100,
                "max_score": 2.5,
                "hits": [
                    {
                        "score": 2.5,
                        "_id": "doc1",
                        "_source": {...}
                    },
                    ...
                ]
            }

    Returns:
        Re-ranked results with updated scores
    """
    hits = result.get("hits", [])

    if not hits:
        return result

    # Extract query context (could come from request metadata)
    query_context = {}

    # Compute ML scores for each hit
    scored_hits = []
    for hit in hits:
        # Extract features
        features = extract_features(hit, query_context)

        # Predict relevance score
        ml_score = predict_score(features)

        # Store original BM25 score for reference
        hit["_original_score"] = hit.get("score", 0.0)

        # Update hit with ML score
        hit["score"] = ml_score

        # Add feature explanations (optional, for debugging)
        hit["_ml_features"] = {
            "bm25": features[0],
            "ctr": features[1],
            "recency": features[2],
            "has_image": features[3],
            "price_score": features[4]
        }

        scored_hits.append((ml_score, hit))

    # Sort by ML score (descending)
    scored_hits.sort(key=lambda x: x[0], reverse=True)

    # Extract sorted hits
    result["hits"] = [hit for score, hit in scored_hits]

    # Update max_score
    if scored_hits:
        result["max_score"] = scored_hits[0][0]

    # Add metadata about re-ranking
    if "_meta" not in result:
        result["_meta"] = {}
    result["_meta"]["ml_ranking"] = {
        "model": "learning_to_rank_v1",
        "features": MODEL_WEIGHTS,
        "reranked_count": len(hits)
    }

    return result


# Example usage:
# Input: {
#     "total": 2,
#     "max_score": 2.5,
#     "hits": [
#         {"score": 2.5, "_id": "doc1", "_source": {"title": "Laptop", "price": 999}},
#         {"score": 2.0, "_id": "doc2", "_source": {"title": "Notebook", "price": 599}}
#     ]
# }
# Output: {
#     "total": 2,
#     "max_score": 1.95,
#     "hits": [
#         {"score": 1.95, "_original_score": 2.0, "_id": "doc2", ...},
#         {"score": 1.87, "_original_score": 2.5, "_id": "doc1", ...}
#     ],
#     "_meta": {"ml_ranking": {...}}
# }
