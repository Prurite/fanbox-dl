package fanbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteAndLoadStateFile(t *testing.T) {
	tempDir := t.TempDir()
	statePath := filepath.Join(tempDir, "test.state")

	state := DownloadState{
		Version:     1,
		Status:      downloadStateStatusDownloading,
		CreatorID:   "test_creator",
		PostID:      "12345678",
		PostTitle:   "Test Post",
		AssetID:     "asset123",
		AssetType:   "image",
		URL:         "https://example.com/file.jpg",
		FilePath:    filepath.Join(tempDir, "file.jpg"),
		Bytes:       1024,
		TotalBytes:  2048,
		UpdatedAt:   time.Now().Format(time.RFC3339Nano),
	}

	err := writeStateFile(statePath, state)
	if err != nil {
		t.Fatalf("writeStateFile() error: %v", err)
	}

	loaded, err := loadStateFile(statePath)
	if err != nil {
		t.Fatalf("loadStateFile() error: %v", err)
	}

	if loaded.Version != state.Version {
		t.Errorf("Version = %d, expected %d", loaded.Version, state.Version)
	}
	if loaded.Status != state.Status {
		t.Errorf("Status = %s, expected %s", loaded.Status, state.Status)
	}
	if loaded.CreatorID != state.CreatorID {
		t.Errorf("CreatorID = %s, expected %s", loaded.CreatorID, state.CreatorID)
	}
	if loaded.PostID != state.PostID {
		t.Errorf("PostID = %s, expected %s", loaded.PostID, state.PostID)
	}
	if loaded.AssetID != state.AssetID {
		t.Errorf("AssetID = %s, expected %s", loaded.AssetID, state.AssetID)
	}
	if loaded.Bytes != state.Bytes {
		t.Errorf("Bytes = %d, expected %d", loaded.Bytes, state.Bytes)
	}
}

func TestHasIncompleteState(t *testing.T) {
	tempDir := t.TempDir()

	// No state file
	path1 := filepath.Join(tempDir, "file1.jpg")
	if hasIncompleteState(path1) {
		t.Error("hasIncompleteState() = true, expected false for file without state")
	}

	// Has state file
	path2 := filepath.Join(tempDir, "file2.jpg")
	statePath2 := stateFilePath(path2)
	err := os.WriteFile(statePath2, []byte(`{"status":"downloading"}`), 0644)
	if err != nil {
		t.Fatalf("create state file: %v", err)
	}
	if !hasIncompleteState(path2) {
		t.Error("hasIncompleteState() = false, expected true for file with state")
	}

	// Has part file
	path3 := filepath.Join(tempDir, "file3.jpg")
	partPath3 := tempFilePath(path3)
	err = os.WriteFile(partPath3, []byte("partial"), 0644)
	if err != nil {
		t.Fatalf("create part file: %v", err)
	}
	if !hasIncompleteState(path3) {
		t.Error("hasIncompleteState() = false, expected true for file with part")
	}
}

func TestTempFileOffset(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "test.jpg")
	partPath := tempFilePath(path)

	// No part file
	offset, hasTemp, err := tempFileOffset(path)
	if err != nil {
		t.Fatalf("tempFileOffset() error: %v", err)
	}
	if hasTemp {
		t.Error("tempFileOffset() hasTemp = true, expected false")
	}
	if offset != 0 {
		t.Errorf("tempFileOffset() offset = %d, expected 0", offset)
	}

	// Has part file with content
	testData := make([]byte, 4096)
	for i := range testData {
		testData[i] = byte(i % 256)
	}
	err = os.WriteFile(partPath, testData, 0644)
	if err != nil {
		t.Fatalf("create part file: %v", err)
	}

	offset, hasTemp, err = tempFileOffset(path)
	if err != nil {
		t.Fatalf("tempFileOffset() error: %v", err)
	}
	if !hasTemp {
		t.Error("tempFileOffset() hasTemp = false, expected true")
	}
	if offset != int64(len(testData)) {
		t.Errorf("tempFileOffset() offset = %d, expected %d", offset, len(testData))
	}
}

