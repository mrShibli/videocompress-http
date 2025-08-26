# VideoCompress API Usage Guide

## Problem: Getting UI instead of compressed file

When you call the `/compress` API, you might get an HTML page instead of the compressed video file. This happens because the API has two modes:

1. **UI Mode** (default): Returns an HTML page with download links
2. **API Mode**: Returns the compressed file directly

## Solution: Use API Mode

To get the compressed file directly, you need to trigger API mode using one of these methods:

### Method 1: Set Accept Header (Recommended)

```bash
curl -X POST \
  -H "Accept: application/octet-stream" \
  -F "file=@input.mp4" \
  -F "speed=ai" \
  -o compressed.mp4 \
  http://localhost:8080/compress
```

### Method 2: Add api=1 Parameter

```bash
curl -X POST \
  -F "file=@input.mp4" \
  -F "speed=ai" \
  -F "api=1" \
  -o compressed.mp4 \
  http://localhost:8080/compress
```

## Programming Examples

### Go Example

```go
package main

import (
    "bytes"
    "io"
    "mime/multipart"
    "net/http"
    "os"
    "path/filepath"
)

func compressVideo(filePath, speed string) error {
    file, err := os.Open(filePath)
    if err != nil {
        return err
    }
    defer file.Close()

    var buf bytes.Buffer
    writer := multipart.NewWriter(&buf)
    
    part, err := writer.CreateFormFile("file", filepath.Base(filePath))
    if err != nil {
        return err
    }
    io.Copy(part, file)
    
    writer.WriteField("speed", speed)
    writer.Close()

    req, err := http.NewRequest("POST", "http://localhost:8080/compress", &buf)
    if err != nil {
        return err
    }
    
    req.Header.Set("Content-Type", writer.FormDataContentType())
    // This is the key header to get file bytes instead of UI
    req.Header.Set("Accept", "application/octet-stream")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()

    if resp.StatusCode != 200 {
        return fmt.Errorf("HTTP %d", resp.StatusCode)
    }

    // Save compressed file
    out, err := os.Create("compressed.mp4")
    if err != nil {
        return err
    }
    defer out.Close()

    _, err = io.Copy(out, resp.Body)
    return err
}
```

### PHP Example

```php
<?php
$url = 'http://localhost:8080/compress';
$file_path = 'video.mp4';

$post_data = [
    'file' => new CURLFile($file_path),
    'speed' => 'ai'
];

$ch = curl_init();
curl_setopt($ch, CURLOPT_URL, $url);
curl_setopt($ch, CURLOPT_POST, true);
curl_setopt($ch, CURLOPT_POSTFIELDS, $post_data);
curl_setopt($ch, CURLOPT_RETURNTRANSFER, true);
curl_setopt($ch, CURLOPT_HTTPHEADER, [
    'Accept: application/octet-stream'  // This is the key header
]);

$response = curl_exec($ch);
$http_code = curl_getinfo($ch, CURLINFO_HTTP_CODE);
curl_close($ch);

if ($http_code === 200) {
    file_put_contents('compressed.mp4', $response);
    echo "Video compressed successfully!";
} else {
    echo "Error: HTTP $http_code";
}
?>
```

### Node.js Example

```javascript
const FormData = require('form-data');
const fs = require('fs');
const fetch = require('node-fetch');

const form = new FormData();
form.append('file', fs.createReadStream('video.mp4'));
form.append('speed', 'ai');

const response = await fetch('http://localhost:8080/compress', {
    method: 'POST',
    body: form,
    headers: { 
        'Accept': 'application/octet-stream'  // This is the key header
    }
});

const fileStream = fs.createWriteStream('compressed.mp4');
response.body.pipe(fileStream);
```

### Python Example

```python
import requests

with open('video.mp4', 'rb') as f:
    files = {'file': f}
    data = {'speed': 'ai'}
    headers = {'Accept': 'application/octet-stream'}  # This is the key header

    response = requests.post(
        'http://localhost:8080/compress',
        files=files,
        data=data,
        headers=headers
    )

    with open('compressed.mp4', 'wb') as out:
        out.write(response.content)
```

## Response Headers

When using API mode, you'll get useful metadata in the response headers:

- `X-Mode`: Final compression mode used
- `X-Encode-Duration-Ms`: Encoding time in milliseconds
- `X-Throughput-MBps`: Processing speed in MB/s
- `X-Input-Bytes`: Original file size
- `X-Output-Bytes`: Compressed file size
- `X-Resolution`: Output resolution
- `X-Video-Codec`: Video codec used
- `X-Audio-Codec`: Audio codec used
- `X-HW`: Hardware acceleration used

## Testing the API

You can test the API using the example in `examples/api-usage.go`:

```bash
cd go/examples
go run api-usage.go /path/to/your/video.mp4
```

## Summary

The key is to add the `Accept: application/octet-stream` header to your request. This tells the server you want the compressed file bytes instead of the HTML UI page.
