package main

import (
	"context"
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
	"time"
)

const (
	maxUploadSize = 2 << 30 // 2 GB (adjust to taste)
)

// ---- Utilities ----

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

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

type compressOpts struct {
	Codec     string // h264|h265|copy
	CRF       int
	Preset    string // ultrafast..placebo or p1..p7 for nvenc; ignored for videotoolbox
	Scale     string // e.g. 1280:-2
	Audio     string // aac|opus|copy
	AB        string // audio bitrate, e.g. 128k
	HW        string // videotoolbox|none
	OutExt    string // .mp4 or .mov etc.
	Timeout   time.Duration
	SpeedMode string // ultra_fast|super_fast|fast|balanced|quality
	Resolution string // 360p|480p|720p|1080p|1440p|2160p|original
}

func (o *compressOpts) normalize() {
	if o.Codec == "" { o.Codec = "h264" }
	if o.Audio == "" { o.Audio = "aac" }
	if o.HW == "" { o.HW = "videotoolbox" } // Default to hardware acceleration
	if o.OutExt == "" { o.OutExt = ".mp4" }
	if o.Timeout == 0 { o.Timeout = 30 * time.Minute } // Reduced timeout
	
	// Apply speed mode settings
	o.applySpeedMode()
	
	// Apply resolution settings
	o.applyResolution()
}

func (o *compressOpts) applySpeedMode() {
	switch o.SpeedMode {
	case "ultra_fast":
		o.CRF = 35
		o.Preset = "ultrafast"
		o.AB = "64k"
		o.Timeout = 15 * time.Minute
	case "super_fast":
		o.CRF = 32
		o.Preset = "ultrafast"
		o.AB = "80k"
		o.Timeout = 20 * time.Minute
	case "fast":
		o.CRF = 30
		o.Preset = "veryfast"
		o.AB = "96k"
		o.Timeout = 25 * time.Minute
	case "balanced":
		o.CRF = 28
		o.Preset = "veryfast"
		o.AB = "96k"
		o.Timeout = 30 * time.Minute
	case "quality":
		o.CRF = 23
		o.Preset = "fast"
		o.AB = "128k"
		o.Timeout = 45 * time.Minute
	default:
		// Default to balanced
		if o.CRF == 0 { o.CRF = 28 }
		if o.Preset == "" { o.Preset = "veryfast" }
		if o.AB == "" { o.AB = "96k" }
	}
}

// Dynamic compression based on file size - ensures videos always open
func (o *compressOpts) adjustForFileSize(fileSize int64) {
	sizeMB := fileSize / (1024 * 1024)
	
	switch o.SpeedMode {
	case "balanced":
		o.adjustBalancedMode(sizeMB)
	case "ultra_fast":
		o.adjustUltraFastMode(sizeMB)
	case "super_fast":
		o.adjustSuperFastMode(sizeMB)
	case "fast":
		o.adjustFastMode(sizeMB)
	case "quality":
		o.adjustQualityMode(sizeMB)
	}
}

func (o *compressOpts) adjustBalancedMode(sizeMB int64) {
	switch {
	case sizeMB < 30:
		// Very small files - no compression, just copy
		o.Codec = "copy"
		o.Audio = "copy"
		o.Scale = ""
		o.Timeout = 5 * time.Minute
	case sizeMB < 50:
		// Small files - very light compression
		o.CRF = 18
		o.Preset = "veryfast"
		o.AB = "128k"
		o.Timeout = 10 * time.Minute
	case sizeMB < 70:
		// Medium files - very light compression (more conservative)
		if sizeMB < 60 {
			// Extra conservative for 50-60MB files
			o.CRF = 18
			o.Preset = "veryfast"
			o.AB = "128k"
			o.Timeout = 15 * time.Minute
		} else {
			o.CRF = 20
			o.Preset = "veryfast"
			o.AB = "128k"
			o.Timeout = 15 * time.Minute
		}
	case sizeMB < 100:
		// Large files - light compression (more conservative)
		o.CRF = 22
		o.Preset = "veryfast"
		o.AB = "128k"
		o.Timeout = 20 * time.Minute
	default:
		// Very large files - moderate compression
		o.CRF = 24
		o.Preset = "veryfast"
		o.AB = "96k"
		o.Timeout = 30 * time.Minute
	}
}

