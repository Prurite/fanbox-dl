// +build integration

package fanbox

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestDriveDownloaderRealFile tests downloading a real Google Drive file
// Run with: go test -tags=integration -v ./pkg/fanbox -run TestDriveDownloaderRealFile
// Requires TEST_DRIVE_FILE_ID environment variable to be set
func TestDriveDownloaderRealFile(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get test file ID from environment variable
	fileID := os.Getenv("TEST_DRIVE_FILE_ID")
	if fileID == "" {
		t.Skip("Skipping integration test: TEST_DRIVE_FILE_ID environment variable not set")
	}
	driveURL := "https://drive.google.com/file/d/" + fileID + "/view?usp=sharing"

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	downloader := NewDriveDownloader(client, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	saveDir := t.TempDir()

	t.Logf("Downloading from: %s", driveURL)
	t.Logf("Saving to: %s", saveDir)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	err := downloader.DownloadFile(ctx, driveURL, saveDir, DownloadStateMeta{
		CreatorID: "test-creator",
		PostID:    "test-post",
		PostTitle: "Integration Test",
		AssetType: "drive",
		URL:       driveURL,
	})

	if err != nil {
		t.Fatalf("DownloadFile() error: %v", err)
	}

	// List downloaded files
	files, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("No files were downloaded")
	}

	t.Logf("Downloaded %d file(s):", len(files))
	for _, file := range files {
		info, err := file.Info()
		if err != nil {
			t.Logf("  - %s (error getting info: %v)", file.Name(), err)
			continue
		}
		t.Logf("  - %s (%d bytes)", file.Name(), info.Size())

		// Verify file is not empty
		if info.Size() == 0 {
			t.Errorf("Downloaded file %s is empty", file.Name())
		}

		// Check for state files (should be cleaned up after successful download)
		if filepath.Ext(file.Name()) == ".state" {
			t.Errorf("State file %s was not cleaned up", file.Name())
		}
		if filepath.Ext(file.Name()) == ".part" {
			t.Errorf("Partial file %s was not cleaned up", file.Name())
		}
	}
}

// TestDriveDownloaderRealFileResume tests resuming a download
// Run with: go test -tags=integration -v ./pkg/fanbox -run TestDriveDownloaderRealFileResume
// Requires TEST_DRIVE_FILE_ID environment variable to be set
func TestDriveDownloaderRealFileResume(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Get test file ID from environment variable
	fileID := os.Getenv("TEST_DRIVE_FILE_ID")
	if fileID == "" {
		t.Skip("Skipping integration test: TEST_DRIVE_FILE_ID environment variable not set")
	}
	driveURL := "https://drive.google.com/file/d/" + fileID + "/view?usp=sharing"

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	downloader := NewDriveDownloader(client, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	saveDir := t.TempDir()

	// First download - cancel after 1 second
	ctx1, cancel1 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel1()

	t.Log("Starting first download (will be cancelled)...")
	err1 := downloader.DownloadFile(ctx1, driveURL, saveDir, DownloadStateMeta{
		CreatorID: "test-creator",
		PostID:    "test-post",
		PostTitle: "Resume Test",
		AssetType: "drive",
		URL:       driveURL,
	})

	// We expect a timeout or context cancelled error
	if err1 == nil {
		t.Log("First download completed (file might be very small)")
	} else {
		t.Logf("First download cancelled as expected: %v", err1)
	}

	// Check for partial files
	files1, _ := os.ReadDir(saveDir)
	hasPartialFile := false
	for _, file := range files1 {
		if filepath.Ext(file.Name()) == ".part" || filepath.Ext(file.Name()) == ".state" {
			hasPartialFile = true
			t.Logf("Found partial/state file: %s", file.Name())
		}
	}

	// Second download - complete it
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel2()

	t.Log("Starting second download (resume)...")
	err2 := downloader.DownloadFile(ctx2, driveURL, saveDir, DownloadStateMeta{
		CreatorID: "test-creator",
		PostID:    "test-post",
		PostTitle: "Resume Test",
		AssetType: "drive",
		URL:       driveURL,
	})

	if err2 != nil {
		t.Fatalf("Second download failed: %v", err2)
	}

	// Verify final state
	files2, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	completedFiles := 0
	for _, file := range files2 {
		ext := filepath.Ext(file.Name())
		if ext != ".state" && ext != ".part" {
			completedFiles++
			info, _ := file.Info()
			t.Logf("Completed file: %s (%d bytes)", file.Name(), info.Size())
		}
	}

	if completedFiles == 0 {
		t.Fatal("No completed files found after resume")
	}

	if hasPartialFile {
		t.Log("Resume functionality was tested")
	} else {
		t.Log("File was too small to test resume, but download succeeded")
	}
}
