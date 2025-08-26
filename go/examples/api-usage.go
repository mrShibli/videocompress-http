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

// Example 1: Using Accept header to get file bytes
func compressVideoWithHeader(filePath, speed string) error {
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

	// Print metadata headers
	fmt.Printf("Mode: %s\n", resp.Header.Get("X-Mode"))
	fmt.Printf("Duration: %s ms\n", resp.Header.Get("X-Encode-Duration-Ms"))
	fmt.Printf("Throughput: %s MB/s\n", resp.Header.Get("X-Throughput-MBps"))
	fmt.Printf("Input size: %s bytes\n", resp.Header.Get("X-Input-Bytes"))
	fmt.Printf("Output size: %s bytes\n", resp.Header.Get("X-Output-Bytes"))

	// Save compressed file
	outPath := "compressed_" + filepath.Base(filePath)
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	fmt.Printf("Compressed file saved as: %s\n", outPath)
	return nil
}

// Example 2: Using api=1 parameter to get file bytes
func compressVideoWithParam(filePath, speed string) error {
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
	writer.WriteField("api", "1") // This parameter also triggers API mode
	writer.Close()

	req, err := http.NewRequest("POST", "http://localhost:8080/compress", &buf)
	if err != nil {
		return err
	}
	
	req.Header.Set("Content-Type", writer.FormDataContentType())

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
	outPath := "compressed_api_" + filepath.Base(filePath)
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	fmt.Printf("Compressed file saved as: %s\n", outPath)
	return nil
}

// PrintCurlExample shows cURL commands for API usage
func printCurlExample() {
	fmt.Println(`
# cURL example to get compressed file bytes:
curl -X POST \
  -H "Accept: application/octet-stream" \
  -F "file=@input.mp4" \
  -F "speed=ai" \
  -o compressed.mp4 \
  http://localhost:8080/compress

# Alternative using api=1 parameter:
curl -X POST \
  -F "file=@input.mp4" \
  -F "speed=ai" \
  -F "api=1" \
  -o compressed.mp4 \
  http://localhost:8080/compress

# To see metadata headers:
curl -X POST \
  -H "Accept: application/octet-stream" \
  -F "file=@input.mp4" \
  -F "speed=ai" \
  -o compressed.mp4 \
  -D headers.txt \
  http://localhost:8080/compress
`)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run api-usage.go <video-file>")
		fmt.Println("\nExamples:")
		printCurlExample()
		return
	}

	filePath := os.Args[1]
	
	fmt.Println("=== Example 1: Using Accept header ===")
	err := compressVideoWithHeader(filePath, "ai")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}

	fmt.Println("\n=== Example 2: Using api=1 parameter ===")
	err = compressVideoWithParam(filePath, "ai")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
	}
}
