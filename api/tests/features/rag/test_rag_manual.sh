#!/bin/bash

# Configuration
API_URL="http://localhost:8000/v1/rag"
TOKEN="YOUR_AUTH_TOKEN"  # Replace with actual token
AUTH_HEADER="Authorization: Bearer $TOKEN"

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

# Test function
test_endpoint() {
    echo -e "${GREEN}Testing: $1${NC}"
    eval "$2"
    echo
}

# Create test PDF
echo "Creating test PDF..."
cat > test.pdf << EOL
%PDF-1.5
Retrieval-Augmented Generation (RAG) is a technique that combines retrieval and generation
to create more accurate and contextual responses. It works by first retrieving relevant
information from a knowledge base, then using that information to generate responses.

Key benefits of RAG include:
1. Improved accuracy
2. Up-to-date information
3. Reduced hallucination
4. Better context awareness
EOL

# 1. Upload document
test_endpoint "Upload PDF" "curl -X POST \
  -H \"$AUTH_HEADER\" \
  -F \"file=@test.pdf\" \
  $API_URL/upload"

# Store document ID
DOC_ID=$(curl -s -X POST -H "$AUTH_HEADER" -F "file=@test.pdf" $API_URL/upload | jq -r '.id')

# 2. List documents
test_endpoint "List Documents" "curl -X GET \
  -H \"$AUTH_HEADER\" \
  \"$API_URL/documents?page=1&page_size=10\""

# 3. Get document details
test_endpoint "Get Document Details" "curl -X GET \
  -H \"$AUTH_HEADER\" \
  $API_URL/documents/$DOC_ID"

# 4. Search documents
test_endpoint "Search Documents" "curl -X POST \
  -H \"$AUTH_HEADER\" \
  -H \"Content-Type: application/json\" \
  -d '{
    \"query\": \"What is RAG?\",
    \"document_ids\": [$DOC_ID],
    \"top_k\": 3
  }' \
  $API_URL/search"

# Store search results for generation
SEARCH_RESULTS=$(curl -s -X POST \
  -H "$AUTH_HEADER" \
  -H "Content-Type: application/json" \
  -d "{
    \"query\": \"What is RAG?\",
    \"document_ids\": [$DOC_ID],
    \"top_k\": 3
  }" \
  $API_URL/search)

# 5. Generate response
test_endpoint "Generate Response" "curl -X POST \
  -H \"$AUTH_HEADER\" \
  -H \"Content-Type: application/json\" \
  -d '{
    \"query\": \"Explain RAG and its benefits\",
    \"context_chunks\": $SEARCH_RESULTS,
    \"temperature\": 0.7
  }' \
  $API_URL/generate"

# 6. Combined query
test_endpoint "Combined Query" "curl -X POST \
  -H \"$AUTH_HEADER\" \
  -H \"Content-Type: application/json\" \
  -d '{
    \"query\": \"What are the main benefits of RAG?\",
    \"document_ids\": [$DOC_ID],
    \"top_k\": 3,
    \"temperature\": 0.7
  }' \
  $API_URL/query"

# 7. Delete document
test_endpoint "Delete Document" "curl -X DELETE \
  -H \"$AUTH_HEADER\" \
  $API_URL/documents/$DOC_ID"

# Cleanup
rm test.pdf

echo -e "${GREEN}All tests completed!${NC}"
