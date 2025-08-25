<?php
// config/services.php - Add this to your Laravel services configuration

return [
    // ... other services

    'videocompress' => [
        'url' => env('VIDEOCOMPRESS_URL', 'http://localhost:8080/compress'),
        'timeout' => env('VIDEOCOMPRESS_TIMEOUT', 300),
        'max_file_size' => env('VIDEOCOMPRESS_MAX_FILE_SIZE', 2048), // MB
        'default_speed' => env('VIDEOCOMPRESS_DEFAULT_SPEED', 'ai'),
        'default_resolution' => env('VIDEOCOMPRESS_DEFAULT_RESOLUTION', 'original'),
        'default_codec' => env('VIDEOCOMPRESS_DEFAULT_CODEC', 'h264'),
        'default_hw' => env('VIDEOCOMPRESS_DEFAULT_HW', 'none'),
    ],
];

// .env file additions:
/*
VIDEOCOMPRESS_URL=http://localhost:8080/compress
VIDEOCOMPRESS_TIMEOUT=300
VIDEOCOMPRESS_MAX_FILE_SIZE=2048
VIDEOCOMPRESS_DEFAULT_SPEED=ai
VIDEOCOMPRESS_DEFAULT_RESOLUTION=original
VIDEOCOMPRESS_DEFAULT_CODEC=h264
VIDEOCOMPRESS_DEFAULT_HW=none
*/

// Database migration for video compressions table:
/*
<?php

use Illuminate\Database\Migrations\Migration;
use Illuminate\Database\Schema\Blueprint;
use Illuminate\Support\Facades\Schema;

class CreateVideoCompressionsTable extends Migration
{
    public function up()
    {
        Schema::create('video_compressions', function (Blueprint $table) {
            $table->id();
            $table->foreignId('user_id')->nullable()->constrained()->onDelete('cascade');
            $table->string('input_path');
            $table->string('output_path')->nullable();
            $table->json('options')->nullable();
            $table->enum('status', ['pending', 'processing', 'completed', 'failed'])->default('pending');
            $table->json('result_data')->nullable();
            $table->timestamp('started_at')->nullable();
            $table->timestamp('completed_at')->nullable();
            $table->timestamps();
            
            $table->index(['user_id', 'status']);
            $table->index('status');
        });
    }

    public function down()
    {
        Schema::dropIfExists('video_compressions');
    }
}
*/

// Routes example (routes/web.php or routes/api.php):
/*
Route::middleware(['auth'])->group(function () {
    Route::post('/videos/compress', [VideoController::class, 'compress'])->name('videos.compress');
    Route::get('/videos/compress/{id}/status', [VideoController::class, 'status'])->name('videos.status');
    Route::get('/videos/download/{filename}', [VideoController::class, 'download'])->name('videos.download');
});
*/

// Service Provider example (app/Providers/VideoCompressionServiceProvider.php):
/*
<?php

namespace App\Providers;

use Illuminate\Support\ServiceProvider;
use App\Services\VideoCompressionService;

class VideoCompressionServiceProvider extends ServiceProvider
{
    public function register()
    {
        $this->app->singleton(VideoCompressionService::class, function ($app) {
            return new VideoCompressionService(
                config('services.videocompress.url'),
                config('services.videocompress.timeout')
            );
        });
    }

    public function boot()
    {
        //
    }
}
*/

// Add to config/app.php providers array:
/*
'providers' => [
    // ... other providers
    App\Providers\VideoCompressionServiceProvider::class,
],
*/

// Blade view example (resources/views/videos/upload.blade.php):
/*
@extends('layouts.app')

@section('content')
<div class="container">
    <div class="row justify-content-center">
        <div class="col-md-8">
            <div class="card">
                <div class="card-header">Upload Video for Compression</div>
                <div class="card-body">
                    <form action="{{ route('videos.compress') }}" method="POST" enctype="multipart/form-data">
                        @csrf
                        <div class="form-group">
                            <label for="video">Video File</label>
                            <input type="file" class="form-control-file" id="video" name="video" accept="video/*" required>
                        </div>
                        
                        <div class="form-group">
                            <label for="speed">Compression Speed</label>
                            <select class="form-control" id="speed" name="speed">
                                <option value="ai">AI (Automatic)</option>
                                <option value="turbo">Turbo (Very Fast)</option>
                                <option value="max">Max (Maximum Compression)</option>
                                <option value="balanced">Balanced</option>
                                <option value="quality">Quality</option>
                            </select>
                        </div>
                        
                        <div class="form-group">
                            <label for="resolution">Resolution</label>
                            <select class="form-control" id="resolution" name="resolution">
                                <option value="original">Original</option>
                                <option value="720p">720p</option>
                                <option value="1080p">1080p</option>
                            </select>
                        </div>
                        
                        <button type="submit" class="btn btn-primary">Compress Video</button>
                    </form>
                </div>
            </div>
        </div>
    </div>
</div>

<script>
document.querySelector('form').addEventListener('submit', function(e) {
    e.preventDefault();
    
    const formData = new FormData(this);
    const submitBtn = this.querySelector('button[type="submit"]');
    
    submitBtn.disabled = true;
    submitBtn.textContent = 'Compressing...';
    
    fetch(this.action, {
        method: 'POST',
        body: formData,
        headers: {
            'X-CSRF-TOKEN': document.querySelector('meta[name="csrf-token"]').getAttribute('content')
        }
    })
    .then(response => response.json())
    .then(data => {
        if (data.success) {
            alert('Video compression started! ID: ' + data.compression_id);
            // Poll for status or redirect to status page
        } else {
            alert('Error: ' + data.error);
        }
    })
    .catch(error => {
        alert('Error: ' + error.message);
    })
    .finally(() => {
        submitBtn.disabled = false;
        submitBtn.textContent = 'Compress Video';
    });
});
</script>
@endsection
*/

// Artisan command example (app/Console/Commands/CompressVideo.php):
/*
<?php

namespace App\Console\Commands;

use Illuminate\Console\Command;
use App\Services\VideoCompressionService;

class CompressVideo extends Command
{
    protected $signature = 'video:compress {input} {output} {--speed=ai} {--resolution=original}';
    protected $description = 'Compress a video file using the VideoCompress API';

    public function handle(VideoCompressionService $compressionService)
    {
        $input = $this->argument('input');
        $output = $this->argument('output');
        $speed = $this->option('speed');
        $resolution = $this->option('resolution');

        if (!file_exists($input)) {
            $this->error("Input file not found: $input");
            return 1;
        }

        $this->info("Compressing video...");
        $this->info("Input: $input");
        $this->info("Output: $output");
        $this->info("Speed: $speed");
        $this->info("Resolution: $resolution");

        try {
            $result = $compressionService->compressAndSave($input, $output, [
                'speed' => $speed,
                'resolution' => $resolution
            ]);

            $this->info("Video compressed successfully!");
            $this->info("Output size: " . number_format($result['size'] / 1024 / 1024, 2) . " MB");

        } catch (\Exception $e) {
            $this->error("Compression failed: " . $e->getMessage());
            return 1;
        }

        return 0;
    }
}
*/

// Usage: php artisan video:compress input.mp4 output.mp4 --speed=balanced --resolution=720p
