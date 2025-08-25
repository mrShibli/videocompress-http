const express = require('express');
const multer = require('multer');
const ffmpeg = require('fluent-ffmpeg');
const crypto = require('crypto');
const path = require('path');
const fs = require('fs');
const os = require('os');

// ======================
// Config
// ======================

const MAX_UPLOAD_SIZE = 2 * 1024 * 1024 * 1024; // 2 GB

// ======================
// Helpers
// ======================

function envOr(key, def) {
    return process.env[key] || def;
}

function withExt(filePath, newExt) {
    const base = path.basename(filePath, path.extname(filePath));
    return path.join(path.dirname(filePath), base + newExt);
}

function randID(n) {
    return crypto.randomBytes(n).toString('hex');
}

function humanBytes(n) {
    const k = 1024.0;
    const f = parseFloat(n);
    if (f >= k * k * k) {
        return `${(f / (k * k * k)).toFixed(2)} GB`;
    } else if (f >= k * k) {
        return `${(f / (k * k)).toFixed(2)} MB`;
    } else if (f >= k) {
        return `${(f / k).toFixed(2)} KB`;
    } else {
        return `${n} B`;
    }
}

// ======================
// Encoding options + logic
// ======================

class CompressOpts {
    constructor() {
        this.codec = ''; // h264|h265|copy
        this.crf = 0; // CPU encoders quality
        this.preset = ''; // ultrafast..placebo (CPU encoders)
        this.scale = ''; // e.g. 1280:-2 or 1920:1080 (fixed WxH). Leave empty to auto.
        this.fps = 0; // force output fps if >0
        this.audio = ''; // aac|opus|copy
        this.ab = ''; // audio bitrate (e.g. 128k)
        this.hw = ''; // videotoolbox|none
        this.outExt = ''; // .mp4 (recommended)
        this.speedMode = ''; // ultra_fast|super_fast|fast|balanced|quality|ai|max|turbo
        this.resolution = ''; // 360p|480p|720p|1080p|1440p|2160p|original
    }

    normalize() {
        if (!this.codec) this.codec = 'h264';
        if (!this.audio) this.audio = 'aac';
        if (!this.ab) this.ab = '128k';
        if (!this.hw) this.hw = 'none'; // VPS-safe default
        if (!this.outExt) this.outExt = '.mp4';
        if (!this.speedMode) this.speedMode = 'ai';
        if (!this.resolution) this.resolution = 'original';
        this.applyResolution();
    }

    applySpeedMode() {
        switch (this.speedMode) {
            case 'turbo':
                // Soft-fast turbo: AAC stereo, orientation-safe 720p long-edge handled in buildFFmpegArgs
                this.crf = 34;
                this.preset = 'ultrafast';
                this.ab = '96k';
                break;
            case 'max':
                // Very small/fast: orientation-safe 480p long-edge handled in buildFFmpegArgs
                this.crf = 36;
                this.preset = 'ultrafast';
                this.ab = '64k';
                break;
            case 'ultra_fast':
                this.crf = 32;
                this.preset = 'ultrafast';
                this.ab = '96k';
                break;
            case 'super_fast':
                this.crf = 30;
                this.preset = 'ultrafast';
                this.ab = '96k';
                break;
            case 'fast':
                this.crf = 28;
                this.preset = 'veryfast';
                this.ab = '128k';
                break;
            case 'quality':
                this.crf = 23;
                this.preset = 'fast';
                this.ab = '128k';
                break;
            default: // balanced
                if (this.crf === 0) this.crf = 26;
                if (!this.preset) this.preset = 'veryfast';
        }
    }

    applyResolution() {
        switch (this.resolution) {
            case '360p':
                this.scale = '640:360';
                break;
            case '480p':
                this.scale = '854:480';
                break;
            case '720p':
                this.scale = '1280:720';
                break;
            case '1080p':
                this.scale = '1920:1080';
                break;
            case '1440p':
                this.scale = '2560:1440';
                break;
            case '2160p':
                this.scale = '3840:2160';
                break;
            case 'original':
                // keep original (Scale empty)
                break;
            default:
            // unknown -> keep original
        }
    }

    tinyInputSafety(fileSize) {
        const sizeMB = fileSize / (1024 * 1024);
        if (sizeMB < 10) {
            this.codec = 'h264';
            this.audio = 'aac';
            this.scale = '';
            this.crf = 22;
            this.preset = 'veryfast';
            this.hw = 'none';
        }
    }
}

