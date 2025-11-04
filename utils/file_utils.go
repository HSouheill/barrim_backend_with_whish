package utils

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

const (
	// Base directory for storing uploaded files
	uploadBaseDir = "uploads"
	// Base URL for serving files
	baseURL = "/uploads"
	// Maximum file size (10MB)
	maxFileSize = 10 * 1024 * 1024
)

var (
	// Allowed image extensions
	allowedImageExts = map[string]bool{
		".jpg":  true,
		".jpeg": true,
		".png":  true,
		".gif":  true,
		".svg":  true,
	}
	// Allowed video extensions
	allowedVideoExts = map[string]bool{
		".mp4":  true,
		".mov":  true,
		".avi":  true,
		".webm": true,
	}
)

// cleanFilename removes any potentially dangerous characters from the filename
func cleanFilename(filename string) string {
	// Remove any path components
	filename = filepath.Base(filename)
	// Remove any non-alphanumeric characters except for dots and hyphens
	reg := regexp.MustCompile(`[^a-zA-Z0-9.-]`)
	return reg.ReplaceAllString(filename, "")
}

// ValidateFileType checks if the file extension is allowed for the given media type
func ValidateFileType(filename, mediaType string) error {
	ext := strings.ToLower(filepath.Ext(filename))

	switch mediaType {
	case "image":
		if !allowedImageExts[ext] {
			return fmt.Errorf("unsupported image format. Allowed formats: jpg, jpeg, png, gif, svg")
		}
	case "video":
		if !allowedVideoExts[ext] {
			return fmt.Errorf("unsupported video format. Allowed formats: mp4, mov, avi, webm")
		}
	default:
		return fmt.Errorf("invalid media type. Must be 'image' or 'video'")
	}
	return nil
}

// InitializeStorage creates necessary directories for file storage
func InitializeStorage() error {
	// Create main uploads directory
	if err := os.MkdirAll(uploadBaseDir, 0755); err != nil {
		return fmt.Errorf("failed to create uploads directory: %v", err)
	}

	// Create subdirectories
	dirs := []string{
		filepath.Join(uploadBaseDir, "bookings"),
		filepath.Join(uploadBaseDir, "thumbnails"),
		filepath.Join(uploadBaseDir, "reviews"),
		filepath.Join(uploadBaseDir, "category"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %v", dir, err)
		}
	}

	return nil
}

// UploadFile saves a file to local storage and returns the URL
func UploadFile(fileData []byte, filename string, mediaType string) (string, error) {
	// Validate file size
	if len(fileData) > maxFileSize {
		return "", fmt.Errorf("file too large. Maximum size is %d bytes", maxFileSize)
	}

	// Clean and validate filename
	cleanName := cleanFilename(filename)
	if err := ValidateFileType(cleanName, mediaType); err != nil {
		return "", err
	}

	// Ensure the uploads directory exists
	if err := InitializeStorage(); err != nil {
		return "", err
	}

	// Create full path for the file
	fullPath := filepath.Join(uploadBaseDir, filename)

	// Ensure the directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %v", err)
	}

	// Write the file with restricted permissions
	if err := os.WriteFile(fullPath, fileData, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %v", err)
	}

	// Generate URL (relative to the server)
	url := fmt.Sprintf("%s/%s", baseURL, filename)
	return url, nil
}

