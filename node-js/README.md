# VideoCompress HTTP Server (Node.js)

A high-performance video compression HTTP server with AI-powered speed selection, built with Node.js and FFmpeg.

## Features

- **AI Speed Selection**: Automatically chooses optimal compression settings based on file size
- **Multiple Speed Modes**: turbo, max, ultra_fast, super_fast, fast, balanced, quality
- **Hardware Acceleration**: macOS VideoToolbox support with CPU fallback
- **Orientation-Safe Scaling**: Preserves video orientation while scaling
- **Web UI**: Beautiful, responsive interface for easy video compression
- **REST API**: Programmatic access with detailed metadata headers
- **Multiple Codecs**: H.264, H.265/HEVC, and stream copying
- **Audio Support**: AAC, Opus, and audio stream copying
- **Resolution Control**: 360p to 4K with original preservation option

## Installation

### Prerequisites

1. **Node.js** (v14 or higher)
2. **FFmpeg** - Install from [ffmpeg.org](https://ffmpeg.org/download.html)

### Setup

1. Clone or download the project files
2. Install dependencies:
   ```bash
   npm install
   ```

3. Start the server:
   ```bash
   npm start
   ```

   Or for development with auto-restart:
   ```bash
   npm run dev
   ```

The server will start on `http://localhost:8080` (or the port specified in the `PORT` environment variable).

## Usage

### Web Interface

1. Open `http://localhost:8080` in your browser
2. Upload a video file
3. Choose compression settings or use AI mode
4. Download the compressed video

### API Usage

#### Basic Compression
```bash
curl -f -S -o out.mp4 \
  -H "Accept: application/octet-stream" \
  -F "file=@input.mp4" \
  -F "speed=ai" \
  http://localhost:8080/compress -D headers.txt
```

#### With Custom Settings
```bash
curl -f -S -o out.mp4 \
  -H "Accept: application/octet-stream" \
  -F "file=@input.mp4" \
  -F "speed=balanced" \
  -F "resolution=720p" \
  -F "codec=h264" \
  -F "hw=videotoolbox" \
  http://localhost:8080/compress
```

### API Parameters

| Parameter | Values | Default | Description |
|-----------|--------|---------|-------------|
| `speed` | `ai`, `turbo`, `max`, `ultra_fast`, `super_fast`, `fast`, `balanced`, `quality` | `ai` | Compression speed mode |
| `resolution` | `original`, `360p`, `480p`, `720p`, `1080p`, `1440p`, `2160p` | `original` | Output resolution |
| `codec` | `h264`, `h265`, `copy` | `h264` | Video codec |
| `audio` | `aac`, `opus`, `copy` | `aac` | Audio codec |
| `hw` | `none`, `videotoolbox` | `none` | Hardware acceleration |
| `outExt` | `.mp4`, `.mov` | `.mp4` | Output file extension |
| `fps` | `1-60` | auto | Force output frame rate |

### Response Headers

API responses include detailed metadata headers:

- `X-Mode`: Final compression mode used
- `X-Mode-Decider`: Whether mode was chosen by AI or manually
- `X-Encode-Duration-Ms`: Encoding time in milliseconds
- `X-Throughput-MBps`: Processing speed in MB/s
- `X-Input-Bytes`: Original file size
- `X-Output-Bytes`: Compressed file size
- `X-Resolution`: Output resolution
- `X-Video-Codec`: Video codec used
- `X-Audio-Codec`: Audio codec used
- `X-HW`: Hardware acceleration used

## Speed Modes

### AI Mode (Default)
Automatically selects the best speed mode based on file size:
- **≥700MB**: ultra_fast
- **≥200MB**: super_fast  
- **≥50MB**: fast
- **≥10MB**: balanced
- **<10MB**: balanced

### Manual Modes

| Mode | CRF | Preset | Audio | Description |
|------|-----|--------|-------|-------------|
| `turbo` | 34 | ultrafast | 96k stereo | Very fast, 720p long-edge |
| `max` | 36 | ultrafast | 64k mono | Maximum compression, 480p long-edge |
| `ultra_fast` | 32 | ultrafast | 96k | Ultra fast compression |
| `super_fast` | 30 | ultrafast | 96k | Super fast compression |
| `fast` | 28 | veryfast | 128k | Fast compression |
| `balanced` | 26 | veryfast | 128k | Balanced speed/quality |
| `quality` | 23 | fast | 128k | High quality compression |

## Hardware Acceleration

### macOS VideoToolbox
Enable hardware acceleration on macOS:
```bash
curl -F "file=@video.mp4" -F "hw=videotoolbox" http://localhost:8080/compress
```

The server automatically falls back to CPU encoding if hardware acceleration fails.

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Web interface |
| `/compress` | POST | Compress video file |
| `/dl/:id` | GET | Download compressed file |
| `/meta/:id` | GET | Get compression metadata |
| `/health` | GET | Server health check |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | Server port |

## File Size Limits

- **Maximum upload size**: 2GB
- **Supported formats**: All video formats supported by FFmpeg
- **Recommended output**: MP4 (H.264/AAC) for maximum compatibility

## Performance Tips

1. **Use AI mode** for automatic optimization
2. **Enable VideoToolbox** on macOS for faster encoding
3. **Choose appropriate resolution** - don't upscale unnecessarily
4. **Use turbo/max modes** for quick previews
5. **Consider H.265** for better compression (but slower encoding)

## Troubleshooting

### FFmpeg Not Found
Ensure FFmpeg is installed and accessible in your PATH:
```bash
ffmpeg -version
```

### Hardware Acceleration Issues
If VideoToolbox fails, the server automatically falls back to CPU encoding. Check FFmpeg hardware support:
```bash
ffmpeg -encoders | grep videotoolbox
```

### Memory Issues
For large files, ensure sufficient system memory. The server processes files in chunks to minimize memory usage.

## License

MIT License - see LICENSE file for details.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

## Version History

- **3.2.0-orientation**: Orientation-safe scaling, improved turbo/max modes
- **3.1.0**: Hardware acceleration support
- **3.0.0**: AI speed selection, web interface
- **2.0.0**: REST API, metadata headers
- **1.0.0**: Basic video compression
