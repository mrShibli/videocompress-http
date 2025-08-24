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

// ======================
// Config
// ======================

const (
	// Max upload size accepted by the server (adjust if you need more/less)
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

// ======================
// Encoding options + logic
// ======================

type compressOpts struct {
	Codec      string        // h264|h265|copy
	CRF        int           // used for CPU encoders and as a "quality hint"
	Preset     string        // ultrafast..placebo (CPU encoders)
	Scale      string        // e.g. 1280:-2 or 1920:1080
	Audio      string        // aac|opus|copy
	AB         string        // audio bitrate, e.g. 128k
	HW         string        // videotoolbox|none
	OutExt     string        // .mp4 (recommended)
	Timeout    time.Duration // overall job timeout
	SpeedMode  string        // ultra_fast|super_fast|fast|balanced|quality
	Resolution string        // 360p|480p|720p|1080p|1440p|2160p|original
}

func (o *compressOpts) normalize() {
	// sensible defaults
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
		o.HW = "none" // default to CPU (most reliable across VPS)
	}
	if o.OutExt == "" {
		o.OutExt = ".mp4"
	}
	if o.Timeout == 0 {
		o.Timeout = 45 * time.Minute
	}
	if o.SpeedMode == "" {
		o.SpeedMode = "balanced"
	}
	if o.Resolution == "" {
		o.Resolution = "original"
	}
	o.applySpeedMode()
	o.applyResolution()
}

// Speed profiles primarily tune CRF/Preset for CPU mode.
// For VideoToolbox we’ll map CRF → a reasonable bitrate later.
func (o *compressOpts) applySpeedMode() {
	switch o.SpeedMode {
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
		// keep original, leave o.Scale = ""
	default:
		// unknown -> keep original
	}
}

// Extra safety for small inputs: gently re-encode instead of over-compressing
func (o *compressOpts) adjustForFileSize(fileSize int64) {
	sizeMB := fileSize / (1024 * 1024)
	if sizeMB < 10 { // tiny inputs
		o.Codec = "h264"
		o.Audio = "aac"
		o.Scale = ""       // preserve resolution
		o.CRF = 22         // light touch to avoid artifacts
		o.Preset = "veryfast"
		o.HW = "none"      // CPU = most stable
	}
}

// Translate options → ffmpeg arguments
func buildFFmpegArgs(inPath, outPath string, o compressOpts) []string {
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-i", inPath}

	// Scaling (fast bilinear is fine for speed modes)
	if o.Scale != "" && strings.ToLower(o.Codec) != "copy" {
		args = append(args, "-vf", "scale="+o.Scale+":flags=fast_bilinear")
	}

	// Pick video encoder
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

	// Set video params
	if vcodec == "copy" {
		args = append(args, "-c:v", "copy")
	} else {
		args = append(args, "-c:v", vcodec)

		switch vcodec {
		case "libx264", "libx265":
			// CPU encoders: CRF + preset
			args = append(args, "-crf", strconv.Itoa(o.CRF), "-preset", o.Preset)
		case "h264_videotoolbox", "hevc_videotoolbox":
			// VideoToolbox prefers a bitrate target; map CRF → bitrate.
			// Feel free to tweak these if you know your content.
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
			default:
				bitrate = "2M"
			}
			args = append(args, "-b:v", bitrate)
		}

		// MP4 playback compatibility (only when re-encoding video)
		if strings.ToLower(o.OutExt) == ".mp4" {
			args = append(args, "-pix_fmt", "yuv420p")
		}
	}

	// Audio
	switch strings.ToLower(o.Audio) {
	case "copy":
		args = append(args, "-c:a", "copy")
	case "opus":
		args = append(args, "-c:a", "libopus", "-b:a", o.AB)
	default: // aac
		args = append(args, "-c:a", "aac", "-b:a", o.AB)
	}

	// Web playback: moov atom first + let ffmpeg decide threads
	args = append(args, "-movflags", "+faststart", "-threads", "0", outPath)

	return args
}

