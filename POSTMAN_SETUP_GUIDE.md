# Postman Setup Guide for VideoCompress API

## 🚀 Quick Start

### 1. Import the Collection

1. **Open Postman**
2. **Click "Import"** (top left)
3. **Import the collection file:**
   - Drag and drop `VideoCompress_API.postman_collection.json`
   - Or click "Upload Files" and select the file

### 2. Import the Environment

1. **Import the environment file:**
   - Drag and drop `VideoCompress_Environment.postman_environment.json`
   - Or click "Upload Files" and select the file

2. **Select the environment:**
   - In the top-right corner, select "VideoCompress Environment" from the dropdown

### 3. Start the Server

```bash
# Install dependencies (if not done already)
npm install

# Start the server
npm start
```

The server will start on `http://localhost:8080`

---

## 📋 Available Requests

### Basic Requests

| Request | Method | Description |
|---------|--------|-------------|
| **Health Check** | GET | Check if server is running |
| **Web Interface** | GET | Get the web UI |
| **Compress Video - AI Mode** | POST | Auto-compress with AI settings |

### Advanced Requests

| Request | Method | Description |
|---------|--------|-------------|
| **Compress Video - Turbo Mode** | POST | Maximum speed compression |
| **Compress Video - Balanced Mode** | POST | Balanced speed/quality |
| **Compress Video - Quality Mode** | POST | High quality compression |
| **Compress Video - Hardware Acceleration** | POST | Use GPU acceleration |
| **Compress Video - Custom Settings** | POST | Full control over all parameters |
| **Compress Video - Web UI Response** | POST | Get HTML response with download link |

### Utility Requests

| Request | Method | Description |
|---------|--------|-------------|
| **Download Compressed File** | GET | Download compressed file (web UI flow) |
| **Get Compression Metadata** | GET | Get compression details (web UI flow) |

---

## 🎯 How to Use Each Request

### 1. Health Check
**Purpose:** Verify server is running and get available features

1. Select "Health Check" request
2. Click "Send"
3. **Expected Response:**
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
     }
   }
   ```

### 2. Compress Video - AI Mode
**Purpose:** Automatically compress video with optimal settings

1. Select "Compress Video - AI Mode" request
2. In the **Body** tab, under **form-data**:
   - Click "Select Files" next to the `file` field
   - Choose your video file (MP4, MOV, etc.)
3. Click "Send"
4. **Response:** The compressed video file will be downloaded automatically

**Response Headers to Check:**
- `X-Mode`: Final compression mode used
- `X-Encode-Duration-Ms`: Time taken in milliseconds
- `X-Throughput-MBps`: Processing speed
- `X-Input-Bytes`: Original file size
- `X-Output-Bytes`: Compressed file size

### 3. Compress Video - Custom Settings
**Purpose:** Full control over compression parameters

1. Select "Compress Video - Custom Settings" request
2. In the **Body** tab, configure:
   - **file**: Upload your video
   - **speed**: Choose from `ai`, `turbo`, `max`, `ultra_fast`, `super_fast`, `fast`, `balanced`, `quality`
   - **resolution**: Choose from `original`, `360p`, `480p`, `720p`, `1080p`, `1440p`, `2160p`
   - **codec**: Choose from `h264`, `h265`, `copy`
   - **audio**: Choose from `aac`, `opus`, `copy`
   - **hw**: Choose from `none`, `videotoolbox` (macOS only)
   - **outExt**: Choose from `.mp4`, `.mov`
   - **fps**: Enter frame rate (1-60) or leave empty for auto
3. Click "Send"

### 4. Compress Video - Web UI Response
**Purpose:** Get HTML response with download link (for web integration)

1. Select "Compress Video - Web UI Response" request
2. Upload your video file
3. Click "Send"
4. **Response:** HTML page with compression results and download link

---

## 🔧 Environment Variables

The collection uses these environment variables:

| Variable | Default Value | Description |
|----------|---------------|-------------|
| `base_url` | `http://localhost:8080` | Server base URL |
| `file_id` | (empty) | Auto-populated file ID for downloads |
| `server_port` | `8080` | Server port |
| `max_file_size` | `2147483648` | Maximum file size (2GB) |

