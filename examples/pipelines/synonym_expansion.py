"""
Synonym Expansion Pipeline Stage

This stage expands query terms with synonyms to improve recall.
For example, "laptop" becomes "laptop OR notebook OR computer"

Usage:
    POST /api/v1/pipelines/synonym_expander
    Body: {
        "name": "synonym_expander",
        "version": "1.0.0",
        "type": "query",
        "description": "Expands query terms with synonyms",
        "stages": [{
            "name": "expand_synonyms",
            "type": "python",
            "config": {
                "udf_name": "synonym_expansion",
                "udf_version": "1.0.0"
            }
        }]
    }
"""

# Synonym dictionary - in production, load from database or file
SYNONYMS = {
    "laptop": ["notebook", "computer"],
    "phone": ["mobile", "smartphone", "cellphone"],
    "car": ["vehicle", "automobile", "auto"],
    "buy": ["purchase", "acquire", "get"],
    "cheap": ["inexpensive", "affordable", "budget"],
    "fast": ["quick", "rapid", "speedy"],
    "good": ["great", "excellent", "quality"],
}


def expand_term(term):
    """
    Expand a single term with its synonyms.

    Args:
        term: Search term to expand

    Returns:
        List of original term + synonyms
    """
    term_lower = term.lower()

    # Check if term has synonyms
    if term_lower in SYNONYMS:
        # Return original + synonyms
        return [term] + SYNONYMS[term_lower]

    return [term]


def udf_main(request):
    """
    Main UDF entry point.

    Receives a search request and expands query terms with synonyms.

    Args:
        request: Dictionary containing search request
            {
                "query": {...},
                "size": 10,
                "from": 0,
                ...
            }

    Returns:
        Modified request with expanded query
    """
    # Extract query from request
    query = request.get("query", {})

    # Handle match query
    if "match" in query:
        for field, match_query in query["match"].items():
            if isinstance(match_query, str):
                # Simple string query
                original_text = match_query
            elif isinstance(match_query, dict):
                # Complex match query
                original_text = match_query.get("query", "")
            else:
                continue

            # Expand terms
            terms = original_text.split()
            expanded_terms = []

            for term in terms:
                synonyms = expand_term(term)
                if len(synonyms) > 1:
                    # Create OR clause: (term1 OR term2 OR term3)
                    expanded_terms.append("(" + " OR ".join(synonyms) + ")")
                else:
                    expanded_terms.append(term)

            # Rebuild query
            expanded_query = " ".join(expanded_terms)

            # Update request
            if isinstance(query["match"][field], str):
                query["match"][field] = expanded_query
            elif isinstance(query["match"][field], dict):
                query["match"][field]["query"] = expanded_query

    # Handle query_string query
    if "query_string" in query:
        query_text = query["query_string"].get("query", "")
        terms = query_text.split()
        expanded_terms = []

        for term in terms:
            # Skip operators
            if term.upper() in ["AND", "OR", "NOT"]:
                expanded_terms.append(term)
                continue

            synonyms = expand_term(term)
            if len(synonyms) > 1:
                expanded_terms.append("(" + " OR ".join(synonyms) + ")")
            else:
                expanded_terms.append(term)

        query["query_string"]["query"] = " ".join(expanded_terms)

    # Return modified request
    return request


# Example usage:
# Input: {"query": {"match": {"title": "laptop"}}}
# Output: {"query": {"match": {"title": "(laptop OR notebook OR computer)"}}}
