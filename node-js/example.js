const fs = require('fs');
const FormData = require('form-data');
const fetch = require('node-fetch');

// Example usage of the VideoCompress API
async function compressVideo(inputPath, outputPath, options = {}) {
    const form = new FormData();

    // Add the video file
    form.append('file', fs.createReadStream(inputPath));

    // Add compression options
    form.append('speed', options.speed || 'ai');
    form.append('resolution', options.resolution || 'original');
    form.append('codec', options.codec || 'h264');
    form.append('hw', options.hw || 'none');

    try {
        console.log(`Compressing ${inputPath}...`);

        const response = await fetch('http://localhost:8080/compress', {
            method: 'POST',
            body: form,
            headers: {
                'Accept': 'application/octet-stream'
            }
        });

        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }

        // Get metadata from headers
        const metadata = {
            mode: response.headers.get('X-Mode'),
            modeDecider: response.headers.get('X-Mode-Decider'),
            duration: response.headers.get('X-Encode-Duration-Ms'),
            throughput: response.headers.get('X-Throughput-MBps'),
            inputBytes: response.headers.get('X-Input-Bytes'),
            outputBytes: response.headers.get('X-Output-Bytes'),
            resolution: response.headers.get('X-Resolution'),
            codec: response.headers.get('X-Video-Codec'),
            audio: response.headers.get('X-Audio-Codec'),
            hw: response.headers.get('X-HW')
        };

        // Save the compressed video
        const fileStream = fs.createWriteStream(outputPath);
        response.body.pipe(fileStream);

        return new Promise((resolve, reject) => {
            fileStream.on('finish', () => {
                console.log('✅ Compression complete!');
                console.log('📊 Metadata:', metadata);
                resolve(metadata);
            });
            fileStream.on('error', reject);
        });

    } catch (error) {
        console.error('❌ Compression failed:', error.message);
        throw error;
    }
}

// Example usage
async function main() {
    try {
        // Example 1: AI mode (automatic settings)
        await compressVideo('input.mp4', 'output_ai.mp4', {
            speed: 'ai'
        });

        // Example 2: Turbo mode for fast compression
        await compressVideo('input.mp4', 'output_turbo.mp4', {
            speed: 'turbo',
            resolution: '720p'
        });

        // Example 3: Quality mode with custom settings
        await compressVideo('input.mp4', 'output_quality.mp4', {
            speed: 'quality',
            resolution: '1080p',
            codec: 'h265',
            hw: 'videotoolbox' // macOS only
        });

    } catch (error) {
        console.error('Example failed:', error);
    }
}

// Health check
async function checkHealth() {
    try {
        const response = await fetch('http://localhost:8080/health');
        const health = await response.json();
        console.log('🏥 Server health:', health);
        return health.ok;
    } catch (error) {
        console.error('❌ Server not responding:', error.message);
        return false;
    }
}

// Run examples if this file is executed directly
if (require.main === module) {
    (async () => {
        console.log('🔍 Checking server health...');
        const healthy = await checkHealth();

        if (!healthy) {
            console.log('❌ Server is not running. Please start the server first:');
            console.log('   npm start');
            process.exit(1);
        }

        console.log('✅ Server is healthy!');
        console.log('📝 Note: Make sure you have an "input.mp4" file in the current directory');
        console.log('   or modify the example.js file to use your own video file.\n');

        // Uncomment to run examples:
        // await main();
    })();
}

module.exports = { compressVideo, checkHealth };