// Run ffmpeg and WAIT for it to finish (critical!)
// If VideoToolbox fails, automatically retry with CPU.
func runFFmpeg(ctx context.Context, inPath, outPath string, o compressOpts, logWriter io.Writer) error {
	o.normalize()
	args := buildFFmpegArgs(inPath, outPath, o)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = logWriter
	cmd.Stderr = logWriter
	err := cmd.Run()
	if err == nil {
		return nil
	}

	// Auto-fallback: if HW failed, retry CPU
	if strings.Contains(strings.ToLower(o.HW), "videotoolbox") {
		fmt.Fprintln(logWriter, "VideoToolbox failed; falling back to CPU (libx264/libx265).")
		o.HW = "none"
		args = buildFFmpegArgs(inPath, outPath, o)
		cmd = exec.CommandContext(ctx, "ffmpeg", args...)
		cmd.Stdout = logWriter
		cmd.Stderr = logWriter
		return cmd.Run()
	}
	return err
}

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
body{font-family:ui-sans-serif,system-ui;margin:40px;max-width:800px;line-height:1.6}
.form-group{margin:12px 0}
label{display:block;margin-bottom:6px;font-weight:600}
input,select{width:100%;padding:8px;border:1px solid #d1d5db;border-radius:6px}
button{background:#111827;color:#fff;border:0;padding:12px 20px;border-radius:8px;cursor:pointer}
button:hover{background:#0f172a}
details{margin:12px 0}
pre{background:#f3f4f6;padding:12px;border-radius:6px;overflow:auto}
.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:8px}
.card{border:1px solid #e5e7eb;border-radius:8px;padding:8px}
.card input{width:auto}
</style>

<h1>Video Compress</h1>

<form method="post" action="/compress" enctype="multipart/form-data">
  <div class="form-group">
    <label>Video file</label>
    <input type="file" name="file" accept="video/*" required>
  </div>

  <div class="grid">
    <div class="card">
      <label>Speed mode</label>
      <select name="speed">
        <option value="balanced" selected>Balanced</option>
        <option value="fast">Fast</option>
        <option value="super_fast">Super Fast</option>
        <option value="ultra_fast">Ultra Fast</option>
        <option value="quality">Quality</option>
      </select>
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

<h3>cURL example</h3>
<pre>
curl -F "file=@input.mp4" \
     -F "speed=balanced" \
     -F "resolution=original" \
     http://localhost:8080/compress -o output.mp4
</pre>
`))

func uploadPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = uploadTpl.Execute(w, nil)
}

// Cleanly save a multipart file to disk
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

// Parse all options (after ParseMultipartForm)
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
	o.SpeedMode = get("speed", "balanced")
	o.Resolution = get("resolution", "original")

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

	// Parse the multipart form FIRST so FormValue works
	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		http.Error(w, "expecting multipart/form-data: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, hdr, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "file field required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Save upload to temp file
	inPath := filepath.Join(os.TempDir(), filepath.Base(hdr.Filename))
	out, err := os.Create(inPath)
	if err != nil {
		http.Error(w, "save error: "+err.Error(), 500)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		http.Error(w, "save error: "+err.Error(), 500)
		return
	}
	out.Close()
	defer os.Remove(inPath)

	// Build options (now that form is parsed)
	opts, err := parseOpts(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	// Adjust for tiny files for safety
	if st, err := os.Stat(inPath); err == nil {
		opts.adjustForFileSize(st.Size())
	}

	// Output path
	outPath := withExt(inPath, "_compressed"+opts.OutExt)
	defer func() {
		// Keep output on disk so user can re-download if they refresh; delete if you prefer
	}()

	// Run ffmpeg synchronously and WAIT until it finishes
	ctx, cancel := context.WithTimeout(r.Context(), opts.Timeout)
	defer cancel()

	if err := runFFmpeg(ctx, inPath, outPath, opts, io.Discard); err != nil {
		http.Error(w, "compression failed: "+err.Error(), 500)
		return
	}

	// Validate output
	stat, err := os.Stat(outPath)
	if err != nil || stat.Size() < 1024 {
		http.Error(w, "output seems empty or invalid", 500)
		return
	}

	// Serve the finished file
	ctype := "application/octet-stream"
	switch strings.ToLower(filepath.Ext(outPath)) {
	case ".mp4":
		ctype = "video/mp4"
	case ".mov":
		ctype = "video/quicktime"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(outPath)+"\"")
	http.ServeFile(w, r, outPath)
}

func health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"service":   "videocompress",
		"version":   "2.1.0",
		"ffmpeg":    "required",
		"defaults":  map[string]any{"codec": "h264", "speed": "balanced", "resolution": "original"},
		"hardware":  []string{"none (CPU)", "videotoolbox (macOS)"},
		"endpoints": []string{"/", "/compress", "/health"},
	})
}

func main() {
	addr := envOr("PORT", "8080")

	mux := http.NewServeMux()
	mux.HandleFunc("/", uploadPage)
	mux.HandleFunc("/compress", compressHandler)
	mux.HandleFunc("/health", health)

	s := &http.Server{
		Addr:              ":" + addr,
		Handler:           logMiddleware(mux),
		ReadHeaderTimeout: 20 * time.Second,
	}

	log.Printf("VideoCompress server listening on http://localhost:%s ...", addr)

	if err := s.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

// basic request logger
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}
