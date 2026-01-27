"""
Text Similarity Filter UDF

Filters documents based on text similarity using Levenshtein distance.

# @udf: name=text_similarity
# @udf: version=1.0.0
# @udf: author=quidditch
# @udf: category=filter
# @udf: tags=text,similarity,filter,string
"""


def udf_main() -> bool:
    """
    Main entry point for the text similarity UDF.

    Returns:
        bool: True if document passes the similarity threshold, False otherwise
    """
    # Get document field
    title = get_field_string("title")
    if not title:
        return False

    # Get query parameters
    query_text = get_param_string("query")
    threshold = get_param_i64("threshold")

    # Calculate Levenshtein distance
    distance = levenshtein_distance(title, query_text)

    # Return True if similar enough
    return distance <= threshold


def levenshtein_distance(s1: str, s2: str) -> int:
    """
    Calculate the Levenshtein (edit) distance between two strings.

    Args:
        s1: First string
        s2: Second string

    Returns:
        int: The minimum number of single-character edits required
    """
    # Ensure s1 is the shorter string
    if len(s1) > len(s2):
        s1, s2 = s2, s1

    # Handle empty strings
    if len(s1) == 0:
        return len(s2)
    if len(s2) == 0:
        return len(s1)

    # Create distance matrix
    previous_row = list(range(len(s2) + 1))

    for i, c1 in enumerate(s1):
        current_row = [i + 1]
        for j, c2 in enumerate(s2):
            # Calculate costs
            insertions = previous_row[j + 1] + 1
            deletions = current_row[j] + 1
            substitutions = previous_row[j] + (0 if c1 == c2 else 1)

            # Take minimum
            current_row.append(min(insertions, deletions, substitutions))

        previous_row = current_row

    return previous_row[-1]


# Host function declarations (implemented by WASM runtime)
# These functions are provided by Quidditch and access document/query data

def get_field_string(field_name: str) -> str:
    """
    Get a string field from the current document.

    Args:
        field_name: Name of the field to retrieve (e.g., "title", "content")

    Returns:
        str: The field value, or empty string if not found
    """
    pass  # Implemented by host


def get_field_int(field_name: str) -> int:
    """
    Get an integer field from the current document.

    Args:
        field_name: Name of the field to retrieve

    Returns:
        int: The field value, or 0 if not found
    """
    pass  # Implemented by host


def get_field_float(field_name: str) -> float:
    """
    Get a float field from the current document.

    Args:
        field_name: Name of the field to retrieve

    Returns:
        float: The field value, or 0.0 if not found
    """
    pass  # Implemented by host


def get_field_bool(field_name: str) -> bool:
    """
    Get a boolean field from the current document.

    Args:
        field_name: Name of the field to retrieve

    Returns:
        bool: The field value, or False if not found
    """
    pass  # Implemented by host


def get_param_string(param_name: str) -> str:
    """
    Get a string parameter from the query.

    Args:
        param_name: Name of the parameter (e.g., "query", "target")

    Returns:
        str: The parameter value, or empty string if not found
    """
    pass  # Implemented by host


def get_param_i64(param_name: str) -> int:
    """
    Get an integer parameter from the query.

    Args:
        param_name: Name of the parameter (e.g., "threshold", "max_distance")

    Returns:
        int: The parameter value, or 0 if not found
    """
    pass  # Implemented by host


def get_param_f64(param_name: str) -> float:
    """
    Get a float parameter from the query.

    Args:
        param_name: Name of the parameter

    Returns:
        float: The parameter value, or 0.0 if not found
    """
    pass  # Implemented by host


def get_param_bool(param_name: str) -> bool:
    """
    Get a boolean parameter from the query.

    Args:
        param_name: Name of the parameter

    Returns:
        bool: The parameter value, or False if not found
    """
    pass  # Implemented by host


def log(message: str) -> None:
    """
    Log a message (for debugging).

    Args:
        message: Message to log
    """
    pass  # Implemented by host


def get_document_id() -> str:
    """
    Get the current document's ID.

    Returns:
        str: The document ID
    """
    pass  # Implemented by host


def get_score() -> float:
    """
    Get the current document's search score.

    Returns:
        float: The BM25 or relevance score
    """
    pass  # Implemented by host