### How to Change Server URL

1. **Edit Environment:**
   - Click the environment dropdown (top-right)
   - Click the edit icon (pencil)
   - Change `base_url` value
   - Click "Save"

2. **Or use a different environment:**
   - Create a new environment
   - Set `base_url` to your server URL
   - Select the new environment

---

## 📊 Understanding Response Headers

When using API mode (`Accept: application/octet-stream`), check these headers:

| Header | Example | Description |
|--------|---------|-------------|
| `X-Mode` | `balanced` | Final compression mode used |
| `X-Mode-Decider` | `ai` | How mode was chosen (ai/manual) |
| `X-Encode-Duration-Ms` | `15000` | Encoding time in milliseconds |
| `X-Throughput-MBps` | `25.5` | Processing speed in MB/s |
| `X-Input-Bytes` | `52428800` | Original file size |
| `X-Output-Bytes` | `15728640` | Compressed file size |
| `X-Resolution` | `720p` | Output resolution |
| `X-Video-Codec` | `h264` | Video codec used |
| `X-Audio-Codec` | `aac` | Audio codec used |
| `X-HW` | `none` | Hardware acceleration used |

---

## 🎨 Speed Mode Comparison

| Mode | Best For | Speed | Quality | File Size |
|------|----------|-------|---------|-----------|
| `turbo` | Quick previews | ⚡⚡⚡ | ⭐ | 🗜️🗜️🗜️ |
| `max` | Maximum compression | ⚡⚡⚡ | ⭐ | 🗜️🗜️🗜️🗜️ |
| `ultra_fast` | Large files | ⚡⚡⚡ | ⭐⭐ | 🗜️🗜️ |
| `super_fast` | Fast compression | ⚡⚡ | ⭐⭐ | 🗜️🗜️ |
| `fast` | Good balance | ⚡⚡ | ⭐⭐⭐ | 🗜️ |
| `balanced` | Default choice | ⚡ | ⭐⭐⭐ | 🗜️ |
| `quality` | High quality | ⚡ | ⭐⭐⭐⭐ | 🗜️ |
| `ai` | Automatic | Auto | Auto | Auto |

---

## 🛠️ Troubleshooting

### Common Issues

**1. "Connection refused" error**
- Make sure the server is running: `npm start`
- Check if port 8080 is available
- Verify the `base_url` in environment

**2. "file field required" error**
- Make sure you've selected a file in the form-data
- Check that the file key is named exactly `file`

**3. "compression failed" error**
- Ensure FFmpeg is installed and in PATH
- Check if the video file is valid
- Try a smaller file first

**4. Large files timeout**
- Increase Postman timeout settings
- Use smaller files for testing
- Check server memory usage

### Testing with Small Files

For testing, use small video files (1-10MB) to:
- Verify the API works
- Check response headers
- Test different modes quickly

### Performance Tips

1. **Use AI mode** for automatic optimization
2. **Enable hardware acceleration** on macOS for better performance
3. **Choose appropriate resolution** - don't upscale unnecessarily
4. **Monitor response headers** for performance metrics

---

## 📝 Example Workflows

### Workflow 1: Quick Test
1. Run "Health Check" to verify server
2. Run "Compress Video - AI Mode" with a small test file
3. Check response headers for performance data

### Workflow 2: Quality Compression
1. Run "Compress Video - Quality Mode" with H.265 codec
2. Set resolution to 1080p
3. Use hardware acceleration if available

### Workflow 3: Web Integration
1. Run "Compress Video - Web UI Response"
2. Extract the file ID from the HTML response
3. Use the file ID to download or get metadata

---

## 🔗 Additional Resources

- **API Documentation**: See `API_DOCUMENTATION.md`
- **Server Code**: See `server.js`
- **Example Scripts**: See `example.js` and `test.js`
- **GitHub**: Full source code and issues

---

## 🎉 Success!

You're now ready to test the VideoCompress API with Postman! Start with the Health Check to verify everything is working, then try compressing some videos with different modes.