function chooseSpeedBySize(sizeMB) {
    if (sizeMB >= 700) return 'ultra_fast';
    if (sizeMB >= 200) return 'super_fast';
    if (sizeMB >= 50) return 'fast';
    if (sizeMB >= 10) return 'balanced';
    return 'balanced';
}

// ======================
// FFmpeg execution
// ======================

function buildFFmpegArgs(inPath, outPath, opts) {
    const args = ['-y', '-hide_banner', '-loglevel', 'error'];

    // Base flags; try HW decode on mac when enabled
    if (opts.hw.toLowerCase() === 'videotoolbox') {
        args.push('-hwaccel', 'videotoolbox', '-hwaccel_output_format', 'videotoolbox');
    }
    args.push('-i', inPath);

    // ---------------------------
    // ORIENTATION-SAFE SCALING
    // ---------------------------
    // For turbo/max we cap the LONG EDGE and preserve orientation:
    // a = iw/ih (aspect). Use -2 to keep even dimensions.
    //   turbo longEdge=720:  landscape -> h=720 (w auto), portrait -> w=720 (h auto)
    //   max   longEdge=480:  landscape -> h=480 (w auto), portrait -> w=480 (h auto)
    let vf = '';
    if (opts.codec.toLowerCase() !== 'copy') {
        switch (opts.speedMode) {
            case 'turbo':
                if (opts.fps === 0) opts.fps = 24;
                vf = "scale='if(gt(a,1),-2,720)':'if(gt(a,1),720,-2)':flags=fast_bilinear,setsar=1";
                break;
            case 'max':
                if (opts.fps === 0) opts.fps = 24;
                vf = "scale='if(gt(a,1),-2,480)':'if(gt(a,1),480,-2)':flags=fast_bilinear,setsar=1";
                break;
            default:
                // Respect explicit fixed WxH if provided (e.g. from Resolution),
                // otherwise don't add a scale filter.
                if (opts.scale) {
                    vf = `scale=${opts.scale}:flags=fast_bilinear,setsar=1`;
                }
        }
    }
    if (vf) {
        args.push('-vf', vf);
    }

    // fps (only if re-encoding video)
    if (opts.fps > 0 && opts.codec.toLowerCase() !== 'copy') {
        args.push('-r', opts.fps.toString());
    }

    // ---------------------------
    // VIDEO CODEC
    // ---------------------------
    let vcodec = '';
    switch (opts.codec.toLowerCase()) {
        case 'copy':
            vcodec = 'copy';
            break;
        case 'h265':
            if (opts.hw.toLowerCase() === 'videotoolbox') {
                vcodec = 'hevc_videotoolbox';
            } else {
                vcodec = 'libx265';
            }
            break;
        default: // h264
            if (opts.hw.toLowerCase() === 'videotoolbox') {
                vcodec = 'h264_videotoolbox';
            } else {
                vcodec = 'libx264';
            }
    }

    if (vcodec === 'copy') {
        args.push('-c:v', 'copy');
    } else {
        args.push('-c:v', vcodec);

        switch (vcodec) {
            case 'libx264':
            case 'libx265':
                args.push('-crf', opts.crf.toString(), '-preset', opts.preset);
                break;
            case 'h264_videotoolbox':
            case 'hevc_videotoolbox':
                // map CRF→bitrate for hardware encoders
                let bitrate = '3M';
                if (opts.crf <= 20) bitrate = '5M';
                else if (opts.crf <= 23) bitrate = '4M';
                else if (opts.crf <= 26) bitrate = '3M';
                else if (opts.crf <= 30) bitrate = '2.5M';
                else if (opts.crf <= 36) bitrate = '2M';
                else bitrate = '1500k';

                // Turbo a bit higher for decent 720p long-edge
                if (opts.speedMode === 'turbo') {
                    bitrate = '2500k';
                }
                args.push('-b:v', bitrate);
        }

        // browser/player compatibility
        if (opts.outExt.toLowerCase() === '.mp4') {
            args.push('-pix_fmt', 'yuv420p');
        }
    }

    // Extra accelerations (zero-latency style) for turbo/max
    if (opts.speedMode === 'max' || opts.speedMode === 'turbo') {
        switch (vcodec) {
            case 'libx264':
                args.push('-tune', 'fastdecode,zerolatency');
                args.push('-g', '300', '-keyint_min', '300');
                args.push('-x264-params',
                    'no-scenecut=1:ref=1:bframes=0:me=dia:subme=0:trellis=0:aq-mode=0:fast_pskip=1:sync-lookahead=0:rc-lookahead=0');
                break;
            case 'libx265':
                args.push('-tune', 'fastdecode');
                args.push('-g', '300', '-keyint_min', '300');
                break;
            case 'h264_videotoolbox':
            case 'hevc_videotoolbox':
                args.push('-realtime', 'true');
                args.push('-g', '300');
        }
    }

    // ---------------------------
    // AUDIO
    // ---------------------------
    switch (opts.audio.toLowerCase()) {
        case 'copy':
            args.push('-c:a', 'copy');
            break;
        case 'opus':
            args.push('-c:a', 'libopus', '-b:a', opts.ab);
            break;
        default:
            args.push('-c:a', 'aac', '-b:a', opts.ab);
            // turbo: stereo 96k; max: mono 64k
            if (opts.speedMode === 'turbo') {
                args.push('-ac', '2');
                args.push('-b:a', '96k');
            } else if (opts.speedMode === 'max') {
                args.push('-ac', '1');
                args.push('-b:a', '64k');
            }
    }

    // faststart + threads
    args.push('-movflags', '+faststart', '-threads', '0', outPath);
    return args;
}