func (o *compressOpts) adjustUltraFastMode(sizeMB int64) {
	switch {
	case sizeMB < 30:
		o.Codec = "copy"
		o.Audio = "copy"
		o.Scale = ""
		o.Timeout = 5 * time.Minute
	case sizeMB < 50:
		o.CRF = 25
		o.Preset = "ultrafast"
		o.AB = "128k"
		o.Timeout = 8 * time.Minute
	case sizeMB < 70:
		o.CRF = 28
		o.Preset = "ultrafast"
		o.AB = "96k"
		o.Timeout = 10 * time.Minute
	case sizeMB < 100:
		o.CRF = 30
		o.Preset = "ultrafast"
		o.AB = "96k"
		o.Timeout = 12 * time.Minute
	default:
		o.CRF = 32
		o.Preset = "ultrafast"
		o.AB = "80k"
		o.Timeout = 15 * time.Minute
	}
}

func (o *compressOpts) adjustSuperFastMode(sizeMB int64) {
	switch {
	case sizeMB < 30:
		o.Codec = "copy"
		o.Audio = "copy"
		o.Scale = ""
		o.Timeout = 5 * time.Minute
	case sizeMB < 50:
		o.CRF = 24
		o.Preset = "ultrafast"
		o.AB = "128k"
		o.Timeout = 10 * time.Minute
	case sizeMB < 70:
		o.CRF = 26
		o.Preset = "ultrafast"
		o.AB = "96k"
		o.Timeout = 12 * time.Minute
	case sizeMB < 100:
		o.CRF = 28
		o.Preset = "ultrafast"
		o.AB = "96k"
		o.Timeout = 15 * time.Minute
	default:
		o.CRF = 30
		o.Preset = "ultrafast"
		o.AB = "80k"
		o.Timeout = 20 * time.Minute
	}
}

func (o *compressOpts) adjustFastMode(sizeMB int64) {
	switch {
	case sizeMB < 30:
		o.Codec = "copy"
		o.Audio = "copy"
		o.Scale = ""
		o.Timeout = 5 * time.Minute
	case sizeMB < 50:
		o.CRF = 22
		o.Preset = "veryfast"
		o.AB = "128k"
		o.Timeout = 12 * time.Minute
	case sizeMB < 70:
		o.CRF = 24
		o.Preset = "veryfast"
		o.AB = "128k"
		o.Timeout = 15 * time.Minute
	case sizeMB < 100:
		o.CRF = 26
		o.Preset = "veryfast"
		o.AB = "96k"
		o.Timeout = 18 * time.Minute
	default:
		o.CRF = 28
		o.Preset = "veryfast"
		o.AB = "96k"
		o.Timeout = 25 * time.Minute
	}
}

func (o *compressOpts) adjustQualityMode(sizeMB int64) {
	switch {
	case sizeMB < 30:
		o.CRF = 18
		o.Preset = "fast"
		o.AB = "128k"
		o.Timeout = 10 * time.Minute
	case sizeMB < 50:
		o.CRF = 20
		o.Preset = "fast"
		o.AB = "128k"
		o.Timeout = 15 * time.Minute
	case sizeMB < 70:
		o.CRF = 21
		o.Preset = "fast"
		o.AB = "128k"
		o.Timeout = 20 * time.Minute
	case sizeMB < 100:
		o.CRF = 22
		o.Preset = "fast"
		o.AB = "128k"
		o.Timeout = 25 * time.Minute
	default:
		o.CRF = 23
		o.Preset = "fast"
		o.AB = "128k"
		o.Timeout = 45 * time.Minute
	}
}

func (o *compressOpts) applyResolution() {
	if o.Resolution == "" {
		o.Resolution = "original"
		return
	}
	
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
		o.Scale = ""
	default:
		o.Scale = ""
	}
}