func TestFindStateByURL(t *testing.T) {
	tempDir := t.TempDir()

	// Create subdirectory structure
	subDir := filepath.Join(tempDir, "creator", "post123")
	err := os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatalf("create directory: %v", err)
	}

	// Create state file with specific URL
	targetURL := "https://example.com/test.jpg"
	filePath := filepath.Join(subDir, "test.jpg")
	statePath := stateFilePath(filePath)

	state := DownloadState{
		Version:   1,
		Status:    downloadStateStatusDownloading,
		URL:       targetURL,
		FilePath:  filePath,
		Bytes:     1024,
		UpdatedAt: time.Now().Format(time.RFC3339Nano),
	}
	err = writeStateFile(statePath, state)
	if err != nil {
		t.Fatalf("write state file: %v", err)
	}

	// Test finding the state
	foundPath, bytes, found, err := findStateByURL(tempDir, targetURL)
	if err != nil {
		t.Fatalf("findStateByURL() error: %v", err)
	}
	if !found {
		t.Error("findStateByURL() found = false, expected true")
	}
	if foundPath != filePath {
		t.Errorf("findStateByURL() path = %s, expected %s", foundPath, filePath)
	}
	if bytes != 1024 {
		t.Errorf("findStateByURL() bytes = %d, expected 1024", bytes)
	}

	// Test with non-existent URL
	_, _, found, err = findStateByURL(tempDir, "https://example.com/notfound.jpg")
	if err != nil {
		t.Fatalf("findStateByURL() error: %v", err)
	}
	if found {
		t.Error("findStateByURL() found = true, expected false for non-existent URL")
	}
}

func TestIsDownloadComplete(t *testing.T) {
	tempDir := t.TempDir()

	// No file
	path1 := filepath.Join(tempDir, "notexist.jpg")
	complete, err := isDownloadComplete(path1)
	if err != nil {
		t.Fatalf("isDownloadComplete() error: %v", err)
	}
	if complete {
		t.Error("isDownloadComplete() = true, expected false for non-existent file")
	}

	// Has state file (incomplete)
	path2 := filepath.Join(tempDir, "incomplete.jpg")
	_ = os.WriteFile(stateFilePath(path2), []byte(`{}`), 0644)
	complete, err = isDownloadComplete(path2)
	if err != nil {
		t.Fatalf("isDownloadComplete() error: %v", err)
	}
	if complete {
		t.Error("isDownloadComplete() = true, expected false for file with state")
	}

	// Has part file (incomplete)
	path3 := filepath.Join(tempDir, "incomplete2.jpg")
	_ = os.WriteFile(tempFilePath(path3), []byte("data"), 0644)
	complete, err = isDownloadComplete(path3)
	if err != nil {
		t.Fatalf("isDownloadComplete() error: %v", err)
	}
	if complete {
		t.Error("isDownloadComplete() = true, expected false for file with part")
	}

	// Complete file
	path4 := filepath.Join(tempDir, "complete.jpg")
	err = os.WriteFile(path4, []byte("complete"), 0644)
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	complete, err = isDownloadComplete(path4)
	if err != nil {
		t.Fatalf("isDownloadComplete() error: %v", err)
	}
	if !complete {
		t.Error("isDownloadComplete() = false, expected true for complete file")
	}
}

func TestCleanupOrphanedStates(t *testing.T) {
	tempDir := t.TempDir()

	// Create test directory structure
	subDir := filepath.Join(tempDir, "creator", "post")
	err := os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatalf("create directory: %v", err)
	}

	// Create orphaned state file (no target file)
	orphanedState := filepath.Join(subDir, "orphaned.jpg.state")
	err = os.WriteFile(orphanedState, []byte(`{}`), 0644)
	if err != nil {
		t.Fatalf("create orphaned state: %v", err)
	}

	// Create orphaned part file (no state file)
	orphanedPart := filepath.Join(subDir, "orphaned2.jpg.part")
	err = os.WriteFile(orphanedPart, []byte("data"), 0644)
	if err != nil {
		t.Fatalf("create orphaned part: %v", err)
	}

	// Create valid state file pair (state file + part file = active download)
	validPath := filepath.Join(subDir, "valid.jpg")
	validState := stateFilePath(validPath)
	validPart := tempFilePath(validPath)
	err = os.WriteFile(validState, []byte(`{}`), 0644)
	if err != nil {
		t.Fatalf("create valid state: %v", err)
	}
	err = os.WriteFile(validPart, []byte("partial"), 0644)
	if err != nil {
		t.Fatalf("create valid part: %v", err)
	}

	// Clean up
	cleaned, err := CleanupOrphanedStates(tempDir)
	if err != nil {
		t.Fatalf("CleanupOrphanedStates() error: %v", err)
	}

	if cleaned != 2 {
		t.Errorf("CleanupOrphanedStates() cleaned = %d, expected 2", cleaned)
	}

	// Verify orphaned files are gone
	if _, err := os.Stat(orphanedState); !os.IsNotExist(err) {
		t.Error("orphaned state file still exists")
	}
	if _, err := os.Stat(orphanedPart); !os.IsNotExist(err) {
		t.Error("orphaned part file still exists")
	}

	// Verify valid state file still exists
	if _, err := os.Stat(validState); err != nil {
		t.Error("valid state file was removed")
	}
	// Verify valid part file still exists
	if _, err := os.Stat(validPart); err != nil {
		t.Error("valid part file was removed")
	}
}