function runFFmpeg(inPath, outPath, opts) {
    return new Promise((resolve, reject) => {
        opts.normalize();
        const args = buildFFmpegArgs(inPath, outPath, opts);

        const command = ffmpeg(inPath);

        // Apply all arguments
        args.forEach((arg, index) => {
            if (index === 0) return; // Skip input file as it's already set
            if (arg === '-i') {
                // Skip -i and its value as input is already set
                return;
            }
            if (args[index - 1] === '-i') {
                return; // Skip the value after -i
            }

            if (arg.startsWith('-')) {
                const value = args[index + 1];
                if (value && !value.startsWith('-')) {
                    command.addOption(arg, value);
                } else {
                    command.addOption(arg);
                }
            }
        });

        command
            .output(outPath)
            .on('end', () => resolve())
            .on('error', (err) => {
                // If VideoToolbox failed, retry with CPU
                if (opts.hw.toLowerCase().includes('videotoolbox') && err.message.includes('videotoolbox')) {
                    console.log('VideoToolbox failed; falling back to CPU.');
                    opts.hw = 'none';
                    const cpuArgs = buildFFmpegArgs(inPath, outPath, opts);

                    const cpuCommand = ffmpeg(inPath);
                    cpuArgs.forEach((arg, index) => {
                        if (index === 0) return;
                        if (arg === '-i') return;
                        if (args[index - 1] === '-i') return;

                        if (arg.startsWith('-')) {
                            const value = args[index + 1];
                            if (value && !value.startsWith('-')) {
                                cpuCommand.addOption(arg, value);
                            } else {
                                cpuCommand.addOption(arg);
                            }
                        }
                    });

                    cpuCommand
                        .output(outPath)
                        .on('end', () => resolve())
                        .on('error', (cpuErr) => reject(cpuErr))
                        .run();
                } else {
                    reject(err);
                }
            })
            .run();
    });
}

// ======================
// Result Store (for UI flow)
// ======================

class ResultEntry {
    constructor(filePath, modeFinal, modeDecider, inputBytes, outputBytes, resolution, codec, audio, hw, elapsedMs, throughput) {
        this.filePath = filePath;
        this.modeFinal = modeFinal;
        this.modeDecider = modeDecider; // "ai" or "manual"
        this.inputBytes = inputBytes;
        this.outputBytes = outputBytes;
        this.resolution = resolution;
        this.codec = codec;
        this.audio = audio;
        this.hw = hw;
        this.elapsedMs = elapsedMs;
        this.throughput = throughput; // MB/s
    }
}

const store = new Map();

// ======================
// Templates
// ======================

const uploadTemplate = `
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
curl -f -S -o out.mp4 \\
  -H "Accept: application/octet-stream" \\
  -F "file=@input.mp4" \\
  -F "speed=ai" \\
  http://localhost:8080/compress -D headers.txt

# headers include: X-Encode-Duration-Ms and X-Throughput-MBps
</pre>

<p style="margin-top: 20px; text-align: center;">
  <a href="/api-docs" style="color: #667eea; text-decoration: none; font-weight: 600;">📖 View Complete API Documentation</a>
</p>
`;

