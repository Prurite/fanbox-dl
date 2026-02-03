package fanbox

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	downloadStateStatusDownloading = "downloading"
	downloadStateStatusCompleted   = "completed"
	downloadStateStatusFailed      = "failed"
)

type DownloadState struct {
	Version     int    `json:"version"`
	Status      string `json:"status"`
	CreatorID   string `json:"creatorId,omitempty"`
	PostID      string `json:"postId,omitempty"`
	PostTitle   string `json:"postTitle,omitempty"`
	AssetID     string `json:"assetId,omitempty"`
	AssetType   string `json:"assetType,omitempty"`
	URL         string `json:"url,omitempty"`
	FilePath    string `json:"filePath"`
	Bytes       int64  `json:"bytes"`
	TotalBytes  int64  `json:"totalBytes,omitempty"`
	UpdatedAt   string `json:"updatedAt"`
	LastError   string `json:"lastError,omitempty"`
}

type DownloadStateMeta struct {
	CreatorID string
	PostID    string
	PostTitle string
	AssetID   string
	AssetType string
	URL       string
}

func stateFilePath(path string) string {
	return path + ".state"
}

func tempFilePath(path string) string {
	return path + ".part"
}

func writeStateFile(path string, state DownloadState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename state file: %w", err)
	}
	return nil
}

func loadStateFile(path string) (DownloadState, error) {
	var state DownloadState
	data, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func hasIncompleteState(path string) bool {
	if _, err := os.Stat(stateFilePath(path)); err == nil {
		return true
	}
	if _, err := os.Stat(tempFilePath(path)); err == nil {
		return true
	}
	return false
}

func tempFileOffset(path string) (int64, bool, error) {
	info, err := os.Stat(tempFilePath(path))
	if os.IsNotExist(err) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return info.Size(), true, nil
}

func findStateByURL(root string, targetURL string) (string, int64, bool, error) {
	var foundPath string
	var foundBytes int64

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return "", 0, false, nil
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".state") {
			return nil
		}
		state, err := loadStateFile(path)
		if err != nil {
			return nil
		}
		if state.URL != "" && state.URL == targetURL {
			foundPath = state.FilePath
			foundBytes = state.Bytes
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return "", 0, false, err
	}
	if foundPath == "" {
		return "", 0, false, nil
	}
	return foundPath, foundBytes, true, nil
}

