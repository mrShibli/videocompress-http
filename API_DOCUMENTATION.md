# VideoCompress API Documentation

## Base URL
```
http://localhost:8080
```

## Authentication
No authentication required for this API.

## Content Types
- **Input**: `multipart/form-data` for file uploads
- **Output**: `application/octet-stream` for compressed files, `application/json` for metadata

---

## Endpoints

### 1. Health Check

**GET** `/health`

Check server status and get available features.

**Response:**
```json
{
  "ok": true,
  "service": "videocompress",
  "version": "3.2.0-orientation",
  "modes": ["ai", "turbo", "max", "ultra_fast", "super_fast", "fast", "balanced", "quality"],
  "defaults": {
    "codec": "h264",
    "resolution": "original",
    "hw": "none"
  },
  "ui_routes": ["/", "/compress (POST)", "/dl/{id}", "/meta/{id}"]
}
```

---

### 2. Web Interface

**GET** `/`

Returns the web interface HTML for browser-based video compression.

---

### 3. Compress Video

**POST** `/compress`

Compress a video file with specified settings.

#### Request Parameters

| Parameter | Type | Required | Default | Description |
|-----------|------|----------|---------|-------------|
| `file` | File | ✅ Yes | - | Video file to compress |
| `speed` | String | ❌ No | `ai` | Compression speed mode |
| `resolution` | String | ❌ No | `original` | Output resolution |
| `codec` | String | ❌ No | `h264` | Video codec |
| `audio` | String | ❌ No | `aac` | Audio codec |
| `hw` | String | ❌ No | `none` | Hardware acceleration |
| `outExt` | String | ❌ No | `.mp4` | Output file extension |
| `fps` | Number | ❌ No | auto | Force output frame rate |
| `ui` | String | ❌ No | - | Set to "1" for web UI response |

#### Speed Modes

| Mode | CRF | Preset | Audio | Description |
|------|-----|--------|-------|-------------|
| `ai` | Auto | Auto | Auto | AI selects based on file size |
| `turbo` | 34 | ultrafast | 96k stereo | Very fast, 720p long-edge |
| `max` | 36 | ultrafast | 64k mono | Maximum compression, 480p long-edge |
| `ultra_fast` | 32 | ultrafast | 96k | Ultra fast compression |
| `super_fast` | 30 | ultrafast | 96k | Super fast compression |
| `fast` | 28 | veryfast | 128k | Fast compression |
| `balanced` | 26 | veryfast | 128k | Balanced speed/quality |
| `quality` | 23 | fast | 128k | High quality compression |

#### Resolution Options

| Value | Output Size | Description |
|-------|-------------|-------------|
| `original` | Original | Keep original resolution |
| `360p` | 640x360 | 360p resolution |
| `480p` | 854x480 | 480p resolution |
| `720p` | 1280x720 | 720p resolution |
| `1080p` | 1920x1080 | 1080p resolution |
| `1440p` | 2560x1440 | 1440p resolution |
| `2160p` | 3840x2160 | 4K resolution |

#### Codec Options

| Parameter | Values | Description |
|-----------|--------|-------------|
| `codec` | `h264`, `h265`, `copy` | Video codec |
| `audio` | `aac`, `opus`, `copy` | Audio codec |
| `hw` | `none`, `videotoolbox` | Hardware acceleration |

#### Response Headers (API Mode)

When using `Accept: application/octet-stream`, the response includes these headers:

| Header | Description | Example |
|--------|-------------|---------|
| `X-Mode` | Final compression mode used | `balanced` |
| `X-Mode-Decider` | How mode was chosen | `ai` or `manual` |
| `X-Encode-Duration-Ms` | Encoding time in milliseconds | `15000` |
| `X-Throughput-MBps` | Processing speed in MB/s | `25.5` |
| `X-Input-Bytes` | Original file size in bytes | `52428800` |
| `X-Output-Bytes` | Compressed file size in bytes | `15728640` |
| `X-Resolution` | Output resolution | `720p` |
| `X-Video-Codec` | Video codec used | `h264` |
| `X-Audio-Codec` | Audio codec used | `aac` |
| `X-HW` | Hardware acceleration used | `none` |

#### Example Requests

**Basic AI Compression:**
```bash
curl -X POST \
  -H "Accept: application/octet-stream" \
  -F "file=@video.mp4" \
  -F "speed=ai" \
  -o compressed.mp4 \
  http://localhost:8080/compress
```

**Custom Settings:**
```bash
curl -X POST \
  -H "Accept: application/octet-stream" \
  -F "file=@video.mp4" \
  -F "speed=balanced" \
  -F "resolution=720p" \
  -F "codec=h264" \
  -F "hw=videotoolbox" \
  -o compressed.mp4 \
  http://localhost:8080/compress
```

---

### 4. Download Compressed File

**GET** `/dl/{id}`

Download a compressed file from the temporary store (web UI flow only).

#### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | String | ✅ Yes | File ID from compression result |
| `name` | String | ❌ No | Custom filename for download |

#### Example
```
GET /dl/abc123def456?name=my_video.mp4
```

---

### 5. Get Compression Metadata

**GET** `/meta/{id}`

Get metadata for a compressed file (web UI flow only).

#### Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `id` | String | ✅ Yes | File ID from compression result |

#### Response
```json
{
  "id": "abc123def456",
  "mode": "balanced",
  "mode_decider": "ai",
  "input_bytes": 52428800,
  "output_bytes": 15728640,
  "resolution": "720p",
  "codec": "h264",
  "audio": "aac",
  "hw": "none",
  "encode_duration_ms": 15000,
  "throughput_mb_s": 25.5
}
```

---

## Error Responses

### 400 Bad Request
```json
{
  "error": "file field required"
}
```

### 404 Not Found
```json
{
  "error": "Not found"
}
```

### 500 Internal Server Error
```json
{
  "error": "compression failed: FFmpeg error message"
}
```

---

## Rate Limits

- **File Size Limit**: 2GB maximum
- **Concurrent Requests**: No limit (depends on server resources)
- **Supported Formats**: All video formats supported by FFmpeg

---

## Best Practices

1. **Use AI Mode**: For automatic optimization based on file size
2. **Choose Appropriate Resolution**: Don't upscale unnecessarily
3. **Hardware Acceleration**: Enable VideoToolbox on macOS for better performance
4. **Monitor Headers**: Check response headers for performance metrics
5. **Error Handling**: Always check response status and handle errors gracefully

---

## Testing with Postman

See the Postman collection below for ready-to-use requests.
