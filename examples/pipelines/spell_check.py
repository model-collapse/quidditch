"""
Spell Check Pipeline Stage

This stage corrects common spelling mistakes in search queries.
For example, "labtop" becomes "laptop"

Usage:
    POST /api/v1/pipelines/spell_checker
    Body: {
        "name": "spell_checker",
        "version": "1.0.0",
        "type": "query",
        "description": "Corrects spelling mistakes in queries",
        "stages": [{
            "name": "correct_spelling",
            "type": "python",
            "config": {
                "udf_name": "spell_check",
                "udf_version": "1.0.0",
                "auto_correct": true,
                "suggest": true
            }
        }]
    }
"""

# Common spelling mistakes - in production, use a real spell checker
# or Levenshtein distance against dictionary
SPELLING_CORRECTIONS = {
    "labtop": "laptop",
    "notbook": "notebook",
    "computor": "computer",
    "phoen": "phone",
    "mobil": "mobile",
    "smatphone": "smartphone",
    "vehical": "vehicle",
    "automobil": "automobile",
    "cheep": "cheap",
    "expensiv": "expensive",
    "afforable": "affordable",
    "qick": "quick",
    "rapd": "rapid",
    "excelent": "excellent",
    "qualit": "quality",
}


def correct_word(word):
    """
    Correct a single word if it's misspelled.

    Args:
        word: Word to check and correct

    Returns:
        Tuple of (corrected_word, was_corrected)
    """
    word_lower = word.lower()

    if word_lower in SPELLING_CORRECTIONS:
        return SPELLING_CORRECTIONS[word_lower], True

    return word, False


def correct_text(text):
    """
    Correct spelling in a text string.

    Args:
        text: Text to correct

    Returns:
        Tuple of (corrected_text, corrections_list)
            corrections_list: List of {"original": "...", "corrected": "..."}
    """
    words = text.split()
    corrected_words = []
    corrections = []

    for word in words:
        corrected, was_corrected = correct_word(word)
        corrected_words.append(corrected)

        if was_corrected:
            corrections.append({
                "original": word,
                "corrected": corrected
            })

    return " ".join(corrected_words), corrections


def udf_main(request):
    """
    Main UDF entry point.

    Receives a search request and corrects spelling mistakes.

    Args:
        request: Dictionary containing search request
            {
                "query": {...},
                "size": 10,
                "from": 0,
                ...
            }

    Returns:
        Modified request with corrected spelling
    """
    # Track all corrections for logging
    all_corrections = []

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

            # Correct spelling
            corrected_text, corrections = correct_text(original_text)

            if corrections:
                all_corrections.extend(corrections)

                # Update request
                if isinstance(query["match"][field], str):
                    query["match"][field] = corrected_text
                elif isinstance(query["match"][field], dict):
                    query["match"][field]["query"] = corrected_text

    # Handle query_string query
    if "query_string" in query:
        query_text = query["query_string"].get("query", "")

        # Correct spelling
        corrected_text, corrections = correct_text(query_text)

        if corrections:
            all_corrections.extend(corrections)
            query["query_string"]["query"] = corrected_text

    # Add metadata about corrections (optional)
    if all_corrections:
        if "_meta" not in request:
            request["_meta"] = {}
        request["_meta"]["spelling_corrections"] = all_corrections

    # Return modified request
    return request


# Example usage:
# Input: {"query": {"match": {"title": "labtop"}}}
# Output: {
#     "query": {"match": {"title": "laptop"}},
#     "_meta": {
#         "spelling_corrections": [
#             {"original": "labtop", "corrected": "laptop"}
#         ]
#     }
# }
