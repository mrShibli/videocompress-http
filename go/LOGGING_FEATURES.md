# VideoCompress API - Comprehensive Logging Features

## Overview

The VideoCompress API now includes comprehensive logging for every process, making it easy to track, debug, and monitor video compression operations.

## Logging Features

### 🚀 Startup Logging
- Server startup information
- Working directory
- Maximum upload size
- FFmpeg availability check

### 📥 Request Tracking
- Unique request IDs for each operation
- Client IP address tracking
- HTTP method and URL logging
- Request timing and status codes

### 📁 File Processing
- File upload details (name, size)
- Temporary file creation and cleanup
- File validation and statistics
- Compression ratio calculations

### ⚙️ Configuration Logging
- Compression options parsing
- Speed mode selection (AI vs Manual)
- Hardware acceleration settings
- Codec and resolution choices

### 🤖 AI Mode Decision Making
- File size analysis
- AI mode selection logic
- Mode adjustments for different file sizes
- Final mode determination

### 🔧 FFmpeg Execution
- Complete FFmpeg command logging
- Hardware acceleration attempts
- Fallback to CPU when needed
- Execution success/failure tracking

### 📊 Performance Metrics
- Compression timing
- Throughput calculations (MB/s)
- Input/output file sizes
- Size reduction percentages

### 🎯 Response Mode Detection
- API vs UI mode detection
- Header and parameter analysis
- Response type determination

### 📤 Response Generation
- File serving details
- Metadata header generation
- HTML page rendering
- Download link creation

### 🧹 Cleanup Operations
- Temporary file removal
- Resource cleanup
- Error handling

## Log Format

Each log entry includes:
- **Timestamp**: Microsecond precision
- **Request ID**: Unique identifier for tracking
- **Emoji**: Visual indicator of operation type
- **Message**: Detailed description of the operation

### Example Log Output

```
2024/01/15 10:30:45.123456 🚀 VideoCompress API starting up...
2024/01/15 10:30:45.123457 📁 Working directory: /Users/user/code/videocompress
2024/01/15 10:30:45.123458 💾 Max upload size: 2.00 GB
2024/01/15 10:30:45.123459 🔧 FFmpeg available: true
2024/01/15 10:30:45.123460 🚀 [MAIN] Starting VideoCompress server on port 8080
2024/01/15 10:30:45.123461 🚀 [MAIN] VideoCompress server listening on http://localhost:8080

2024/01/15 10:31:00.123456 📥 [abc12345] New compression request from 127.0.0.1:54321
2024/01/15 10:31:00.123457 📋 [abc12345] Method: POST, URL: /compress
2024/01/15 10:31:00.123458 🎬 [abc12345] Processing compression request
2024/01/15 10:31:00.123459 📝 [abc12345] Parsing multipart form data...
2024/01/15 10:31:00.123460 ✅ [abc12345] Multipart form parsed successfully
2024/01/15 10:31:00.123461 📁 [abc12345] Extracting uploaded file...
2024/01/15 10:31:00.123462 📄 [abc12345] File received: video.mp4 (50.25 MB)
2024/01/15 10:31:00.123463 💾 [abc12345] Saving uploaded file to temp directory...
2024/01/15 10:31:00.123464 📂 [abc12345] Temp file path: /tmp/video.mp4
2024/01/15 10:31:00.123465 📥 [abc12345] Copying file data to temp location...
2024/01/15 10:31:00.123466 ✅ [abc12345] File saved to temp location successfully
2024/01/15 10:31:00.123467 ⚙️ [abc12345] Parsing compression options...
2024/01/15 10:31:00.123468 ✅ [abc12345] Options parsed: speed=ai, resolution=original, codec=h264, audio=aac, hw=none
2024/01/15 10:31:00.123469 📊 [abc12345] Calculating file statistics...
2024/01/15 10:31:00.123470 📈 [abc12345] File size: 50.25 MB (52674560 bytes, 50 MB)
2024/01/15 10:31:00.123471 🤖 [abc12345] Processing speed mode decision...
2024/01/15 10:31:00.123472 🧠 [abc12345] AI selected base mode: balanced (for 50 MB file)
2024/01/15 10:31:00.123473 🎯 [abc12345] Final AI mode: balanced
2024/01/15 10:31:00.123474 🛡️ [abc12345] Applying small-file safety checks...
2024/01/15 10:31:00.123475 ✅ [abc12345] Safety checks applied
2024/01/15 10:31:00.123476 ⚙️ [abc12345] Applying speed profile parameters...
2024/01/15 10:31:00.123477 ✅ [abc12345] Profile applied: CRF=26, Preset=veryfast, AB=128k
2024/01/15 10:31:00.123478 🎬 [abc12345] Output path: /tmp/video_compressed.mp4
2024/01/15 10:31:00.123479 ⏱️ [abc12345] Starting compression process...
2024/01/15 10:31:00.123480 🔧 [abc12345] Executing FFmpeg compression...
2024/01/15 10:31:00.123481 🔧 [def67890] Starting FFmpeg compression
2024/01/15 10:31:00.123482 ⚙️ [def67890] FFmpeg command: ffmpeg -y -hide_banner -loglevel error -i /tmp/video.mp4 -c:v libx264 -crf 26 -preset veryfast -c:a aac -b:a 128k -movflags +faststart -threads 0 /tmp/video_compressed.mp4
2024/01/15 10:31:00.123483 ▶️ [def67890] Executing FFmpeg with hardware: none
2024/01/15 10:31:15.123456 ✅ [def67890] FFmpeg compression completed successfully
2024/01/15 10:31:15.123457 ✅ [abc12345] Compression completed in 15000 ms
2024/01/15 10:31:15.123458 🔍 [abc12345] Validating compressed output...
2024/01/15 10:31:15.123459 ✅ [abc12345] Output validated: 15.73 MB (16485760 bytes)
2024/01/15 10:31:15.123460 📊 [abc12345] Calculating compression statistics...
2024/01/15 10:31:15.123461 📈 [abc12345] Compression stats: 3.35 MB/s throughput, 68.7% size reduction
2024/01/15 10:31:15.123462 🎯 [abc12345] Determining response mode...
2024/01/15 10:31:15.123463 📋 [abc12345] Accept header: application/octet-stream
2024/01/15 10:31:15.123464 🔧 [abc12345] API parameter: 
2024/01/15 10:31:15.123465 📤 [abc12345] API MODE: Returning compressed file directly
2024/01/15 10:31:15.123466 📤 [abc12345] Serving compressed file: video_compressed.mp4 (video/mp4)
2024/01/15 10:31:15.123467 ✅ [abc12345] API response completed successfully
2024/01/15 10:31:15.123468 🧹 [abc12345] Cleaning up temp file: /tmp/video.mp4
2024/01/15 10:31:15.123469 📊 [HTTP] POST /compress - 200 - 127.0.0.1:54321 - 15.123s
```

