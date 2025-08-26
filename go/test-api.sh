#!/bin/bash

# Test script to demonstrate API vs UI mode
# Make sure the server is running on localhost:8080

echo "=== VideoCompress API Test ==="
echo ""

# Check if server is running
if ! curl -s http://localhost:8080/health > /dev/null; then
    echo "❌ Server is not running. Please start the server first:"
    echo "   cd go && go run main.go"
    exit 1
fi

echo "✅ Server is running"
echo ""

# Test 1: UI Mode (should return HTML)
echo "=== Test 1: UI Mode (default) ==="
echo "This should return HTML page:"
curl -s -X POST \
  -F "file=@test.mp4" \
  -F "speed=ai" \
  http://localhost:8080/compress | head -5
echo ""
echo "❌ This returned HTML instead of file bytes"
echo ""

# Test 2: API Mode with Accept header
echo "=== Test 2: API Mode with Accept header ==="
echo "This should return file bytes:"
curl -s -X POST \
  -H "Accept: application/octet-stream" \
  -F "file=@test.mp4" \
  -F "speed=ai" \
  -o compressed_test.mp4 \
  -D headers.txt \
  http://localhost:8080/compress

if [ -f "compressed_test.mp4" ]; then
    echo "✅ Success! File saved as compressed_test.mp4"
    echo "📊 Metadata headers:"
    cat headers.txt | grep -E "^(X-|Content-)"
    rm headers.txt
else
    echo "❌ Failed to get compressed file"
fi
echo ""

# Test 3: API Mode with api=1 parameter
echo "=== Test 3: API Mode with api=1 parameter ==="
echo "This should also return file bytes:"
curl -s -X POST \
  -F "file=@test.mp4" \
  -F "speed=ai" \
  -F "api=1" \
  -o compressed_api_test.mp4 \
  -D headers_api.txt \
  http://localhost:8080/compress

if [ -f "compressed_api_test.mp4" ]; then
    echo "✅ Success! File saved as compressed_api_test.mp4"
    echo "📊 Metadata headers:"
    cat headers_api.txt | grep -E "^(X-|Content-)"
    rm headers_api.txt
else
    echo "❌ Failed to get compressed file"
fi
echo ""

echo "=== Summary ==="
echo "To get compressed file bytes instead of UI:"
echo "1. Add header: Accept: application/octet-stream"
echo "2. OR add parameter: api=1"
echo ""
echo "See API_USAGE.md for more examples"