// UploadFileToPath saves a file to a specific subdirectory and returns the URL
func UploadFileToPath(fileData []byte, filename string, mediaType string, subDir string) (string, error) {
	// Validate file size
	if len(fileData) > maxFileSize {
		return "", fmt.Errorf("file too large. Maximum size is %d bytes", maxFileSize)
	}

	// Clean and validate filename
	cleanName := cleanFilename(filename)
	if err := ValidateFileType(cleanName, mediaType); err != nil {
		return "", err
	}

	// Create full path for the file in the specified subdirectory
	fullPath := filepath.Join(uploadBaseDir, subDir, filename)

	// Ensure only the specific subdirectory exists (don't create all directories)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %v", filepath.Dir(fullPath), err)
	}

	// Write the file with restricted permissions
	if err := os.WriteFile(fullPath, fileData, 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %v", fullPath, err)
	}

	// Generate URL (relative to the server) - subDir should not include "uploads/"
	// Remove "uploads/" prefix if it exists in subDir
	cleanSubDir := strings.TrimPrefix(subDir, "uploads/")
	url := fmt.Sprintf("%s/%s/%s", baseURL, cleanSubDir, filename)
	return url, nil
}

// GenerateVideoThumbnail generates a thumbnail for a video and saves it locally
func GenerateVideoThumbnail(videoURL string) (string, error) {
	// Ensure the uploads directory exists
	if err := InitializeStorage(); err != nil {
		return "", err
	}

	// Extract video path from URL
	videoPath := strings.TrimPrefix(videoURL, baseURL+"/")
	fullVideoPath := filepath.Join(uploadBaseDir, videoPath)

	// Create a temporary file for the thumbnail
	tempDir := os.TempDir()
	thumbnailPath := filepath.Join(tempDir, "thumbnail.jpg")

	// Generate thumbnail using ffmpeg
	err := ffmpeg.Input(fullVideoPath).
		Output(thumbnailPath, ffmpeg.KwArgs{"vframes": 1, "ss": "00:00:01"}).
		OverWriteOutput().
		Run()
	if err != nil {
		return "", fmt.Errorf("failed to generate thumbnail: %v", err)
	}
	defer os.Remove(thumbnailPath)

	// Read the thumbnail file
	thumbnailData, err := os.ReadFile(thumbnailPath)
	if err != nil {
		return "", fmt.Errorf("failed to read thumbnail file: %v", err)
	}

	// Resize thumbnail if needed
	img, err := imaging.Decode(bytes.NewReader(thumbnailData))
	if err != nil {
		return "", fmt.Errorf("failed to decode thumbnail: %v", err)
	}

	// Resize to max width of 320px while maintaining aspect ratio
	resized := imaging.Resize(img, 320, 0, imaging.Lanczos)

	// Encode as JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, resized, &jpeg.Options{Quality: 85}); err != nil {
		return "", fmt.Errorf("failed to encode thumbnail: %v", err)
	}

	// Generate thumbnail filename
	videoFilename := filepath.Base(videoPath)
	thumbnailFilename := fmt.Sprintf("thumbnails/%s.jpg", strings.TrimSuffix(videoFilename, filepath.Ext(videoFilename)))
	fullThumbnailPath := filepath.Join(uploadBaseDir, thumbnailFilename)

	// Ensure thumbnail directory exists
	if err := os.MkdirAll(filepath.Dir(fullThumbnailPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create thumbnail directory: %v", err)
	}

	// Save thumbnail
	if err := os.WriteFile(fullThumbnailPath, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to save thumbnail: %v", err)
	}

	// Generate thumbnail URL
	thumbnailURL := fmt.Sprintf("%s/%s", baseURL, thumbnailFilename)
	return thumbnailURL, nil
}

// ServeFiles handles serving uploaded files
func ServeFiles(w http.ResponseWriter, r *http.Request) {
	// Get the file path from the URL
	path := strings.TrimPrefix(r.URL.Path, baseURL)
	fullPath := filepath.Join(uploadBaseDir, path)

	// Check if file exists
	info, err := os.Stat(fullPath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Don't allow directory listing
	if info.IsDir() {
		http.NotFound(w, r)
		return
	}

	// Set cache headers
	w.Header().Set("Cache-Control", "public, max-age=31536000") // Cache for 1 year
	w.Header().Set("Expires", time.Now().AddDate(1, 0, 0).Format(time.RFC1123))

	// Serve the file
	http.ServeFile(w, r, fullPath)
}