const resultTemplate = (data) => `
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
  <div>Mode</div><div><code>${data.modeFinal}</code> <small>(decided by: ${data.modeDecider})</small></div>
  <div>Time taken</div><div>${data.seconds.toFixed(2)} s</div>
  <div>Throughput</div><div>${data.throughput.toFixed(2)} MB/s</div>
  <div>Input size</div><div>${data.inputHuman} (${data.inputBytes} bytes)</div>
  <div>Output size</div><div>${data.outputHuman} (${data.outputBytes} bytes)</div>
  <div>Resolution</div><div>${data.resolution}</div>
  <div>Video codec</div><div>${data.codec}</div>
  <div>Audio codec</div><div>${data.audio}</div>
  <div>Hardware</div><div>${data.hw}</div>
</div>

<a class="btn" href="/dl/${data.id}?name=${data.suggestName}">⬇️ Download compressed file</a>

<h3>API example</h3>
<pre>
curl -f -S -o out.mp4 \\
  -H "Accept: application/octet-stream" \\
  -F "file=@input.mp4" \\
  -F "speed=${data.modeDecider}" \\
  http://localhost:8080/compress \\
  -D headers.txt

# Timing headers:
# X-Encode-Duration-Ms, X-Throughput-MBps
</pre>
`;

// ======================
// Express app setup
// ======================

const app = express();

// Serve static files from public directory
app.use(express.static('public'));

const upload = multer({
    dest: os.tmpdir(),
    limits: {
        fileSize: MAX_UPLOAD_SIZE
    }
});

// Parse options from request
function parseOpts(req) {
    const opts = new CompressOpts();
    opts.codec = req.body.codec || 'h264';
    opts.audio = req.body.audio || 'aac';
    opts.ab = req.body.ab || '';
    opts.hw = req.body.hw || 'none';
    opts.outExt = req.body.outExt || '.mp4';
    opts.speedMode = req.body.speed || 'ai';
    opts.resolution = req.body.resolution || 'original';

    if (req.body.fps) {
        const fps = parseInt(req.body.fps);
        if (fps > 0 && fps <= 60) {
            opts.fps = fps;
        }
    }

    opts.normalize();
    return opts;
}

// Routes
app.get('/', (req, res) => {
    res.setHeader('Content-Type', 'text/html; charset=utf-8');
    res.send(uploadTemplate);
});