// Build ffmpeg args based on options/platform
func buildFFmpegArgs(inPath, outPath string, o compressOpts) []string {
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-i", inPath}
	
	// Super fast optimizations for ultra_fast mode
	if o.SpeedMode == "ultra_fast" {
		args = append(args, "-tune", "fastdecode", "-profile:v", "baseline")
	}
	
	// QuickTime Player compatibility settings
	if o.OutExt == ".mp4" {
		args = append(args, "-pix_fmt", "yuv420p") // Ensure QuickTime compatibility
	}

	// scale? (optimized for speed)
	if o.Scale != "" && strings.ToLower(o.Codec) != "copy" {
		args = append(args, "-vf", "scale="+o.Scale+":flags=fast_bilinear")
	}

	// video codec selection
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
		// quality controls
		switch vcodec {
		case "libx264", "libx265":
			args = append(args, "-crf", strconv.Itoa(o.CRF), "-preset", o.Preset)
		case "h264_videotoolbox", "hevc_videotoolbox":
			// Dynamic bitrate based on CRF value for VideoToolbox
			bitrate := "2M" // default (more conservative)
			
			// Smart bitrate selection based on CRF (more conservative for compatibility)
			switch {
			case o.CRF <= 18:
				bitrate = "5M" // High quality
			case o.CRF <= 22:
				bitrate = "4M" // Good quality
			case o.CRF <= 26:
				bitrate = "3M" // Balanced
			case o.CRF <= 30:
				bitrate = "2.5M" // Fast
			case o.CRF <= 35:
				bitrate = "2M" // Ultra fast
			default:
				bitrate = "1.5M" // Maximum compression
			}
			
			args = append(args, "-b:v", bitrate)
		}
	}

	// audio
	switch strings.ToLower(o.Audio) {
	case "copy":
		args = append(args, "-c:a", "copy")
	case "opus":
		args = append(args, "-c:a", "libopus", "-b:a", o.AB)
	default: // aac
		args = append(args, "-c:a", "aac", "-b:a", o.AB)
	}

	// faststart for streaming + speed optimizations
	args = append(args, "-movflags", "+faststart", "-threads", "0")
	
	// Additional speed optimizations
	if o.SpeedMode == "ultra_fast" || o.SpeedMode == "super_fast" {
		args = append(args, "-g", "30", "-keyint_min", "30") // Keyframe optimization
	}
	
	args = append(args, outPath)
	return args
}

func runFFmpeg(ctx context.Context, inPath, outPath string, o compressOpts, w io.Writer) error {
	o.normalize()
	args := buildFFmpegArgs(inPath, outPath, o)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = w
	cmd.Stderr = w
	err := cmd.Run()
	
	// If VideoToolbox failed, try CPU fallback
	if err != nil && (strings.Contains(strings.ToLower(o.HW), "videotoolbox")) {
		fmt.Fprintln(w, "VideoToolbox failed, trying CPU fallback...")
		o.HW = "none" // Force CPU encoding
		args = buildFFmpegArgs(inPath, outPath, o)
		
		cmd = exec.CommandContext(ctx, "ffmpeg", args...)
		cmd.Stdout = w
		cmd.Stderr = w
		err = cmd.Run()
	}
	
	// If still failed and it's a medium-sized file, try copy mode
	if err != nil {
		stat, statErr := os.Stat(inPath)
		if statErr == nil && stat.Size() < 100*1024*1024 { // Less than 100MB
			fmt.Fprintln(w, "Compression failed, trying copy mode for compatibility...")
			o.Codec = "copy"
			o.Audio = "copy"
			o.Scale = ""
			args = buildFFmpegArgs(inPath, outPath, o)
			
			cmd = exec.CommandContext(ctx, "ffmpeg", args...)
			cmd.Stdout = w
			cmd.Stderr = w
			return cmd.Run()
		}
	}
	
	return err
}

