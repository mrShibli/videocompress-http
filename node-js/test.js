const { checkHealth } = require('./example');

async function runTests() {
    console.log('🧪 Running VideoCompress server tests...\n');

    // Test 1: Health check
    console.log('1. Testing health endpoint...');
    const healthy = await checkHealth();
    if (healthy) {
        console.log('✅ Health check passed\n');
    } else {
        console.log('❌ Health check failed\n');
        return;
    }

    // Test 2: Check if FFmpeg is available
    console.log('2. Checking FFmpeg availability...');
    const { exec } = require('child_process');
    exec('ffmpeg -version', (error, stdout, stderr) => {
        if (error) {
            console.log('❌ FFmpeg not found. Please install FFmpeg first.');
            console.log('   Download from: https://ffmpeg.org/download.html\n');
        } else {
            console.log('✅ FFmpeg is available');
            const version = stdout.split('\n')[0];
            console.log(`   Version: ${version}\n`);
        }
    });

    // Test 3: Check server endpoints
    console.log('3. Testing server endpoints...');
    const fetch = require('node-fetch');

    try {
        // Test web interface
        const webResponse = await fetch('http://localhost:8080/');
        if (webResponse.ok) {
            console.log('✅ Web interface accessible');
        } else {
            console.log('❌ Web interface not accessible');
        }

        // Test health endpoint
        const healthResponse = await fetch('http://localhost:8080/health');
        if (healthResponse.ok) {
            const health = await healthResponse.json();
            console.log('✅ Health endpoint working');
            console.log(`   Service: ${health.service}`);
            console.log(`   Version: ${health.version}`);
            console.log(`   Available modes: ${health.modes.join(', ')}`);
        } else {
            console.log('❌ Health endpoint not working');
        }

    } catch (error) {
        console.log('❌ Server endpoints test failed:', error.message);
    }

    console.log('\n🎉 Tests completed!');
    console.log('\n📝 Next steps:');
    console.log('   1. Open http://localhost:8080 in your browser');
    console.log('   2. Upload a video file to test compression');
    console.log('   3. Or use the API with curl/example.js');
}

// Run tests if this file is executed directly
if (require.main === module) {
    runTests();
}

module.exports = { runTests };