app.post('/compress', upload.single('file'), async (req, res) => {
    try {
        if (!req.file) {
            return res.status(400).send('file field required');
        }

        const inPath = req.file.path;
        const originalName = req.file.originalname;

        // Parse options
        const opts = parseOpts(req);

        // File size
        const stats = fs.statSync(inPath);
        const inputBytes = stats.size;
        const sizeMB = inputBytes / (1024 * 1024);

        // Decide final mode if AI (size-only)
        let modeDecider = 'manual';
        if (opts.speedMode === 'ai') {
            modeDecider = 'ai';
            let base = chooseSpeedBySize(sizeMB);
            if (sizeMB >= 200 && sizeMB < 2048) {
                if (sizeMB <= 250) {
                    base = 'balanced';
                } else {
                    base = 'ultra_fast';
                }
            }
            opts.speedMode = base;
        }

        // Small-file safety
        opts.tinyInputSafety(inputBytes);

        // Apply profile params
        opts.applySpeedMode();

        const outPath = withExt(inPath, '_compressed' + opts.outExt);

        // --- timing starts here ---
        const start = Date.now();

        // Run ffmpeg
        await runFFmpeg(inPath, outPath, opts);

        const elapsed = Date.now() - start;

        // validate output
        const outStats = fs.statSync(outPath);
        if (outStats.size < 1024) {
            return res.status(500).send('output seems empty or invalid');
        }
        const outputBytes = outStats.size;

        // throughput (MB/s) = input size / seconds
        const throughput = (inputBytes / (1024 * 1024)) / (elapsed / 1000);

        // If API call (Accept: application/octet-stream), return FILE BYTES + metadata headers
        const accept = req.headers.accept || '';
        if (accept.includes('application/octet-stream') || req.body.api === '1') {
            // add metadata headers
            res.setHeader('X-Mode', opts.speedMode);
            res.setHeader('X-Mode-Decider', modeDecider);
            res.setHeader('X-Encode-Duration-Ms', elapsed.toString());
            res.setHeader('X-Throughput-MBps', throughput.toFixed(4));
            res.setHeader('X-Input-Bytes', inputBytes.toString());
            res.setHeader('X-Output-Bytes', outputBytes.toString());
            res.setHeader('X-Resolution', opts.resolution);
            res.setHeader('X-Video-Codec', opts.codec);
            res.setHeader('X-Audio-Codec', opts.audio);
            res.setHeader('X-HW', opts.hw);

            let ctype = 'application/octet-stream';
            const ext = path.extname(outPath).toLowerCase();
            if (ext === '.mp4') ctype = 'video/mp4';
            else if (ext === '.mov') ctype = 'video/quicktime';

            res.setHeader('Content-Type', ctype);
            res.setHeader('Content-Disposition', `attachment; filename="${path.basename(outPath)}"`);
            res.sendFile(outPath);
            return;
        }

        // UI flow: keep file in temp store and show a result page
        const id = randID(12);
        const entry = new ResultEntry(
            outPath,
            opts.speedMode,
            modeDecider,
            inputBytes,
            outputBytes,
            opts.resolution,
            opts.codec,
            opts.audio,
            opts.hw,
            elapsed,
            throughput
        );
        store.set(id, entry);

        // Render result HTML
        res.setHeader('Content-Type', 'text/html; charset=utf-8');
        const data = {
            id: id,
            modeFinal: entry.modeFinal,
            modeDecider: entry.modeDecider,
            inputBytes: entry.inputBytes,
            outputBytes: entry.outputBytes,
            inputHuman: humanBytes(entry.inputBytes),
            outputHuman: humanBytes(entry.outputBytes),
            resolution: entry.resolution,
            codec: entry.codec,
            audio: entry.audio,
            hw: entry.hw,
            suggestName: path.basename(outPath),
            seconds: entry.elapsedMs / 1000.0,
            throughput: entry.throughput
        };
        res.send(resultTemplate(data));

    } catch (error) {
        console.error('Compression error:', error);
        res.status(500).send('compression failed: ' + error.message);
    } finally {
        // Clean up input file
        if (req.file) {
            fs.unlinkSync(req.file.path);
        }
    }
});

app.get('/dl/:id', (req, res) => {
    const id = req.params.id;
    const entry = store.get(id);
    if (!entry) {
        return res.status(404).send('Not found');
    }

    const name = req.query.name || path.basename(entry.filePath);
    let ctype = 'application/octet-stream';
    const ext = path.extname(name).toLowerCase();
    if (ext === '.mp4') ctype = 'video/mp4';
    else if (ext === '.mov') ctype = 'video/quicktime';

    res.setHeader('Content-Type', ctype);
    res.setHeader('Content-Disposition', `attachment; filename="${name}"`);
    res.sendFile(entry.filePath);
});

app.get('/meta/:id', (req, res) => {
    const id = req.params.id;
    const entry = store.get(id);
    if (!entry) {
        return res.status(404).send('Not found');
    }

    res.setHeader('Content-Type', 'application/json');
    res.json({
        id: id,
        mode: entry.modeFinal,
        mode_decider: entry.modeDecider,
        input_bytes: entry.inputBytes,
        output_bytes: entry.outputBytes,
        resolution: entry.resolution,
        codec: entry.codec,
        audio: entry.audio,
        hw: entry.hw,
        encode_duration_ms: entry.elapsedMs,
        throughput_mb_s: entry.throughput
    });
});

app.get('/health', (req, res) => {
    res.setHeader('Content-Type', 'application/json');
    res.json({
        ok: true,
        service: 'videocompress',
        version: '3.2.0-orientation',
        modes: ['ai', 'turbo', 'max', 'ultra_fast', 'super_fast', 'fast', 'balanced', 'quality'],
        defaults: { codec: 'h264', resolution: 'original', hw: 'none' },
        ui_routes: ['/', '/compress (POST)', '/dl/{id}', '/meta/{id}']
    });
});

app.get('/api-docs', (req, res) => {
    res.sendFile(path.join(__dirname, 'public', 'api-docs.html'));
});

// Basic request logger
app.use((req, res, next) => {
    console.log(`${req.method} ${req.path}`);
    next();
});

// ======================
// Server startup
// ======================

const port = envOr('PORT', '8080');

app.listen(port, () => {
    console.log(`VideoCompress server listening on http://localhost:${port} ...`);
});
