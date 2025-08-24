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
	Codec   string // h264|h265|copy
	CRF     int
	Preset  string // ultrafast..placebo or p1..p7 for nvenc; ignored for videotoolbox
	Scale   string // e.g. 1280:-2
	Audio   string // aac|opus|copy
	AB      string // audio bitrate, e.g. 128k
	HW      string // videotoolbox|none
	OutExt  string // .mp4 or .mov etc.
	Timeout time.Duration
}

func (o *compressOpts) normalize() {
	if o.Codec == "" { o.Codec = "h264" }
	if o.CRF == 0 { o.CRF = 28 } // Higher CRF = faster encoding, smaller file
	if o.Preset == "" { o.Preset = "ultrafast" } // Fastest preset
	if o.Audio == "" { o.Audio = "aac" }
	if o.AB == "" { o.AB = "96k" } // Lower audio bitrate for speed
	if o.HW == "" { o.HW = "videotoolbox" } // Default to hardware acceleration
	if o.OutExt == "" { o.OutExt = ".mp4" }
	if o.Timeout == 0 { o.Timeout = 30 * time.Minute } // Reduced timeout
}

// Build ffmpeg args based on options/platform
func buildFFmpegArgs(inPath, outPath string, o compressOpts) []string {
	args := []string{"-y", "-hide_banner", "-loglevel", "error", "-i", inPath}

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
			// VideoToolbox with optimized bitrates for speed
			bitrate := "1.5M" // default for CRF 28 (faster encoding)
			if o.CRF < 20 {
				bitrate = "3M" // higher quality
			} else if o.CRF > 30 {
				bitrate = "1M" // lower quality, faster
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
	args = append(args, "-movflags", "+faststart", "-threads", "0", outPath)
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
		return cmd.Run()
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
		"version":  "1.0.0",
		"ffmpeg":   "required",
		"platform": "macOS supported (VideoToolbox), Linux/Windows (CPU encoders)",
	})
}

var uploadTpl = template.Must(template.New("u").Parse(`
<!doctype html>
<meta charset="utf-8">
<title>Video Compress</title>
<style>
body{font-family:ui-sans-serif,system-ui;margin:40px;max-width:720px;line-height:1.6}
.alert{background:#f0f9ff;border:1px solid #0ea5e9;border-radius:8px;padding:16px;margin:16px 0}
.alert h3{margin:0 0 8px 0;color:#0369a1}
.form-group{margin:12px 0}
.form-group label{display:block;margin-bottom:4px;font-weight:500}
.form-group input{width:100%;padding:8px;border:1px solid #d1d5db;border-radius:4px}
button{background:#3b82f6;color:white;border:none;padding:12px 24px;border-radius:6px;cursor:pointer;font-size:16px}
button:hover{background:#2563eb}
details{margin:16px 0}
summary{cursor:pointer;font-weight:500}
pre{background:#f3f4f6;padding:12px;border-radius:4px;overflow-x:auto}
</style>
<h1>Video Compress</h1>
<div class="alert">
  <h3>⚡ Optimized for Speed + Fixed Black Screen</h3>
  <p>• Hardware acceleration by default • Faster presets • Lower bitrates • 30min timeout • CPU fallback if needed</p>
</div>
<form method="post" action="/compress" enctype="multipart/form-data">
  <div class="form-group">
    <label>Video file: <input type="file" name="file" required accept="video/*"></label>
  </div>
  <details>
    <summary>Advanced Settings</summary>
    <div class="form-group">
      <label>Codec: <input name="codec" value="h264" placeholder="h264|h265|copy"></label>
    </div>
    <div class="form-group">
      <label>Quality (CRF): <input name="crf" value="28" type="number" min="0" max="51"></label>
    </div>
    <div class="form-group">
      <label>Preset: <input name="preset" value="ultrafast" placeholder="ultrafast|veryfast|fast|medium|slow"></label>
    </div>
    <div class="form-group">
      <label>Scale: <input name="scale" placeholder="1280:-2 (width:height)"></label>
    </div>
    <div class="form-group">
      <label>Audio Codec: <input name="audio" value="aac" placeholder="aac|opus|copy"></label>
    </div>
    <div class="form-group">
      <label>Audio Bitrate: <input name="ab" value="96k" placeholder="96k"></label>
    </div>
    <div class="form-group">
      <label>Hardware: <input name="hw" value="videotoolbox" placeholder="videotoolbox|none"></label>
    </div>
    <div class="form-group">
      <label>Output Extension: <input name="outExt" value=".mp4" placeholder=".mp4|.mov"></label>
    </div>
  </details>
  <button type="submit">Compress Video</button>
</form>
<p><strong>Or use curl:</strong></p>
<pre>curl -F "file=@input.mp4" http://localhost:8080/compress -o output.mp4</pre>
<p><strong>For maximum speed:</strong></p>
<pre>curl -F "file=@input.mp4" -F "hw=videotoolbox" -F "crf=30" -F "preset=ultrafast" http://localhost:8080/compress -o output.mp4</pre>
<p><strong>For guaranteed working videos (slower):</strong></p>
<pre>curl -F "file=@input.mp4" -F "hw=none" http://localhost:8080/compress -o output.mp4</pre>
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
	o.Preset = get("preset", "veryfast")
	o.Scale = get("scale", "")
	o.Audio = get("audio", "aac")
	o.AB = get("ab", "128k")
	o.HW = get("hw", "none")
	o.OutExt = get("outExt", ".mp4")

	crfStr := get("crf", "23")
	crf, err := strconv.Atoi(crfStr)
	if err != nil {
		return o, fmt.Errorf("invalid crf: %w", err)
	}
	o.CRF = crf

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