func isDownloadComplete(path string) (bool, error) {
	if hasIncompleteState(path) {
		return false, nil
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}

func saveReaderWithState(ctx context.Context, path string, r io.Reader, meta DownloadStateMeta, totalBytes int64, fileMode os.FileMode) error {
	return saveReaderWithStateAt(ctx, path, r, meta, totalBytes, fileMode, 0, false)
}

func saveReaderWithStateAt(ctx context.Context, path string, r io.Reader, meta DownloadStateMeta, totalBytes int64, fileMode os.FileMode, startOffset int64, appendMode bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if fileMode == 0 {
		fileMode = 0664
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory (%s): %w", dir, err)
	}

	statePath := stateFilePath(path)
	tempPath := tempFilePath(path)
	if !appendMode {
		_ = os.Remove(tempPath)
	}

	state := DownloadState{
		Version:    1,
		Status:     downloadStateStatusDownloading,
		CreatorID:  meta.CreatorID,
		PostID:     meta.PostID,
		PostTitle:  meta.PostTitle,
		AssetID:    meta.AssetID,
		AssetType:  meta.AssetType,
		URL:        meta.URL,
		FilePath:   path,
		Bytes:      startOffset,
		UpdatedAt:  time.Now().Format(time.RFC3339Nano),
	}
	if totalBytes > 0 {
		state.TotalBytes = totalBytes
	}

	if err := writeStateFile(statePath, state); err != nil {
		return fmt.Errorf("write state file: %w", err)
	}

	flags := os.O_WRONLY | os.O_CREATE
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	file, err := os.OpenFile(tempPath, flags, fileMode)
	if err != nil {
		state.Status = downloadStateStatusFailed
		state.LastError = err.Error()
		state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
		_ = writeStateFile(statePath, state)
		return fmt.Errorf("open temp file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	buf := make([]byte, 32*1024)
	written := startOffset
	lastUpdate := time.Now()
	lastBytes := startOffset
	lastProgressLog := time.Now()

	// Extract filename from path for progress logging
	filename := filepath.Base(path)

	for {
		if ctx.Err() != nil {
			state.Status = downloadStateStatusFailed
			state.LastError = ctx.Err().Error()
			state.Bytes = written
			state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
			_ = writeStateFile(statePath, state)
			return ctx.Err()
		}

		n, readErr := r.Read(buf)
		if n > 0 {
			if _, err := file.Write(buf[:n]); err != nil {
				state.Status = downloadStateStatusFailed
				state.LastError = err.Error()
				state.Bytes = written
				state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
				_ = writeStateFile(statePath, state)
				return fmt.Errorf("write file: %w", err)
			}
			written += int64(n)
			now := time.Now()
			if now.Sub(lastUpdate) >= time.Second || written-lastBytes >= 1<<20 {
				state.Bytes = written
				state.UpdatedAt = now.Format(time.RFC3339Nano)
				_ = writeStateFile(statePath, state)
				lastUpdate = now
				lastBytes = written

				// Log progress every 2 seconds
				if now.Sub(lastProgressLog) >= 2*time.Second {
					logProgress(filename, written, totalBytes)
					lastProgressLog = now
				}
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			state.Status = downloadStateStatusFailed
			state.LastError = readErr.Error()
			state.Bytes = written
			state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
			_ = writeStateFile(statePath, state)
			return fmt.Errorf("read stream: %w", readErr)
		}
	}

	// Final progress log
	logProgress(filename, written, totalBytes)

	if err := file.Sync(); err != nil {
		state.Status = downloadStateStatusFailed
		state.LastError = err.Error()
		state.Bytes = written
		state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
		_ = writeStateFile(statePath, state)
		return fmt.Errorf("sync file: %w", err)
	}
	if err := file.Close(); err != nil {
		state.Status = downloadStateStatusFailed
		state.LastError = err.Error()
		state.Bytes = written
		state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
		_ = writeStateFile(statePath, state)
		return fmt.Errorf("close file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		if removeErr := os.Remove(path); removeErr == nil {
			if retryErr := os.Rename(tempPath, path); retryErr == nil {
				goto renamed
			}
		}
		state.Status = downloadStateStatusFailed
		state.LastError = err.Error()
		state.Bytes = written
		state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
		_ = writeStateFile(statePath, state)
		return fmt.Errorf("rename file: %w", err)
	}
renamed:

	state.Status = downloadStateStatusCompleted
	state.Bytes = written
	state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	_ = writeStateFile(statePath, state)
	_ = os.Remove(statePath)

	return nil
}

func logProgress(filename string, current int64, total int64) {
	if total > 0 {
		percentage := float64(current) / float64(total) * 100
		slog.Info("Download progress",
			"filename", filename,
			"downloaded", formatBytes(current),
			"total", formatBytes(total),
			"progress", fmt.Sprintf("%.1f%%", percentage))
	} else {
		slog.Info("Download progress",
			"filename", filename,
			"downloaded", formatBytes(current))
	}
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// CleanupOrphanedStates removes orphaned .state and .part files
// Returns the number of cleaned files
func CleanupOrphanedStates(root string) (int, error) {
	cleaned := 0

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return 0, nil
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Clean orphaned .state files (when both target file and part file don't exist)
		if strings.HasSuffix(path, ".state") {
			targetPath := strings.TrimSuffix(path, ".state")
			partPath := targetPath + ".part"
			_, targetExists := os.Stat(targetPath)
			_, partExists := os.Stat(partPath)
			// Only remove if neither target nor part file exists
			if os.IsNotExist(targetExists) && os.IsNotExist(partExists) {
				if err := os.Remove(path); err == nil {
					cleaned++
					slog.Debug("Removed orphaned state file", "path", path)
				}
			}
			return nil
		}

		// Clean orphaned .part files (when no .state file exists)
		if strings.HasSuffix(path, ".part") {
			statePath := strings.TrimSuffix(path, ".part") + ".state"
			if _, err := os.Stat(statePath); os.IsNotExist(err) {
				if err := os.Remove(path); err == nil {
					cleaned++
					slog.Debug("Removed orphaned part file", "path", path)
				}
			}
			return nil
		}

		return nil
	})

	if err != nil {
		return 0, fmt.Errorf("walk directory: %w", err)
	}

	return cleaned, nil
}

// GetIncompleteDownloads returns a list of incomplete downloads
func GetIncompleteDownloads(root string) ([]DownloadState, error) {
	var incomplete []DownloadState

	if _, err := os.Stat(root); os.IsNotExist(err) {
		return nil, nil
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".state") {
			return nil
		}

		state, err := loadStateFile(path)
		if err != nil {
			slog.Debug("Failed to load state file", "path", path, "error", err)
			return nil
		}

		if state.Status == downloadStateStatusDownloading {
			incomplete = append(incomplete, state)
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return incomplete, nil
}