func TestGetIncompleteDownloads(t *testing.T) {
	tempDir := t.TempDir()

	// Create test directory structure
	subDir := filepath.Join(tempDir, "creator", "post")
	err := os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatalf("create directory: %v", err)
	}

	// Create incomplete download state
	incompletePath := filepath.Join(subDir, "incomplete.jpg")
	incompleteState := stateFilePath(incompletePath)
	incompleteStateData := DownloadState{
		Version:    1,
		Status:     downloadStateStatusDownloading,
		CreatorID:  "creator1",
		PostID:     "123",
		URL:        "https://example.com/incomplete.jpg",
		FilePath:   incompletePath,
		Bytes:      512,
		UpdatedAt:   time.Now().Format(time.RFC3339Nano),
	}
	err = writeStateFile(incompleteState, incompleteStateData)
	if err != nil {
		t.Fatalf("write incomplete state: %v", err)
	}

	// Create completed download state
	completedPath := filepath.Join(subDir, "completed.jpg")
	completedState := stateFilePath(completedPath)
	completedStateData := DownloadState{
		Version:    1,
		Status:     downloadStateStatusCompleted,
		CreatorID:  "creator1",
		PostID:     "456",
		URL:        "https://example.com/completed.jpg",
		FilePath:   completedPath,
		Bytes:      1024,
		UpdatedAt:   time.Now().Format(time.RFC3339Nano),
	}
	err = writeStateFile(completedState, completedStateData)
	if err != nil {
		t.Fatalf("write completed state: %v", err)
	}

	// Get incomplete downloads
	incomplete, err := GetIncompleteDownloads(tempDir)
	if err != nil {
		t.Fatalf("GetIncompleteDownloads() error: %v", err)
	}

	if len(incomplete) != 1 {
		t.Errorf("GetIncompleteDownloads() count = %d, expected 1", len(incomplete))
	}

	if len(incomplete) > 0 {
		if incomplete[0].CreatorID != "creator1" {
			t.Errorf("CreatorID = %s, expected creator1", incomplete[0].CreatorID)
		}
		if incomplete[0].Status != downloadStateStatusDownloading {
			t.Errorf("Status = %s, expected downloading", incomplete[0].Status)
		}
		if incomplete[0].Bytes != 512 {
			t.Errorf("Bytes = %d, expected 512", incomplete[0].Bytes)
		}
	}
}

func TestSaveReaderWithState(t *testing.T) {
	tempDir := t.TempDir()
	savePath := filepath.Join(tempDir, "test.txt")

	// Save with state
	meta := DownloadStateMeta{
		CreatorID: "test_creator",
		PostID:    "123",
		PostTitle:  "Test Post",
		AssetID:    "asset123",
		AssetType:  "file",
		URL:        "https://example.com/test.txt",
	}

	testContent := strings.Repeat("Hello, World!\n", 100)
	reader := strings.NewReader(testContent)

	err := saveReaderWithState(nil, savePath, reader, meta, int64(len(testContent)), 0644)
	if err != nil {
		t.Fatalf("saveReaderWithState() error: %v", err)
	}

	// Verify file was saved
	data, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != testContent {
		t.Error("saved content doesn't match expected")
	}

	// Verify state file was cleaned up
	statePath := stateFilePath(savePath)
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Error("state file was not cleaned up")
	}

	// Verify part file was cleaned up
	partPath := tempFilePath(savePath)
	if _, err := os.Stat(partPath); !os.IsNotExist(err) {
		t.Error("part file was not cleaned up")
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		bytes    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024 * 1024 * 1.5, "1.5 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1024 * 1024 * 1024 * 2.5, "2.5 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TiB"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatBytes(tt.bytes)
			if result != tt.expected {
				t.Errorf("formatBytes(%d) = %s, expected %s", tt.bytes, result, tt.expected)
			}
		})
	}
}