## Log Categories

### 📥 Request Processing
- `📥` New request received
- `📋` Request details
- `🎬` Processing started

### 📁 File Operations
- `📁` File extraction
- `📄` File details
- `💾` File saving
- `📂` File paths
- `🧹` Cleanup

### ⚙️ Configuration
- `⚙️` Options parsing
- `✅` Success confirmations
- `❌` Error conditions

### 🤖 AI Processing
- `🤖` AI mode processing
- `🧠` AI decisions
- `🎯` Final mode selection

### 🔧 FFmpeg
- `🔧` FFmpeg operations
- `⚙️` Command details
- `▶️` Execution
- `🔄` Fallback attempts

### 📊 Performance
- `📊` Statistics calculation
- `📈` Performance metrics
- `⏱️` Timing information

### 🎯 Response
- `🎯` Mode detection
- `📤` File serving
- `🌐` UI rendering

### 🏥 Health & Status
- `🏥` Health checks
- `📚` Documentation
- `🚀` Server status

## Benefits

### 🔍 Debugging
- Track every step of the compression process
- Identify bottlenecks and failures
- Understand AI mode decisions

### 📈 Monitoring
- Performance metrics for each request
- Throughput and timing analysis
- Success/failure rates

### 🛠️ Development
- Easy to add new logging points
- Consistent log format
- Request tracing with unique IDs

### 🚨 Troubleshooting
- Detailed error information
- FFmpeg command logging
- Fallback mechanism tracking

## Usage

### Starting the Server
```bash
cd go
go run main.go
```

### Testing Logging
```bash
./test-logging.sh
```

### Monitoring Logs
```bash
# In another terminal
tail -f /path/to/logs
```

## Configuration

The logging system is built into the application and provides:
- **Microsecond precision timestamps**
- **Unique request IDs** for tracking
- **Emoji indicators** for quick visual scanning
- **Detailed operation descriptions**
- **Performance metrics** for optimization

All logs are written to stdout and can be redirected to files or log management systems as needed.
