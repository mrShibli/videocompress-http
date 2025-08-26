package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ======================
// Logging Configuration
// ======================

var (
	logger *log.Logger
)

func init() {
	// Create a custom logger with timestamp and process info
	logger = log.New(os.Stdout, "", log.LstdFlags|log.Lmicroseconds)
	
	// Log startup information
	logger.Printf("🚀 VideoCompress API starting up...")
	logger.Printf("📁 Working directory: %s", getCurrentDir())
	logger.Printf("💾 Max upload size: %s", humanBytes(maxUploadSize))
	logger.Printf("🔧 FFmpeg available: %t", isFFmpegAvailable())
}

func getCurrentDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	return dir
}

func isFFmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

// ======================
// Config
// ======================

const (
	maxUploadSize = 2 << 30 // 2 GB
)

// ======================
// Helpers
// ======================

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func withExt(p, newExt string) string {
	base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
	return filepath.Join(filepath.Dir(p), base+newExt)
}

func randID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func humanBytes(n int64) string {
	const k = 1024.0
	f := float64(n)
	switch {
	case f >= k*k*k:
		return fmt.Sprintf("%.2f GB", f/(k*k*k))
	case f >= k*k:
		return fmt.Sprintf("%.2f MB", f/(k*k))
	case f >= k:
		return fmt.Sprintf("%.2f KB", f/k)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// ======================
// Encoding options + logic
// ======================

type compressOpts struct {
	Codec      string // h264|h265|copy
	CRF        int    // CPU encoders quality
	Preset     string // ultrafast..placebo (CPU encoders)
	Scale      string // e.g. 1280:-2 or 1920:1080 (fixed WxH). Leave empty to auto.
	FPS        int    // force output fps if >0
	Audio      string // aac|opus|copy
	AB         string // audio bitrate (e.g. 128k)
	HW         string // videotoolbox|none
	OutExt     string // .mp4 (recommended)
	SpeedMode  string // ultra_fast|super_fast|fast|balanced|quality|ai|max|turbo
	Resolution string // 360p|480p|720p|1080p|1440p|2160p|original
}

func (o *compressOpts) normalize() {
	if o.Codec == "" {
		o.Codec = "h264"
	}
	if o.Audio == "" {
		o.Audio = "aac"
	}
	if o.AB == "" {
		o.AB = "128k"
	}
	if o.HW == "" {
		o.HW = "none" // VPS-safe default
	}
	if o.OutExt == "" {
		o.OutExt = ".mp4"
	}
	if o.SpeedMode == "" {
		o.SpeedMode = "ai"
	}
	if o.Resolution == "" {
		o.Resolution = "original"
	}
	o.applyResolution()
}

func chooseSpeedBySize(sizeMB int64) string {
	switch {
	case sizeMB >= 700:
		return "ultra_fast"
	case sizeMB >= 200:
		return "super_fast"
	case sizeMB >= 50:
		return "fast"
	case sizeMB >= 10:
		return "balanced"
	default:
		return "balanced"
	}
}

// Apply speed profile → CRF/Preset/AB
func (o *compressOpts) applySpeedMode() {
	switch o.SpeedMode {
	case "turbo":
		// Soft-fast turbo: AAC stereo, orientation-safe 720p long-edge handled in buildFFmpegArgs
		o.CRF = 34
		o.Preset = "ultrafast"
		o.AB = "96k"
	case "max":
		// Very small/fast: orientation-safe 480p long-edge handled in buildFFmpegArgs
		o.CRF = 36
		o.Preset = "ultrafast"
		o.AB = "64k"
	case "ultra_fast":
		o.CRF = 32
		o.Preset = "ultrafast"
		o.AB = "96k"
	case "super_fast":
		o.CRF = 30
		o.Preset = "ultrafast"
		o.AB = "96k"
	case "fast":
		o.CRF = 28
		o.Preset = "veryfast"
		o.AB = "128k"
	case "quality":
		o.CRF = 23
		o.Preset = "fast"
		o.AB = "128k"
	default: // balanced
		if o.CRF == 0 {
			o.CRF = 26
		}
		if o.Preset == "" {
			o.Preset = "veryfast"
		}
	}
}

func (o *compressOpts) applyResolution() {
	switch o.Resolution {
	case "360p":
		o.Scale = "640:360"
	case "480p":
		o.Scale = "854:480"
	case "720p":
		o.Scale = "1280:720"
	case "1080p":
		o.Scale = "1920:1080"
	case "1440p":
		o.Scale = "2560:1440"
	case "2160p":
		o.Scale = "3840:2160"
	case "original":
		// keep original (Scale empty)
	default:
		// unknown -> keep original
	}
}

// Extra safety for very small inputs
func (o *compressOpts) tinyInputSafety(fileSize int64) {
	sizeMB := fileSize / (1024 * 1024)
	if sizeMB < 10 {
		o.Codec = "h264"
		o.Audio = "aac"
		o.Scale = ""
		o.CRF = 22
		o.Preset = "veryfast"
		o.HW = "none"
	}
}

// ffmpeg args (orientation‑aware for turbo/max)
func buildFFmpegArgs(inPath, outPath string, o compressOpts) []string {
	// Base flags; try HW decode on mac when enabled
	args := []string{"-y", "-hide_banner", "-loglevel", "error"}
	if strings.ToLower(o.HW) == "videotoolbox" {
		args = append(args, "-hwaccel", "videotoolbox", "-hwaccel_output_format", "videotoolbox")
	}
	args = append(args, "-i", inPath)

	// ---------------------------
	// ORIENTATION-SAFE SCALING
	// ---------------------------
	// For turbo/max we cap the LONG EDGE and preserve orientation:
	// a = iw/ih (aspect). Use -2 to keep even dimensions.
	//   turbo longEdge=720:  landscape -> h=720 (w auto), portrait -> w=720 (h auto)
	//   max   longEdge=480:  landscape -> h=480 (w auto), portrait -> w=480 (h auto)
	vf := ""
	if strings.ToLower(o.Codec) != "copy" {
		switch o.SpeedMode {
		case "turbo":
			if o.FPS == 0 {
				o.FPS = 24
			}
			vf = "scale='if(gt(a,1),-2,720)':'if(gt(a,1),720,-2)':flags=fast_bilinear,setsar=1"
		case "max":
			if o.FPS == 0 {
				o.FPS = 24
			}
			vf = "scale='if(gt(a,1),-2,480)':'if(gt(a,1),480,-2)':flags=fast_bilinear,setsar=1"
		default:
			// Respect explicit fixed WxH if provided (e.g. from Resolution),
			// otherwise don't add a scale filter.
			if o.Scale != "" {
				vf = "scale=" + o.Scale + ":flags=fast_bilinear,setsar=1"
			}
		}
	}
	if vf != "" {
		args = append(args, "-vf", vf)
	}

	// fps (only if re-encoding video)
	if o.FPS > 0 && strings.ToLower(o.Codec) != "copy" {
		args = append(args, "-r", strconv.Itoa(o.FPS))
	}

	// ---------------------------
	// VIDEO CODEC
	// ---------------------------
	vcodec := ""
	switch strings.ToLower(o.Codec) {
	case "copy":
		vcodec = "copy"
	case "h265":
		if strings.ToLower(o.HW) == "videotoolbox" {
			vcodec = "hevc_videotoolbox"
		} else {
			vcodec = "libx265"
		}
	default: // h264
		if strings.ToLower(o.HW) == "videotoolbox" {
			vcodec = "h264_videotoolbox"
		} else {
			vcodec = "libx264"
		}
	}

	if vcodec == "copy" {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", vcodec)

		switch vcodec {
		case "libx264", "libx265":
			args = append(args, "-crf", strconv.Itoa(o.CRF), "-preset", o.Preset)
		case "h264_videotoolbox", "hevc_videotoolbox":
			// map CRF→bitrate for hardware encoders
			bitrate := "3M"
			switch {
			case o.CRF <= 20:
				bitrate = "5M"
			case o.CRF <= 23:
				bitrate = "4M"
			case o.CRF <= 26:
				bitrate = "3M"
			case o.CRF <= 30:
				bitrate = "2.5M"
			case o.CRF <= 36:
				bitrate = "2M"
			default:
				bitrate = "1500k"
			}
			// Turbo a bit higher for decent 720p long-edge
			if o.SpeedMode == "turbo" {
				bitrate = "2500k"
			}
			args = append(args, "-b:v", bitrate)
		}

		// browser/player compatibility
		if strings.ToLower(o.OutExt) == ".mp4" {
			args = append(args, "-pix_fmt", "yuv420p")
		}
	}

	// Extra accelerations (zero-latency style) for turbo/max
	if o.SpeedMode == "max" || o.SpeedMode == "turbo" {
		switch vcodec {
		case "libx264":
			args = append(args, "-tune", "fastdecode,zerolatency")
			args = append(args, "-g", "300", "-keyint_min", "300")
			args = append(args, "-x264-params",
				"no-scenecut=1:ref=1:bframes=0:me=dia:subme=0:trellis=0:aq-mode=0:fast_pskip=1:sync-lookahead=0:rc-lookahead=0")
		case "libx265":
			args = append(args, "-tune", "fastdecode")
			args = append(args, "-g", "300", "-keyint_min", "300")
		case "h264_videotoolbox", "hevc_videotoolbox":
			args = append(args, "-realtime", "true")
			args = append(args, "-g", "300")
		}
	}

	// ---------------------------
	// AUDIO
	// ---------------------------
	switch strings.ToLower(o.Audio) {
	case "copy":
		args = append(args, "-c:a", "copy")
	case "opus":
		args = append(args, "-c:a", "libopus", "-b:a", o.AB)
	default:
		args = append(args, "-c:a", "aac", "-b:a", o.AB)
		// turbo: stereo 96k; max: mono 64k
		if o.SpeedMode == "turbo" {
			args = append(args, "-ac", "2")
			args = append(args, "-b:a", "96k")
		} else if o.SpeedMode == "max" {
			args = append(args, "-ac", "1")
			args = append(args, "-b:a", "64k")
		}
	}

	// faststart + threads
	args = append(args, "-movflags", "+faststart", "-threads", "0", outPath)
	return args
}

// run ffmpeg synchronously; if HW fails, retry CPU
func runFFmpeg(ctx context.Context, inPath, outPath string, o compressOpts, logWriter io.Writer) error {
	requestID := randID(6)
	logger.Printf("🔧 [%s] Starting FFmpeg compression", requestID)
	
	o.normalize()
	args := buildFFmpegArgs(inPath, outPath, o)
	
	logger.Printf("⚙️ [%s] FFmpeg command: ffmpeg %s", requestID, strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter
	
	logger.Printf("▶️ [%s] Executing FFmpeg with hardware: %s", requestID, o.HW)
	err := cmd.Run()
	if err == nil {
		logger.Printf("✅ [%s] FFmpeg compression completed successfully", requestID)
		return nil
	}

	logger.Printf("⚠️ [%s] FFmpeg failed: %v", requestID, err)
	
	if strings.Contains(strings.ToLower(o.HW), "videotoolbox") {
		logger.Printf("🔄 [%s] VideoToolbox failed; falling back to CPU", requestID)
		fmt.Fprintln(logWriter, "VideoToolbox failed; falling back to CPU.")
		o.HW = "none"
		args = buildFFmpegArgs(inPath, outPath, o)
		cmd = exec.CommandContext(ctx, "ffmpeg", args...)
		cmd.Stdout = logWriter
		cmd.Stderr = logWriter
		
		logger.Printf("🔄 [%s] Retrying FFmpeg with CPU only", requestID)
		err = cmd.Run()
		if err == nil {
			logger.Printf("✅ [%s] FFmpeg CPU fallback completed successfully", requestID)
		} else {
			logger.Printf("❌ [%s] FFmpeg CPU fallback also failed: %v", requestID, err)
		}
		return err
	}
	
	logger.Printf("❌ [%s] FFmpeg failed and no fallback available: %v", requestID, err)
	return err
}

// ======================
// Result Store (for UI flow)
// ======================

type resultEntry struct {
	FilePath    string
	ModeFinal   string
	ModeDecider string // "ai" or "manual"
	InputBytes  int64
	OutputBytes int64
	Resolution  string
	Codec       string
	Audio       string
	HW          string
	ElapsedMs   int64
	Throughput  float64 // MB/s
}

var (
	storeMu sync.Mutex
	store   = map[string]*resultEntry{}
)

// ======================
//
// HTTP layer
//
// ======================

var uploadTpl = template.Must(template.New("u").Parse(`
<!doctype html>
<meta charset="utf-8">
<title>Video Compress</title>
<style>
body{font-family:ui-sans-serif,system-ui;margin:40px;max-width:900px;line-height:1.6}
.form-group{margin:12px 0}
label{display:block;margin-bottom:6px;font-weight:600}
input,select{width:100%;padding:8px;border:1px solid #d1d5db;border-radius:6px}
button{background:#111827;color:#fff;border:0;padding:12px 20px;border-radius:8px;cursor:pointer}
button:hover{background:#0f172a}
details{margin:12px 0}
pre{background:#f3f4f6;padding:12px;border-radius:6px;overflow:auto}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:8px}
.card{border:1px solid #e5e7eb;border-radius:8px;padding:8px}
small{color:#6b7280}
.kv{display:grid;grid-template-columns:200px 1fr;gap:8px 16px}
kbd{background:#f3f4f6;border:1px solid #e5e7eb;border-radius:4px;padding:2px 6px}
</style>

<h1>Video Compress</h1>
<p style="text-align: center; margin-bottom: 20px;">
  <a href="/api-docs" style="color: #667eea; text-decoration: none; font-weight: 500;">📖 View API Documentation</a>
</p>

<form method="post" action="/compress" enctype="multipart/form-data">
  <input type="hidden" name="ui" value="1">
  <div class="form-group">
    <label>Video file</label>
    <input type="file" name="file" accept="video/*" required>
  </div>

  <div class="grid">
    <div class="card">
      <label>Mode</label>
      <select name="speed">
        <option value="ai" selected>AI (auto by size)</option>
        <option value="turbo">TURBO (very fast, 720p long-edge)</option>
        <option value="max">MAX (very fast, 480p long-edge)</option>
        <option value="ultra_fast">Ultra Fast</option>
        <option value="super_fast">Super Fast</option>
        <option value="fast">Fast</option>
        <option value="balanced">Balanced</option>
        <option value="quality">Quality</option>
      </select>
      <small>AI picks by file size only.</small>
    </div>
    <div class="card">
      <label>Resolution</label>
      <select name="resolution">
        <option value="original" selected>Original</option>
        <option value="360p">360p</option>
        <option value="480p">480p</option>
        <option value="720p">720p</option>
        <option value="1080p">1080p</option>
        <option value="1440p">1440p</option>
        <option value="2160p">2160p</option>
      </select>
    </div>
  </div>

  <details>
    <summary>Advanced</summary>
    <div class="grid">
      <div class="card">
        <label>Video codec</label>
        <select name="codec">
          <option value="h264" selected>H.264</option>
          <option value="h265">H.265/HEVC</option>
          <option value="copy">Copy video stream</option>
        </select>
      </div>
      <div class="card">
        <label>Hardware</label>
        <select name="hw">
          <option value="none" selected>CPU only</option>
          <option value="videotoolbox">macOS VideoToolbox</option>
        </select>
      </div>
      <div class="card">
        <label>Audio codec</label>
        <select name="audio">
          <option value="aac" selected>AAC</option>
          <option value="opus">Opus</option>
          <option value="copy">Copy audio</option>
        </select>
      </div>
      <div class="card">
        <label>Output extension</label>
        <select name="outExt">
          <option value=".mp4" selected>.mp4</option>
          <option value=".mov">.mov</option>
        </select>
      </div>
    </div>
  </details>

  <button type="submit">Compress</button>
</form>

<h3>cURL (API, returns file bytes + timing headers)</h3>
<pre>
curl -f -S -o out.mp4 \
  -H "Accept: application/octet-stream" \
  -F "file=@input.mp4" \
  -F "speed=ai" \
  http://localhost:8080/compress -D headers.txt

# headers include: X-Encode-Duration-Ms and X-Throughput-MBps
</pre>
`))

var resultTpl = template.Must(template.New("r").Parse(`
<!doctype html>
<meta charset="utf-8">
<title>Compression result</title>
<style>
body{font-family:ui-sans-serif,system-ui;margin:40px;max-width:900px;line-height:1.6}
h1{margin-top:0}
.kv{display:grid;grid-template-columns:240px 1fr;gap:8px 16px}
code{background:#f3f4f6;border-radius:4px;padding:2px 6px}
a.btn{display:inline-block;margin-top:16px;background:#111827;color:#fff;text-decoration:none;padding:12px 16px;border-radius:8px}
a.btn:hover{background:#0f172a}
pre{background:#f3f4f6;padding:12px;border-radius:6px;overflow:auto}
</style>

<h1>✅ Compression complete</h1>
<div class="kv">
  <div>Mode</div><div><code>{{.ModeFinal}}</code> <small>(decided by: {{.ModeDecider}})</small></div>
  <div>Time taken</div><div>{{printf "%.2f" .Seconds}} s</div>
  <div>Throughput</div><div>{{printf "%.2f" .Throughput}} MB/s</div>
  <div>Input size</div><div>{{.InputHuman}} ({{.InputBytes}} bytes)</div>
  <div>Output size</div><div>{{.OutputHuman}} ({{.OutputBytes}} bytes)</div>
  <div>Resolution</div><div>{{.Resolution}}</div>
  <div>Video codec</div><div>{{.Codec}}</div>
  <div>Audio codec</div><div>{{.Audio}}</div>
  <div>Hardware</div><div>{{.HW}}</div>
</div>

<a class="btn" href="/dl/{{.ID}}?name={{.SuggestName}}">⬇️ Download compressed file</a>

<h3>API example</h3>
<pre>
curl -f -S -o out.mp4 \
  -H "Accept: application/octet-stream" \
  -F "file=@input.mp4" \
  -F "speed={{.ModeDecider}}" \
  http://localhost:8080/compress \
  -D headers.txt

# Timing headers:
# X-Encode-Duration-Ms, X-Throughput-MBps
</pre>
`))

var apiDocsTpl = template.Must(template.New("api").Parse(`
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>VideoCompress API Documentation</title>
    <style>
        * {
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }

        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, sans-serif;
            line-height: 1.6;
            color: #333;
            background: #f8fafc;
        }

        .container {
            max-width: 1200px;
            margin: 0 auto;
            padding: 20px;
        }

        .header {
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 40px 0;
            margin-bottom: 30px;
            border-radius: 12px;
            text-align: center;
        }

        .header h1 {
            font-size: 2.5rem;
            margin-bottom: 10px;
            font-weight: 700;
        }

        .header p {
            font-size: 1.1rem;
            opacity: 0.9;
        }

        .stats {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 40px;
        }

        .stat-card {
            background: white;
            padding: 20px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
            text-align: center;
        }

        .stat-number {
            font-size: 2rem;
            font-weight: 700;
            color: #667eea;
            margin-bottom: 5px;
        }

        .stat-label {
            color: #666;
            font-size: 0.9rem;
        }

        .section {
            background: white;
            margin-bottom: 30px;
            border-radius: 12px;
            box-shadow: 0 2px 4px rgba(0, 0, 0, 0.1);
            overflow: hidden;
        }

        .section-header {
            background: #f8f9fa;
            padding: 20px;
            border-bottom: 1px solid #e9ecef;
        }

        .section-header h2 {
            color: #333;
            font-size: 1.5rem;
            margin-bottom: 5px;
        }

        .section-header p {
            color: #666;
            font-size: 0.95rem;
        }

        .endpoint {
            border-bottom: 1px solid #e9ecef;
            padding: 20px;
        }

        .endpoint:last-child {
            border-bottom: none;
        }

        .endpoint-header {
            display: flex;
            align-items: center;
            margin-bottom: 15px;
        }

        .method {
            padding: 4px 12px;
            border-radius: 6px;
            font-weight: 600;
            font-size: 0.85rem;
            margin-right: 15px;
            min-width: 80px;
            text-align: center;
        }

        .method.get {
            background: #d4edda;
            color: #155724;
        }

        .method.post {
            background: #d1ecf1;
            color: #0c5460;
        }

        .method.put {
            background: #fff3cd;
            color: #856404;
        }

        .method.delete {
            background: #f8d7da;
            color: #721c24;
        }

        .endpoint-path {
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            font-size: 1.1rem;
            color: #333;
            font-weight: 500;
        }

        .endpoint-description {
            color: #666;
            margin-bottom: 15px;
        }

        .parameters {
            margin-bottom: 20px;
        }

        .parameters h4 {
            color: #333;
            margin-bottom: 10px;
            font-size: 1rem;
        }

        .param-table {
            width: 100%;
            border-collapse: collapse;
            font-size: 0.9rem;
        }

        .param-table th,
        .param-table td {
            padding: 8px 12px;
            text-align: left;
            border-bottom: 1px solid #e9ecef;
        }

        .param-table th {
            background: #f8f9fa;
            font-weight: 600;
            color: #333;
        }

        .param-table td {
            color: #666;
        }

        .required {
            background: #f8d7da;
            color: #721c24;
            padding: 2px 6px;
            border-radius: 4px;
            font-size: 0.75rem;
            font-weight: 600;
        }

        .optional {
            background: #d4edda;
            color: #155724;
            padding: 2px 6px;
            border-radius: 4px;
            font-size: 0.75rem;
            font-weight: 600;
        }

        .example {
            background: #f8f9fa;
            border: 1px solid #e9ecef;
            border-radius: 6px;
            padding: 15px;
            margin: 15px 0;
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            font-size: 0.9rem;
            overflow-x: auto;
        }

        .example-header {
            font-weight: 600;
            color: #333;
            margin-bottom: 10px;
            font-size: 0.9rem;
        }

        .response-headers {
            background: #e3f2fd;
            border: 1px solid #bbdefb;
            border-radius: 6px;
            padding: 15px;
            margin: 15px 0;
        }

        .response-headers h4 {
            color: #1976d2;
            margin-bottom: 10px;
        }

        .header-table {
            width: 100%;
            border-collapse: collapse;
            font-size: 0.85rem;
        }

        .header-table th,
        .header-table td {
            padding: 6px 10px;
            text-align: left;
            border-bottom: 1px solid #bbdefb;
        }

        .header-table th {
            background: #f3f8ff;
            font-weight: 600;
            color: #1976d2;
        }

        .speed-modes {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 15px;
            margin: 20px 0;
        }

        .mode-card {
            background: #f8f9fa;
            border: 1px solid #e9ecef;
            border-radius: 8px;
            padding: 15px;
        }

        .mode-name {
            font-weight: 600;
            color: #333;
            margin-bottom: 5px;
        }

        .mode-details {
            font-size: 0.85rem;
            color: #666;
            margin-bottom: 8px;
        }

        .mode-description {
            font-size: 0.9rem;
            color: #333;
        }

        .try-it {
            background: #667eea;
            color: white;
            border: none;
            padding: 8px 16px;
            border-radius: 6px;
            cursor: pointer;
            font-size: 0.9rem;
            margin-top: 10px;
            transition: background 0.2s;
        }

        .try-it:hover {
            background: #5a6fd8;
        }

        .tabs {
            display: flex;
            border-bottom: 1px solid #e9ecef;
            margin-bottom: 20px;
        }

        .tab {
            padding: 10px 20px;
            cursor: pointer;
            border-bottom: 2px solid transparent;
            transition: all 0.2s;
        }

        .tab.active {
            border-bottom-color: #667eea;
            color: #667eea;
            font-weight: 600;
        }

        .tab-content {
            display: none;
        }

        .tab-content.active {
            display: block;
        }

        .code-block {
            background: #2d3748;
            color: #e2e8f0;
            border-radius: 6px;
            padding: 15px;
            font-family: 'Monaco', 'Menlo', 'Ubuntu Mono', monospace;
            font-size: 0.9rem;
            overflow-x: auto;
            margin: 10px 0;
        }

        .code-block .keyword {
            color: #ff6b6b;
        }

        .code-block .string {
            color: #51cf66;
        }

        .code-block .comment {
            color: #868e96;
        }

        .copy-btn {
            background: #6c757d;
            color: white;
            border: none;
            padding: 4px 8px;
            border-radius: 4px;
            cursor: pointer;
            font-size: 0.8rem;
            margin-left: 10px;
        }

        .copy-btn:hover {
            background: #5a6268;
        }

        .footer {
            text-align: center;
            padding: 40px 0;
            color: #666;
            font-size: 0.9rem;
        }

        @media (max-width: 768px) {
            .container {
                padding: 10px;
            }

            .header h1 {
                font-size: 2rem;
            }

            .stats {
                grid-template-columns: 1fr;
            }

            .speed-modes {
                grid-template-columns: 1fr;
            }
        }
    </style>
</head>

<body>
    <div class="container">
        <div class="header">
            <h1>🎬 VideoCompress API</h1>
            <p>High-performance video compression HTTP server with AI-powered speed selection</p>
        </div>

        <div class="stats">
            <div class="stat-card">
                <div class="stat-number">5</div>
                <div class="stat-label">API Endpoints</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">8</div>
                <div class="stat-label">Speed Modes</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">2GB</div>
                <div class="stat-label">Max File Size</div>
            </div>
            <div class="stat-card">
                <div class="stat-number">100%</div>
                <div class="stat-label">FFmpeg Compatible</div>
            </div>
        </div>

        <div class="section">
            <div class="section-header">
                <h2>🚀 Quick Start</h2>
                <p>Get started with the VideoCompress API in minutes</p>
            </div>
            <div class="endpoint">
                <div class="tabs">
                    <div class="tab active" onclick="switchTab('curl')">cURL</div>
                    <div class="tab" onclick="switchTab('go')">Go</div>
                    <div class="tab" onclick="switchTab('php')">PHP</div>
                    <div class="tab" onclick="switchTab('nodejs')">Node.js</div>
                    <div class="tab" onclick="switchTab('python')">Python</div>
                </div>

                <div class="tab-content active" id="curl">
                    <div class="example-header">Basic AI Compression:</div>
                    <div class="code-block">
curl -X POST \
-H "Accept: application/octet-stream" \
-F "file=@video.mp4" \
-F "speed=ai" \
-o compressed.mp4 \
http://localhost:8080/compress
                    </div>
                    <button class="copy-btn" onclick="copyCode(this)">Copy</button>
                </div>

                <div class="tab-content" id="go">
                    <div class="example-header">Go Example:</div>
                    <div class="code-block">
package main

import (
    "bytes"
    "fmt"
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

    out, err := os.Create("compressed.mp4")
    if err != nil {
        return err
    }
    defer out.Close()

    _, err = io.Copy(out, resp.Body)
    return err
}

func main() {
    err := compressVideo("input.mp4", "ai")
    if err != nil {
        fmt.Printf("Error: %v\n", err)
        return
    }
    fmt.Println("Video compressed successfully!")
}
                    </div>
                    <button class="copy-btn" onclick="copyCode(this)">Copy</button>
                </div>

                <div class="tab-content" id="php">
                    <div class="example-header">PHP Example:</div>
                    <div class="code-block">
&lt;?php
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
    'Accept: application/octet-stream'
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
?&gt;
                    </div>
                    <button class="copy-btn" onclick="copyCode(this)">Copy</button>
                </div>

                <div class="tab-content" id="nodejs">
                    <div class="example-header">Node.js Example:</div>
                    <div class="code-block">
const FormData = require('form-data');
const fs = require('fs');
const fetch = require('node-fetch');

const form = new FormData();
form.append('file', fs.createReadStream('video.mp4'));
form.append('speed', 'ai');

const response = await fetch('http://localhost:8080/compress', {
    method: 'POST',
    body: form,
    headers: { 'Accept': 'application/octet-stream' }
});

const fileStream = fs.createWriteStream('compressed.mp4');
response.body.pipe(fileStream);
                    </div>
                    <button class="copy-btn" onclick="copyCode(this)">Copy</button>
                </div>

                <div class="tab-content" id="python">
                    <div class="example-header">Python Example:</div>
                    <div class="code-block">
import requests

with open('video.mp4', 'rb') as f:
    files = {'file': f}
    data = {'speed': 'ai'}
    headers = {'Accept': 'application/octet-stream'}

    response = requests.post(
        'http://localhost:8080/compress',
        files=files,
        data=data,
        headers=headers
    )

    with open('compressed.mp4', 'wb') as out:
        out.write(response.content)
                    </div>
                    <button class="copy-btn" onclick="copyCode(this)">Copy</button>
                </div>
            </div>
        </div>

        <div class="section">
            <div class="section-header">
                <h2>⚡ Speed Modes</h2>
                <p>Choose the perfect compression mode for your needs</p>
            </div>
            <div class="endpoint">
                <div class="speed-modes">
                    <div class="mode-card">
                        <div class="mode-name">🤖 AI Mode</div>
                        <div class="mode-details">CRF: Auto | Preset: Auto | Audio: Auto</div>
                        <div class="mode-description">Automatically selects optimal settings based on file size</div>
                    </div>
                    <div class="mode-card">
                        <div class="mode-name">🚀 Turbo</div>
                        <div class="mode-details">CRF: 34 | Preset: ultrafast | Audio: 96k stereo</div>
                        <div class="mode-description">Very fast, 720p long-edge, perfect for quick previews</div>
                    </div>
                    <div class="mode-card">
                        <div class="mode-name">🗜️ Max</div>
                        <div class="mode-details">CRF: 36 | Preset: ultrafast | Audio: 64k mono</div>
                        <div class="mode-description">Maximum compression, 480p long-edge, smallest file size</div>
                    </div>
                    <div class="mode-card">
                        <div class="mode-name">⚡ Ultra Fast</div>
                        <div class="mode-details">CRF: 32 | Preset: ultrafast | Audio: 96k</div>
                        <div class="mode-description">Ultra fast compression for large files</div>
                    </div>
                    <div class="mode-card">
                        <div class="mode-name">🏃 Super Fast</div>
                        <div class="mode-details">CRF: 30 | Preset: ultrafast | Audio: 96k</div>
                        <div class="mode-description">Super fast compression with good quality</div>
                    </div>
                    <div class="mode-card">
                        <div class="mode-name">🏃‍♂️ Fast</div>
                        <div class="mode-details">CRF: 28 | Preset: veryfast | Audio: 128k</div>
                        <div class="mode-description">Fast compression with balanced quality</div>
                    </div>
                    <div class="mode-card">
                        <div class="mode-name">⚖️ Balanced</div>
                        <div class="mode-details">CRF: 26 | Preset: veryfast | Audio: 128k</div>
                        <div class="mode-description">Default choice, good balance of speed and quality</div>
                    </div>
                    <div class="mode-card">
                        <div class="mode-name">✨ Quality</div>
                        <div class="mode-details">CRF: 23 | Preset: fast | Audio: 128k</div>
                        <div class="mode-description">High quality compression for important videos</div>
                    </div>
                </div>
            </div>
        </div>

        <div class="section">
            <div class="section-header">
                <h2>🔌 API Endpoints</h2>
                <p>Complete API reference with examples and parameters</p>
            </div>

            <div class="endpoint">
                <div class="endpoint-header">
                    <span class="method get">GET</span>
                    <span class="endpoint-path">/health</span>
                </div>
                <div class="endpoint-description">Check server status and get available features</div>

                <div class="example">
                    <div class="example-header">Response:</div>
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
                </div>
                <button class="try-it" onclick="testEndpoint('GET', '/health')">Try it</button>
            </div>

            <div class="endpoint">
                <div class="endpoint-header">
                    <span class="method post">POST</span>
                    <span class="endpoint-path">/compress</span>
                </div>
                <div class="endpoint-description">Compress a video file with specified settings</div>

                <div class="parameters">
                    <h4>Parameters:</h4>
                    <table class="param-table">
                        <thead>
                            <tr>
                                <th>Parameter</th>
                                <th>Type</th>
                                <th>Required</th>
                                <th>Default</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td>file</td>
                                <td>File</td>
                                <td><span class="required">Required</span></td>
                                <td>-</td>
                                <td>Video file to compress</td>
                            </tr>
                            <tr>
                                <td>speed</td>
                                <td>String</td>
                                <td><span class="optional">Optional</span></td>
                                <td>ai</td>
                                <td>Compression speed mode</td>
                            </tr>
                            <tr>
                                <td>resolution</td>
                                <td>String</td>
                                <td><span class="optional">Optional</span></td>
                                <td>original</td>
                                <td>Output resolution</td>
                            </tr>
                            <tr>
                                <td>codec</td>
                                <td>String</td>
                                <td><span class="optional">Optional</span></td>
                                <td>h264</td>
                                <td>Video codec</td>
                            </tr>
                            <tr>
                                <td>audio</td>
                                <td>String</td>
                                <td><span class="optional">Optional</span></td>
                                <td>aac</td>
                                <td>Audio codec</td>
                            </tr>
                            <tr>
                                <td>hw</td>
                                <td>String</td>
                                <td><span class="optional">Optional</span></td>
                                <td>none</td>
                                <td>Hardware acceleration</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <div class="response-headers">
                    <h4>Response Headers (API Mode):</h4>
                    <table class="header-table">
                        <thead>
                            <tr>
                                <th>Header</th>
                                <th>Description</th>
                                <th>Example</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td>X-Mode</td>
                                <td>Final compression mode used</td>
                                <td>balanced</td>
                            </tr>
                            <tr>
                                <td>X-Encode-Duration-Ms</td>
                                <td>Encoding time in milliseconds</td>
                                <td>15000</td>
                            </tr>
                            <tr>
                                <td>X-Throughput-MBps</td>
                                <td>Processing speed in MB/s</td>
                                <td>25.5</td>
                            </tr>
                            <tr>
                                <td>X-Input-Bytes</td>
                                <td>Original file size</td>
                                <td>52428800</td>
                            </tr>
                            <tr>
                                <td>X-Output-Bytes</td>
                                <td>Compressed file size</td>
                                <td>15728640</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <div class="example">
                    <div class="example-header">Example Request:</div>
curl -X POST \
-H "Accept: application/octet-stream" \
-F "file=@video.mp4" \
-F "speed=balanced" \
-F "resolution=720p" \
-F "codec=h264" \
-F "hw=videotoolbox" \
-o compressed.mp4 \
http://localhost:8080/compress
                </div>
                <button class="try-it" onclick="testEndpoint('POST', '/compress')">Try it</button>
            </div>

            <div class="endpoint">
                <div class="endpoint-header">
                    <span class="method get">GET</span>
                    <span class="endpoint-path">/dl/{id}</span>
                </div>
                <div class="endpoint-description">Download a compressed file from the temporary store (web UI flow only)</div>

                <div class="parameters">
                    <h4>Parameters:</h4>
                    <table class="param-table">
                        <thead>
                            <tr>
                                <th>Parameter</th>
                                <th>Type</th>
                                <th>Required</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td>id</td>
                                <td>String</td>
                                <td><span class="required">Required</span></td>
                                <td>File ID from compression result</td>
                            </tr>
                            <tr>
                                <td>name</td>
                                <td>String</td>
                                <td><span class="optional">Optional</span></td>
                                <td>Custom filename for download</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>

            <div class="endpoint">
                <div class="endpoint-header">
                    <span class="method get">GET</span>
                    <span class="endpoint-path">/meta/{id}</span>
                </div>
                <div class="endpoint-description">Get metadata for a compressed file (web UI flow only)</div>

                <div class="example">
                    <div class="example-header">Response:</div>
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
                </div>
            </div>
        </div>

        <div class="section">
            <div class="section-header">
                <h2>🔧 Configuration Options</h2>
                <p>Available parameters and their values</p>
            </div>
            <div class="endpoint">
                <div class="parameters">
                    <h4>Resolution Options:</h4>
                    <table class="param-table">
                        <thead>
                            <tr>
                                <th>Value</th>
                                <th>Output Size</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td>original</td>
                                <td>Original</td>
                                <td>Keep original resolution</td>
                            </tr>
                            <tr>
                                <td>360p</td>
                                <td>640x360</td>
                                <td>360p resolution</td>
                            </tr>
                            <tr>
                                <td>480p</td>
                                <td>854x480</td>
                                <td>480p resolution</td>
                            </tr>
                            <tr>
                                <td>720p</td>
                                <td>1280x720</td>
                                <td>720p resolution</td>
                            </tr>
                            <tr>
                                <td>1080p</td>
                                <td>1920x1080</td>
                                <td>1080p resolution</td>
                            </tr>
                            <tr>
                                <td>1440p</td>
                                <td>2560x1440</td>
                                <td>1440p resolution</td>
                            </tr>
                            <tr>
                                <td>2160p</td>
                                <td>3840x2160</td>
                                <td>4K resolution</td>
                            </tr>
                        </tbody>
                    </table>
                </div>

                <div class="parameters">
                    <h4>Codec Options:</h4>
                    <table class="param-table">
                        <thead>
                            <tr>
                                <th>Parameter</th>
                                <th>Values</th>
                                <th>Description</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr>
                                <td>codec</td>
                                <td>h264, h265, copy</td>
                                <td>Video codec</td>
                            </tr>
                            <tr>
                                <td>audio</td>
                                <td>aac, opus, copy</td>
                                <td>Audio codec</td>
                            </tr>
                            <tr>
                                <td>hw</td>
                                <td>none, videotoolbox</td>
                                <td>Hardware acceleration</td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>

        <div class="section">
            <div class="section-header">
                <h2>🚨 Error Handling</h2>
                <p>Common error responses and troubleshooting</p>
            </div>
            <div class="endpoint">
                <div class="example">
                    <div class="example-header">400 Bad Request:</div>
{
    "error": "file field required"
}
                </div>

                <div class="example">
                    <div class="example-header">404 Not Found:</div>
{
    "error": "Not found"
}
                </div>

                <div class="example">
                    <div class="example-header">500 Internal Server Error:</div>
{
    "error": "compression failed: FFmpeg error message"
}
                </div>
            </div>
        </div>

        <div class="footer">
            <p>🎬 VideoCompress API v3.2.0-orientation | Built with Go & FFmpeg</p>
            <p>📚 <a href="/" style="color: #667eea;">Web Interface</a> | 📖 <a href="/api-docs" style="color: #667eea;">API Docs</a></p>
        </div>
    </div>

    <script>
        function switchTab(tabName) {
            // Hide all tab contents
            const tabContents = document.querySelectorAll('.tab-content');
            tabContents.forEach(content => content.classList.remove('active'));

            // Remove active class from all tabs
            const tabs = document.querySelectorAll('.tab');
            tabs.forEach(tab => tab.classList.remove('active'));

            // Show selected tab content
            document.getElementById(tabName).classList.add('active');

            // Add active class to clicked tab
            event.target.classList.add('active');
        }

        function copyCode(button) {
            const codeBlock = button.previousElementSibling;
            const text = codeBlock.textContent;

            navigator.clipboard.writeText(text).then(() => {
                const originalText = button.textContent;
                button.textContent = 'Copied!';
                button.style.background = '#28a745';

                setTimeout(() => {
                    button.textContent = originalText;
                    button.style.background = '#6c757d';
                }, 2000);
            });
        }

        function testEndpoint(method, path) {
            const baseUrl = window.location.origin;
            const url = baseUrl + path;

            if (method === 'GET') {
                window.open(url, '_blank');
            } else {
                alert('POST requests require a file upload. Please use the web interface or API tools like Postman.');
            }
        }

        // Add syntax highlighting
        document.addEventListener('DOMContentLoaded', function () {
            const codeBlocks = document.querySelectorAll('.code-block');
            codeBlocks.forEach(block => {
                let html = block.innerHTML;

                // Highlight keywords
                html = html.replace(/\b(curl|POST|GET|Accept|application\/octet-stream|file|speed|ai|balanced|resolution|codec|hw|videotoolbox|package|import|func|var|const|type|struct|interface|map|slice|chan|go|defer|if|else|for|range|switch|case|default|select|break|continue|return|panic|recover|go|select|chan|close|len|cap|new|make|append|copy|delete|complex|real|imag|iota|nil|true|false)\b/g, '<span class="keyword">$1</span>');

                // Highlight strings
                html = html.replace(/"([^"]*)"/g, '<span class="string">"$1"</span>');

                // Highlight comments
                html = html.replace(/(\/\/.*)/g, '<span class="comment">$1</span>');

                block.innerHTML = html;
            });
        });
    </script>
</body>
</html>
`))

func uploadPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = uploadTpl.Execute(w, nil)
}

// Cleanly save a multipart file to disk (kept for completeness)
func savePartToTemp(part *multipart.Part, suggested string) (string, error) {
	tmpDir := os.TempDir()
	name := filepath.Base(suggested)
	if name == "" || name == "." || name == "/" {
		name = "upload_" + randID(6)
	}
	dst := filepath.Join(tmpDir, name)
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	defer f.Close()
	_, err = io.Copy(f, part)
	return dst, err
}

// Parse options (after ParseMultipartForm)
func parseOpts(r *http.Request) (compressOpts, error) {
	o := compressOpts{}
	get := func(key, def string) string {
		if v := r.FormValue(key); v != "" {
			return v
		}
		return def
	}
	o.Codec = get("codec", "h264")
	o.Audio = get("audio", "aac")
	o.AB = get("ab", "")
	o.HW = get("hw", "none")
	o.OutExt = get("outExt", ".mp4")
	o.SpeedMode = get("speed", "ai")
	o.Resolution = get("resolution", "original")
	if fpsStr := get("fps", ""); fpsStr != "" {
		if n, err := strconv.Atoi(fpsStr); err == nil && n > 0 && n <= 60 {
			o.FPS = n
		}
	}
	o.normalize()
	return o, nil
}

func compressHandler(w http.ResponseWriter, r *http.Request) {
	requestID := randID(8)
	logger.Printf("📥 [%s] New compression request from %s", requestID, r.RemoteAddr)
	logger.Printf("📋 [%s] Method: %s, URL: %s", requestID, r.Method, r.URL.Path)
	
	switch r.Method {
	case http.MethodGet:
		logger.Printf("🌐 [%s] Serving upload page", requestID)
		uploadPage(w, r)
		return
	case http.MethodPost:
		logger.Printf("🎬 [%s] Processing compression request", requestID)
	default:
		logger.Printf("❌ [%s] Method not allowed: %s", requestID, r.Method)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logger.Printf("📝 [%s] Parsing multipart form data...", requestID)
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		logger.Printf("❌ [%s] Failed to parse multipart form: %v", requestID, err)
		http.Error(w, "expecting multipart/form-data: "+err.Error(), http.StatusBadRequest)
		return
	}
	logger.Printf("✅ [%s] Multipart form parsed successfully", requestID)

	logger.Printf("📁 [%s] Extracting uploaded file...", requestID)
	file, hdr, err := r.FormFile("file")
	if err != nil {
		logger.Printf("❌ [%s] File field not found: %v", requestID, err)
		http.Error(w, "file field required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	
	logger.Printf("📄 [%s] File received: %s (%s)", requestID, hdr.Filename, humanBytes(hdr.Size))

	// Save upload to temp file
	logger.Printf("💾 [%s] Saving uploaded file to temp directory...", requestID)
	inPath := filepath.Join(os.TempDir(), filepath.Base(hdr.Filename))
	logger.Printf("📂 [%s] Temp file path: %s", requestID, inPath)
	
	outf, err := os.Create(inPath)
	if err != nil {
		logger.Printf("❌ [%s] Failed to create temp file: %v", requestID, err)
		http.Error(w, "save error: "+err.Error(), 500)
		return
	}
	
	logger.Printf("📥 [%s] Copying file data to temp location...", requestID)
	if _, err := io.Copy(outf, file); err != nil {
		outf.Close()
		logger.Printf("❌ [%s] Failed to copy file data: %v", requestID, err)
		http.Error(w, "save error: "+err.Error(), 500)
		return
	}
	outf.Close()
	logger.Printf("✅ [%s] File saved to temp location successfully", requestID)
	defer func() {
		logger.Printf("🧹 [%s] Cleaning up temp file: %s", requestID, inPath)
		os.Remove(inPath)
	}()

	// Parse options
	logger.Printf("⚙️ [%s] Parsing compression options...", requestID)
	opts, err := parseOpts(r)
	if err != nil {
		logger.Printf("❌ [%s] Failed to parse options: %v", requestID, err)
		http.Error(w, err.Error(), 400)
		return
	}
	logger.Printf("✅ [%s] Options parsed: speed=%s, resolution=%s, codec=%s, audio=%s, hw=%s", 
		requestID, opts.SpeedMode, opts.Resolution, opts.Codec, opts.Audio, opts.HW)

	// File size
	logger.Printf("📊 [%s] Calculating file statistics...", requestID)
	st, _ := os.Stat(inPath)
	var sizeMB int64
	var inputBytes int64
	if st != nil {
		inputBytes = st.Size()
		sizeMB = st.Size() / (1024 * 1024)
		logger.Printf("📈 [%s] File size: %s (%d bytes, %d MB)", requestID, humanBytes(inputBytes), inputBytes, sizeMB)
	} else {
		logger.Printf("⚠️ [%s] Could not get file stats", requestID)
	}

	// Decide final mode if AI (size-only)
	logger.Printf("🤖 [%s] Processing speed mode decision...", requestID)
	modeDecider := "manual"
	if opts.SpeedMode == "ai" {
		modeDecider = "ai"
		base := chooseSpeedBySize(sizeMB)
		logger.Printf("🧠 [%s] AI selected base mode: %s (for %d MB file)", requestID, base, sizeMB)
		
		if sizeMB >= 200 && sizeMB < 2048 {
			if sizeMB <= 250 {
				base = "balanced"
				logger.Printf("⚖️ [%s] Adjusted to balanced mode for medium file", requestID)
			} else {
				base = "ultra_fast"
				logger.Printf("⚡ [%s] Adjusted to ultra_fast mode for large file", requestID)
			}
		}
		opts.SpeedMode = base
		logger.Printf("🎯 [%s] Final AI mode: %s", requestID, opts.SpeedMode)
	} else {
		logger.Printf("🎛️ [%s] Using manual mode: %s", requestID, opts.SpeedMode)
	}

	// Small-file safety
	logger.Printf("🛡️ [%s] Applying small-file safety checks...", requestID)
	opts.tinyInputSafety(inputBytes)
	logger.Printf("✅ [%s] Safety checks applied", requestID)

	// Apply profile params
	logger.Printf("⚙️ [%s] Applying speed profile parameters...", requestID)
	opts.applySpeedMode()
	logger.Printf("✅ [%s] Profile applied: CRF=%d, Preset=%s, AB=%s", requestID, opts.CRF, opts.Preset, opts.AB)

	outPath := withExt(inPath, "_compressed"+opts.OutExt)
	logger.Printf("🎬 [%s] Output path: %s", requestID, outPath)

	// --- timing starts here ---
	logger.Printf("⏱️ [%s] Starting compression process...", requestID)
	start := time.Now()

	// Run ffmpeg synchronously (no timeouts)
	logger.Printf("🔧 [%s] Executing FFmpeg compression...", requestID)
	ctx := r.Context()
	if err := runFFmpeg(ctx, inPath, outPath, opts, io.Discard); err != nil {
		logger.Printf("❌ [%s] FFmpeg compression failed: %v", requestID, err)
		http.Error(w, "compression failed: "+err.Error(), 500)
		return
	}

	elapsed := time.Since(start)
	elapsedMs := elapsed.Milliseconds()
	logger.Printf("✅ [%s] Compression completed in %d ms", requestID, elapsedMs)

	// validate output
	logger.Printf("🔍 [%s] Validating compressed output...", requestID)
	stat, err := os.Stat(outPath)
	if err != nil || stat.Size() < 1024 {
		logger.Printf("❌ [%s] Output validation failed: %v, size: %d", requestID, err, stat.Size())
		http.Error(w, "output seems empty or invalid", 500)
		return
	}
	outputBytes := stat.Size()
	logger.Printf("✅ [%s] Output validated: %s (%d bytes)", requestID, humanBytes(outputBytes), outputBytes)

	// throughput (MB/s) = input size / seconds
	logger.Printf("📊 [%s] Calculating compression statistics...", requestID)
	throughput := 0.0
	sec := elapsed.Seconds()
	if sec > 0 {
		throughput = (float64(inputBytes) / (1024 * 1024)) / sec
	}
	
	compressionRatio := float64(outputBytes) / float64(inputBytes) * 100
	logger.Printf("📈 [%s] Compression stats: %.2f MB/s throughput, %.1f%% size reduction", 
		requestID, throughput, 100-compressionRatio)

	// API MODE: Return compressed file bytes directly
	// To get file bytes instead of UI, use either:
	// 1. Set header: Accept: application/octet-stream
	// 2. Add parameter: api=1
	accept := r.Header.Get("Accept")
	apiParam := r.FormValue("api")
	
	logger.Printf("🎯 [%s] Determining response mode...", requestID)
	logger.Printf("📋 [%s] Accept header: %s", requestID, accept)
	logger.Printf("🔧 [%s] API parameter: %s", requestID, apiParam)
	
	if strings.Contains(accept, "application/octet-stream") || apiParam == "1" {
		logger.Printf("📤 [%s] API MODE: Returning compressed file directly", requestID)
		
		// add metadata headers
		w.Header().Set("X-Mode", opts.SpeedMode)
		w.Header().Set("X-Mode-Decider", modeDecider)
		w.Header().Set("X-Encode-Duration-Ms", fmt.Sprintf("%d", elapsedMs))
		w.Header().Set("X-Throughput-MBps", fmt.Sprintf("%.4f", throughput))
		w.Header().Set("X-Input-Bytes", fmt.Sprintf("%d", inputBytes))
		w.Header().Set("X-Output-Bytes", fmt.Sprintf("%d", outputBytes))
		w.Header().Set("X-Resolution", opts.Resolution)
		w.Header().Set("X-Video-Codec", opts.Codec)
		w.Header().Set("X-Audio-Codec", opts.Audio)
		w.Header().Set("X-HW", opts.HW)

		ctype := "application/octet-stream"
		switch strings.ToLower(filepath.Ext(outPath)) {
		case ".mp4":
			ctype = "video/mp4"
		case ".mov":
			ctype = "video/quicktime"
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(outPath)+"\"")
		
		logger.Printf("📤 [%s] Serving compressed file: %s (%s)", requestID, filepath.Base(outPath), ctype)
		http.ServeFile(w, r, outPath)
		logger.Printf("✅ [%s] API response completed successfully", requestID)
		return
	}

	// UI MODE: Show result page with download links
	logger.Printf("🌐 [%s] UI MODE: Preparing result page with download links", requestID)
	
	id := randID(12)
	entry := &resultEntry{
		FilePath:    outPath,
		ModeFinal:   opts.SpeedMode,
		ModeDecider: modeDecider,
		InputBytes:  inputBytes,
		OutputBytes: outputBytes,
		Resolution:  opts.Resolution,
		Codec:       opts.Codec,
		Audio:       opts.Audio,
		HW:          opts.HW,
		ElapsedMs:   elapsedMs,
		Throughput:  throughput,
	}
	
	logger.Printf("💾 [%s] Storing result entry with ID: %s", requestID, id)
	storeMu.Lock()
	store[id] = entry
	storeMu.Unlock()
	logger.Printf("✅ [%s] Result stored successfully", requestID)

	// Render result HTML
	logger.Printf("🎨 [%s] Rendering result HTML page...", requestID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{
		"ID":          id,
		"ModeFinal":   entry.ModeFinal,
		"ModeDecider": entry.ModeDecider,
		"InputBytes":  entry.InputBytes,
		"OutputBytes": entry.OutputBytes,
		"InputHuman":  humanBytes(entry.InputBytes),
		"OutputHuman": humanBytes(entry.OutputBytes),
		"Resolution":  entry.Resolution,
		"Codec":       entry.Codec,
		"Audio":       entry.Audio,
		"HW":          entry.HW,
		"SuggestName": filepath.Base(outPath),
		"Seconds":     float64(entry.ElapsedMs) / 1000.0,
		"Throughput":  entry.Throughput,
	}
	_ = resultTpl.Execute(w, data)
	logger.Printf("✅ [%s] UI response completed successfully", requestID)
}

func dlHandler(w http.ResponseWriter, r *http.Request) {
	requestID := randID(6)
	logger.Printf("📥 [%s] Download request from %s", requestID, r.RemoteAddr)
	
	id := strings.TrimPrefix(r.URL.Path, "/dl/")
	logger.Printf("🔍 [%s] Looking for file ID: %s", requestID, id)
	
	storeMu.Lock()
	e, ok := store[id]
	storeMu.Unlock()
	if !ok {
		logger.Printf("❌ [%s] File ID not found: %s", requestID, id)
		http.NotFound(w, r)
		return
	}
	
	logger.Printf("✅ [%s] File found: %s", requestID, e.FilePath)
	
	name := r.URL.Query().Get("name")
	if name == "" {
		name = filepath.Base(e.FilePath)
		logger.Printf("📄 [%s] Using default filename: %s", requestID, name)
	} else {
		logger.Printf("📄 [%s] Using custom filename: %s", requestID, name)
	}
	
	ctype := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(name)) {
	case ".mp4":
		ctype = "video/mp4"
	case ".mov":
		ctype = "video/quicktime"
	}
	
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	
	logger.Printf("📤 [%s] Serving file: %s (%s)", requestID, name, ctype)
	http.ServeFile(w, r, e.FilePath)
	logger.Printf("✅ [%s] Download completed successfully", requestID)
}

func metaHandler(w http.ResponseWriter, r *http.Request) {
	requestID := randID(6)
	logger.Printf("📥 [%s] Metadata request from %s", requestID, r.RemoteAddr)
	
	id := strings.TrimPrefix(r.URL.Path, "/meta/")
	logger.Printf("🔍 [%s] Looking for metadata for ID: %s", requestID, id)
	
	storeMu.Lock()
	e, ok := store[id]
	storeMu.Unlock()
	if !ok {
		logger.Printf("❌ [%s] File ID not found for metadata: %s", requestID, id)
		http.NotFound(w, r)
		return
	}
	
	logger.Printf("✅ [%s] Metadata found for file: %s", requestID, e.FilePath)
	
	w.Header().Set("Content-Type", "application/json")
	metadata := map[string]any{
		"id":                 id,
		"mode":               e.ModeFinal,
		"mode_decider":       e.ModeDecider,
		"input_bytes":        e.InputBytes,
		"output_bytes":       e.OutputBytes,
		"resolution":         e.Resolution,
		"codec":              e.Codec,
		"audio":              e.Audio,
		"hw":                 e.HW,
		"encode_duration_ms": e.ElapsedMs,
		"throughput_mb_s":    e.Throughput,
	}
	_ = json.NewEncoder(w).Encode(metadata)
	logger.Printf("✅ [%s] Metadata response sent successfully", requestID)
}

func health(w http.ResponseWriter, r *http.Request) {
	requestID := randID(6)
	logger.Printf("🏥 [%s] Health check request from %s", requestID, r.RemoteAddr)
	
	w.Header().Set("Content-Type", "application/json")
	healthData := map[string]any{
		"ok":        true,
		"service":   "videocompress",
		"version":   "3.2.0-orientation",
		"modes":     []string{"ai", "turbo", "max", "ultra_fast", "super_fast", "fast", "balanced", "quality"},
		"defaults":  map[string]any{"codec": "h264", "resolution": "original", "hw": "none"},
		"ui_routes": []string{"/", "/compress (POST)", "/dl/{id}", "/meta/{id}"},
	}
	_ = json.NewEncoder(w).Encode(healthData)
	logger.Printf("✅ [%s] Health check response sent", requestID)
}

func main() {
	addr := envOr("PORT", "8080")
	logger.Printf("🌐 [MAIN] Starting VideoCompress server on port %s", addr)

	mux := http.NewServeMux()
	mux.HandleFunc("/", uploadPage)
	mux.HandleFunc("/compress", compressHandler)
	mux.HandleFunc("/dl/", dlHandler)     // GET /dl/{id}?name=...
	mux.HandleFunc("/meta/", metaHandler) // GET /meta/{id}
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/api-docs", func(w http.ResponseWriter, r *http.Request) {
		requestID := randID(6)
		logger.Printf("📚 [%s] API docs request from %s", requestID, r.RemoteAddr)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = apiDocsTpl.Execute(w, nil)
		logger.Printf("✅ [%s] API docs served successfully", requestID)
	})

	s := &http.Server{
		Addr:    ":" + addr,
		Handler: logMiddleware(mux),
	}

	logger.Printf("🚀 [MAIN] VideoCompress server listening on http://localhost:%s", addr)
	logger.Printf("📖 [MAIN] API Documentation: http://localhost:%s/api-docs", addr)
	logger.Printf("🌐 [MAIN] Web Interface: http://localhost:%s", addr)
	logger.Printf("🏥 [MAIN] Health Check: http://localhost:%s/health", addr)
	
	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Printf("💥 [MAIN] Server error: %v", err)
		log.Fatal(err)
	}
}

// enhanced request logger with timing and status
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Create a custom response writer to capture status code
		statusWriter := &statusResponseWriter{ResponseWriter: w, statusCode: 200}
		
		next.ServeHTTP(statusWriter, r)
		
		elapsed := time.Since(start)
		logger.Printf("📊 [HTTP] %s %s - %d - %s - %v", 
			r.Method, r.URL.Path, statusWriter.statusCode, r.RemoteAddr, elapsed)
	})
}

// Custom response writer to capture status code
type statusResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}