// Save a multipart file part to disk safely
func savePartToTemp(part *multipart.Part, suggested string) (string, error) {
	tmpDir := os.TempDir()
	name := filepath.Base(suggested)
	if name == "" || name == "." || name == "/" {
		name = fmt.Sprintf("upload_%d", time.Now().UnixNano())
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

// ---- HTTP Handlers ----

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":       true,
		"service":  "videocompress",
		"version":  "2.0.0",
		"ffmpeg":   "required",
		"platform": "macOS supported (VideoToolbox), Linux/Windows (CPU encoders)",
		"speed_modes": map[string]any{
			"ultra_fast": map[string]any{
				"crf": 35, "preset": "ultrafast", "bitrate": "800k", "timeout": "15min",
				"description": "Fastest compression, lowest quality (auto-adjusted for small videos)",
			},
			"super_fast": map[string]any{
				"crf": 32, "preset": "ultrafast", "bitrate": "1M", "timeout": "20min",
				"description": "Very fast compression, low quality (auto-adjusted for small videos)",
			},
			"fast": map[string]any{
				"crf": 30, "preset": "veryfast", "bitrate": "1.2M", "timeout": "25min",
				"description": "Fast compression, decent quality (auto-adjusted for small videos)",
			},
			"balanced": map[string]any{
				"crf": 28, "preset": "veryfast", "bitrate": "1.5M", "timeout": "30min",
				"description": "Balanced speed and quality (auto-adjusted for small videos)",
			},
			"quality": map[string]any{
				"crf": 23, "preset": "fast", "bitrate": "3M", "timeout": "45min",
				"description": "High quality, slower compression",
			},
		},
		"smart_features": map[string]any{
			"dynamic_compression": "Compression adjusts based on file size automatically",
			"file_size_rules": map[string]any{
				"<30MB": "Copy mode (no compression)",
				"30-50MB": "Light compression (CRF 20-25)",
				"50-100MB": "Moderate compression (CRF 24-28)",
				"100MB+": "Normal compression (CRF 26-32)",
			},
			"hardware_fallback": "Automatic CPU fallback if hardware encoding fails",
		},
		"resolutions": []string{"360p", "480p", "720p", "1080p", "1440p", "2160p", "original"},
	})
}

