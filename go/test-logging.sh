#!/bin/bash

# Test script to demonstrate comprehensive logging
# Make sure the server is running on localhost:8080

echo "=== VideoCompress API Logging Test ==="
echo ""

# Check if server is running
if ! curl -s http://localhost:8080/health > /dev/null; then
    echo "❌ Server is not running. Please start the server first:"
    echo "   cd go && go run main.go"
    exit 1
fi

echo "✅ Server is running"
echo "📋 Watch the server logs in another terminal to see detailed logging"
echo ""

# Test 1: Health check
echo "=== Test 1: Health Check ==="
curl -s http://localhost:8080/health | jq .
echo ""

# Test 2: API Mode with logging
echo "=== Test 2: API Mode (Get File) ==="
echo "This will show detailed logs for the entire compression process:"
curl -s -X POST \
  -H "Accept: application/octet-stream" \
  -F "file=@test.mp4" \
  -F "speed=ai" \
  -o compressed_test.mp4 \
  -D headers.txt \
  http://localhost:8080/compress

if [ -f "compressed_test.mp4" ]; then
    echo "✅ Success! File saved as compressed_test.mp4"
    echo "📊 Response headers:"
    cat headers.txt | grep -E "^(X-|Content-)"
    rm headers.txt
else
    echo "❌ Failed to get compressed file"
fi
echo ""

# Test 3: UI Mode with logging
echo "=== Test 3: UI Mode (Get HTML) ==="
echo "This will show logs for UI mode processing:"
curl -s -X POST \
  -F "file=@test.mp4" \
  -F "speed=balanced" \
  http://localhost:8080/compress | head -10
echo ""

# Test 4: Download request
echo "=== Test 4: Download Request ==="
echo "This will show logs for file download (if you have a file ID):"
echo "Note: You need a valid file ID from a previous UI mode request"
echo ""

echo "=== Logging Features Demonstrated ==="
echo "✅ Request tracking with unique IDs"
echo "✅ File upload and processing logs"
echo "✅ Compression options and mode selection"
echo "✅ FFmpeg execution and fallback logs"
echo "✅ Performance metrics and timing"
echo "✅ API vs UI mode detection"
echo "✅ File validation and cleanup"
echo "✅ Response generation logs"
echo "✅ Error handling and debugging"
echo ""
echo "📖 Check the server logs for detailed information about each step!"