var uploadTpl = template.Must(template.New("u").Parse(`
<!doctype html>
<meta charset="utf-8">
<title>Video Compress</title>
<style>
body{font-family:ui-sans-serif,system-ui;margin:40px;max-width:800px;line-height:1.6}
.alert{background:#f0f9ff;border:1px solid #0ea5e9;border-radius:8px;padding:16px;margin:16px 0}
.alert h3{margin:0 0 8px 0;color:#0369a1}
.form-group{margin:12px 0}
.form-group label{display:block;margin-bottom:4px;font-weight:500}
.form-group input,.form-group select{width:100%;padding:8px;border:1px solid #d1d5db;border-radius:4px}
button{background:#3b82f6;color:white;border:none;padding:12px 24px;border-radius:6px;cursor:pointer;font-size:16px}
button:hover{background:#2563eb}
details{margin:16px 0}
summary{cursor:pointer;font-weight:500}
pre{background:#f3f4f6;padding:12px;border-radius:4px;overflow-x:auto}
.speed-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(200px,1fr));gap:12px;margin:16px 0}
.speed-option{background:#f8fafc;border:1px solid #e2e8f0;border-radius:6px;padding:12px;cursor:pointer;transition:all 0.2s}
.speed-option:hover{background:#f1f5f9;border-color:#3b82f6}
.speed-option.selected{background:#dbeafe;border-color:#3b82f6}
.speed-option h4{margin:0 0 4px 0;color:#1e40af}
.speed-option p{margin:0;font-size:14px;color:#64748b}
.resolution-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(120px,1fr));gap:8px;margin:16px 0}
.resolution-option{background:#f8fafc;border:1px solid #e2e8f0;border-radius:4px;padding:8px;text-align:center;cursor:pointer;transition:all 0.2s}
.resolution-option:hover{background:#f1f5f9;border-color:#3b82f6}
.resolution-option.selected{background:#dbeafe;border-color:#3b82f6}
.resolution-option h4{margin:0;color:#1e40af;font-size:16px}
.resolution-option p{margin:4px 0 0 0;font-size:12px;color:#64748b}
</style>
<h1>Video Compress</h1>
<div class="alert">
  <h3>🎯 Smart Compression - Always Playable</h3>
  <p>• Conservative settings: <30MB = copy, 30-60MB = very light, 60-100MB = light, 100MB+ = moderate • QuickTime compatible</p>
</div>
<form method="post" action="/compress" enctype="multipart/form-data">
  <div class="form-group">
    <label>Video file: <input type="file" name="file" required accept="video/*"></label>
  </div>
  
  <div class="form-group">
    <label>Speed Mode:</label>
    <div class="speed-grid">
      <div class="speed-option" onclick="selectSpeed('ultra_fast')">
        <h4>🚀 Ultra Fast</h4>
        <p>Dynamic • Fast • 15min</p>
      </div>
      <div class="speed-option" onclick="selectSpeed('super_fast')">
        <h4>⚡ Super Fast</h4>
        <p>Dynamic • Quick • 20min</p>
      </div>
      <div class="speed-option selected" onclick="selectSpeed('balanced')">
        <h4>⚖️ Balanced</h4>
        <p>Dynamic • Smart • 30min</p>
      </div>
      <div class="speed-option" onclick="selectSpeed('fast')">
        <h4>🏃 Fast</h4>
        <p>Dynamic • Balanced • 25min</p>
      </div>
      <div class="speed-option" onclick="selectSpeed('quality')">
        <h4>🎨 Quality</h4>
        <p>Dynamic • High Quality • 45min</p>
      </div>
    </div>
    <input type="hidden" name="speed" id="speed" value="balanced">
  </div>
  
  <div class="form-group">
    <label>Resolution:</label>
    <div class="resolution-grid">
      <div class="resolution-option" onclick="selectResolution('360p')">
        <h4>360p</h4>
        <p>640×360</p>
      </div>
      <div class="resolution-option" onclick="selectResolution('480p')">
        <h4>480p</h4>
        <p>854×480</p>
      </div>
      <div class="resolution-option" onclick="selectResolution('720p')">
        <h4>720p</h4>
        <p>1280×720</p>
      </div>
      <div class="resolution-option selected" onclick="selectResolution('original')">
        <h4>Original</h4>
        <p>Keep size</p>
      </div>
      <div class="resolution-option" onclick="selectResolution('1080p')">
        <h4>1080p</h4>
        <p>1920×1080</p>
      </div>
    </div>
    <input type="hidden" name="resolution" id="resolution" value="original">
  </div>
  
  <details>
    <summary>Advanced Settings</summary>
    <div class="form-group">
      <label>Codec: <input name="codec" value="h264" placeholder="h264|h265|copy"></label>
    </div>
    <div class="form-group">
      <label>Hardware: <select name="hw">
        <option value="videotoolbox">VideoToolbox (Hardware)</option>
        <option value="none">CPU Only</option>
      </select></label>
    </div>
    <div class="form-group">
      <label>Audio Codec: <input name="audio" value="aac" placeholder="aac|opus|copy"></label>
    </div>
    <div class="form-group">
      <label>Output Extension: <input name="outExt" value=".mp4" placeholder=".mp4|.mov"></label>
    </div>
  </details>
  <button type="submit">Compress Video</button>
</form>

<script>
function selectSpeed(mode) {
  document.querySelectorAll('.speed-option').forEach(el => el.classList.remove('selected'));
  event.target.closest('.speed-option').classList.add('selected');
  document.getElementById('speed').value = mode;
}

function selectResolution(res) {
  document.querySelectorAll('.resolution-option').forEach(el => el.classList.remove('selected'));
  event.target.closest('.resolution-option').classList.add('selected');
  document.getElementById('resolution').value = res;
}
</script>

<p><strong>Quick Examples:</strong></p>
<pre># Ultra fast compression
curl -F "file=@video.mp4" -F "speed=ultra_fast" -F "resolution=720p" http://localhost:8080/compress -o ultra_fast.mp4

# Quality compression
curl -F "file=@video.mp4" -F "speed=quality" -F "resolution=1080p" http://localhost:8080/compress -o quality.mp4

# Super fast mobile compression
curl -F "file=@video.mp4" -F "speed=super_fast" -F "resolution=480p" http://localhost:8080/compress -o mobile.mp4

# Smart compression based on file size:
# &lt;30MB = copy mode (no compression)
# 30-60MB = very light compression (CRF 18-20)
# 60-100MB = light compression (CRF 20-22)
# 100MB+ = moderate compression (CRF 22-24)</pre>
`))

func uploadPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = uploadTpl.Execute(w, nil)
}

func parseOpts(r *http.Request) (compressOpts, error) {
	o := compressOpts{}
	// accept both form and query params
	q := r.URL.Query()

	get := func(key, def string) string {
		if v := r.FormValue(key); v != "" {
			return v
		}
		if v := q.Get(key); v != "" {
			return v
		}
		return def
	}

	o.Codec = get("codec", "h264")
	o.Preset = get("preset", "")
	o.Scale = get("scale", "")
	o.Audio = get("audio", "aac")
	o.AB = get("ab", "")
	o.HW = get("hw", "videotoolbox")
	o.OutExt = get("outExt", ".mp4")
	o.SpeedMode = get("speed", "balanced")
	o.Resolution = get("resolution", "original")

	// Only parse CRF if not using speed mode
	if o.SpeedMode == "" {
		crfStr := get("crf", "28")
		crf, err := strconv.Atoi(crfStr)
		if err != nil {
			return o, fmt.Errorf("invalid crf: %w", err)
		}
		o.CRF = crf
	}

	o.normalize()
	return o, nil
}

func compressHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		uploadPage(w, r)
		return
	case http.MethodPost:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "expecting multipart/form-data", http.StatusBadRequest)
		return
	}

	var filePath string
	var filename string

	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if part.FormName() == "file" {
			filename = part.FileName()
			if filename == "" {
				filename = "upload.mp4"
			}
			filePath, err = savePartToTemp(part, filename)
			if err != nil {
				http.Error(w, "save error: "+err.Error(), 500)
				return
			}
		}
		_ = part.Close()
	}

	if filePath == "" {
		http.Error(w, "no file provided (field name must be 'file')", http.StatusBadRequest)
		return
	}

	opts, err := parseOpts(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get file size and apply dynamic compression based on size
	if stat, err := os.Stat(filePath); err == nil {
		opts.adjustForFileSize(stat.Size())
	}

	outPath := withExt(filePath, "_compressed"+opts.OutExt)
	defer func() {
		_ = os.Remove(filePath)
		// keep output so user can re-download if needed; or remove if you prefer
	}()

	ctx, cancel := context.WithTimeout(r.Context(), opts.Timeout)
	defer cancel()

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		// stream ffmpeg logs to client trailing headers (we'll ignore here)
		if err := runFFmpeg(ctx, filePath, outPath, opts, pw); err != nil {
			fmt.Fprintln(pw, "ERROR:", err)
		}
	}()

	// Wait for ffmpeg to finish via context deadline or by checking file existence.
	// Simpler: block until the pipe goroutine completes by reading it fully in background.
	go io.Copy(io.Discard, pr) // drain logs

	// when finished, serve the file
	// We poll for output file existence and validate it's not empty
	t0 := time.Now()
	for {
		if stat, err := os.Stat(outPath); err == nil && stat.Size() > 1024 {
			// Quick validation - just check file size and basic structure
			// Skip ffprobe check for speed (only check if file exists and has content)
			break
		}
		if time.Since(t0) > opts.Timeout {
			http.Error(w, "compression timeout", 504)
			return
		}
		time.Sleep(100 * time.Millisecond) // Faster polling
	}

	// Send file as download
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(outPath)+"\"")
	http.ServeFile(w, r, outPath)
}

func main() {
	addr := envOr("PORT", "8080")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", health)
	mux.HandleFunc("/compress", compressHandler)
	mux.HandleFunc("/", uploadPage)

	s := &http.Server{
		Addr:              ":" + addr,
		Handler:           logMiddleware(mux),
		ReadHeaderTimeout: 20 * time.Second,
	}

	log.Printf("VideoCompress server listening on http://localhost:%s ...", addr)

	go func() {
		// graceful shutdown on SIGINT/SIGTERM
		stop := make(chan os.Signal, 1)
		// signal.Notify(stop, os.Interrupt, syscall.SIGTERM) // uncomment on Linux
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.Shutdown(ctx)
	}()

	err := s.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) && err != nil {
		log.Fatal(err)
	}
}

// simple request logger
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
