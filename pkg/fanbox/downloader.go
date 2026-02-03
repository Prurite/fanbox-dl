package fanbox

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
)

// RobustDownloader provides reliable downloading with chunked resume support
type RobustDownloader struct {
	client        *OfficialAPIClient
	chunkSize     int64
	maxRetries    int
	retryWait     time.Duration
	progressEvery time.Duration
}

// NewRobustDownloader creates a new robust downloader
func NewRobustDownloader(client *OfficialAPIClient) *RobustDownloader {
	return &RobustDownloader{
		client:        client,
		chunkSize:     5 * 1024 * 1024, // 5MB chunks
		maxRetries:    10,
		retryWait:     2 * time.Second,
		progressEvery: 2 * time.Second,
	}
}

// DownloadWithResume downloads a file with automatic resume on failure
func (rd *RobustDownloader) DownloadWithResume(ctx context.Context, url string, filePath string, meta DownloadStateMeta) error {
	// Check if partial download exists
	rangeStart, hasTemp, err := tempFileOffset(filePath)
	if err != nil {
		return fmt.Errorf("check temp file: %w", err)
	}
	if rangeStart < 0 {
		rangeStart = 0
	}

	// Get file size first
	totalSize, supportsRange, err := rd.getFileSize(ctx, url)
	if err != nil {
		return fmt.Errorf("get file size: %w", err)
	}

	slog.InfoContext(ctx, "Starting download",
		"url", url,
		"total_size", formatBytes(totalSize),
		"resume_from", formatBytes(rangeStart),
		"supports_range", supportsRange)

	// Initialize state file
	statePath := stateFilePath(filePath)
	tempPath := tempFilePath(filePath)

	state := DownloadState{
		Version:    1,
		Status:     downloadStateStatusDownloading,
		CreatorID:  meta.CreatorID,
		PostID:     meta.PostID,
		PostTitle:  meta.PostTitle,
		AssetID:    meta.AssetID,
		AssetType:  meta.AssetType,
		URL:        meta.URL,
		FilePath:   filePath,
		Bytes:      rangeStart,
		TotalBytes: totalSize,
		UpdatedAt:  time.Now().Format(time.RFC3339Nano),
	}

	if err := writeStateFile(statePath, state); err != nil {
		slog.WarnContext(ctx, "Failed to write state file", "error", err)
	}

	// Open or create temp file
	flags := os.O_WRONLY | os.O_CREATE
	if hasTemp && rangeStart > 0 && supportsRange {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		rangeStart = 0
	}

	file, err := os.OpenFile(tempPath, flags, 0664)
	if err != nil {
		return fmt.Errorf("open temp file: %w", err)
	}
	closeFile := func() error {
		if file == nil {
			return nil
		}
		if err := file.Close(); err != nil {
			file = nil
			return err
		}
		file = nil
		return nil
	}
	defer func() {
		if err := closeFile(); err != nil {
			slog.WarnContext(ctx, "close temp file", "error", err, "path", tempPath)
		}
	}()

	// Download with chunked resume
	currentOffset := rangeStart
	startTime := time.Now()
	lastProgressTime := time.Now()
	_ = rangeStart // 使用 rangeStart

	for currentOffset < totalSize {
		// Calculate chunk end
		chunkEnd := currentOffset + rd.chunkSize - 1
		if chunkEnd >= totalSize {
			chunkEnd = totalSize - 1
		}

		// Download chunk with retry
		chunkRetries := 0
		for chunkRetries < rd.maxRetries {
			select {
			case <-ctx.Done():
				state.Status = downloadStateStatusFailed
				state.LastError = ctx.Err().Error()
				state.Bytes = currentOffset
				state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
				_ = writeStateFile(statePath, state)
				return ctx.Err()
			default:
			}

			// Create context with timeout for this chunk
			chunkCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)

			bytesWritten, err := rd.downloadChunk(chunkCtx, url, file, currentOffset, chunkEnd)
			cancel()

			if err != nil {
				chunkRetries++
				slog.WarnContext(ctx, "Chunk download failed",
					"offset", currentOffset,
					"end", chunkEnd,
					"attempt", chunkRetries,
					"error", err)

				if chunkRetries < rd.maxRetries {
					waitDur := rd.retryWait * time.Duration(chunkRetries)
					if waitDur > 30*time.Second {
						waitDur = 30 * time.Second
					}
					time.Sleep(waitDur)
					continue
				}

				// Max retries exceeded
				state.Status = downloadStateStatusFailed
				state.LastError = fmt.Sprintf("chunk download failed after %d retries: %v", rd.maxRetries, err)
				state.Bytes = currentOffset
				state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
				_ = writeStateFile(statePath, state)
				return fmt.Errorf("chunk download failed after %d retries: %w", rd.maxRetries, err)
			}

			// Chunk downloaded successfully
			currentOffset += bytesWritten
			break
		}

		// Update state periodically
		now := time.Now()
		if now.Sub(lastProgressTime) >= rd.progressEvery {
			state.Bytes = currentOffset
			state.UpdatedAt = now.Format(time.RFC3339Nano)
			_ = writeStateFile(statePath, state)

			// Log progress
			elapsed := now.Sub(startTime)
			speed := float64(currentOffset-rangeStart) / elapsed.Seconds()
			percentage := float64(currentOffset) / float64(totalSize) * 100

			slog.InfoContext(ctx, "Download progress",
				"downloaded", formatBytes(currentOffset),
				"total", formatBytes(totalSize),
				"progress", fmt.Sprintf("%.1f%%", percentage),
				"speed", formatBytes(int64(speed))+"/s")

			lastProgressTime = now
		}
	}

	// Sync and close file
	if err := file.Sync(); err != nil {
		state.Status = downloadStateStatusFailed
		state.LastError = err.Error()
		state.Bytes = currentOffset
		state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
		_ = writeStateFile(statePath, state)
		return fmt.Errorf("sync file: %w", err)
	}

	if err := closeFile(); err != nil {
		state.Status = downloadStateStatusFailed
		state.LastError = err.Error()
		state.Bytes = currentOffset
		state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
		_ = writeStateFile(statePath, state)
		return fmt.Errorf("close file: %w", err)
	}

	// Rename temp file to final file
	if err := os.Rename(tempPath, filePath); err != nil {
		// Try removing target and retry
		if removeErr := os.Remove(filePath); removeErr == nil {
			if retryErr := os.Rename(tempPath, filePath); retryErr == nil {
				goto renamed
			}
		}
		state.Status = downloadStateStatusFailed
		state.LastError = err.Error()
		state.Bytes = currentOffset
		state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
		_ = writeStateFile(statePath, state)
		return fmt.Errorf("rename file: %w", err)
	}

renamed:
	// Mark as completed
	state.Status = downloadStateStatusCompleted
	state.Bytes = currentOffset
	state.UpdatedAt = time.Now().Format(time.RFC3339Nano)
	_ = writeStateFile(statePath, state)
	_ = os.Remove(statePath)

	elapsed := time.Since(startTime)
	avgSpeed := float64(currentOffset-rangeStart) / elapsed.Seconds()
	slog.InfoContext(ctx, "Download completed",
		"bytes", formatBytes(currentOffset),
		"duration", elapsed.Round(time.Millisecond*100),
		"avg_speed", formatBytes(int64(avgSpeed))+"/s")

	return nil
}

// getFileSize gets the total file size and checks if server supports range requests
func (rd *RobustDownloader) getFileSize(ctx context.Context, url string) (int64, bool, error) {
	resp, err := rd.client.Request(ctx, http.MethodHead, url)
	if err != nil {
		// HEAD not supported, try GET with Range: bytes=0-0
		headers := map[string]string{"Range": "bytes=0-0"}
		resp, err = rd.client.RequestWithHeaders(ctx, http.MethodGet, url, headers)
		if err != nil {
			return 0, false, err
		}
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.WarnContext(ctx, "close response body", "error", err)
		}
	}()

	if resp.StatusCode != 200 && resp.StatusCode != 206 {
		return 0, false, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	contentLength := resp.ContentLength
	if contentLength <= 0 {
		// Try to get from Content-Range header
		if cr := resp.Header.Get("Content-Range"); cr != "" {
			var start, end, total int64
			if _, err := fmt.Sscanf(cr, "bytes %d-%d/%d", &start, &end, &total); err == nil {
				contentLength = total
			}
		}
	}

	supportsRange := resp.Header.Get("Accept-Ranges") == "bytes" || resp.StatusCode == 206

	return contentLength, supportsRange, nil
}

// downloadChunk downloads a specific byte range of a file
func (rd *RobustDownloader) downloadChunk(ctx context.Context, url string, file *os.File, start, end int64) (int64, error) {
	headers := map[string]string{
		"Range": fmt.Sprintf("bytes=%d-%d", start, end),
	}

	resp, err := rd.client.RequestWithHeaders(ctx, http.MethodGet, url, headers)
	if err != nil {
		return 0, fmt.Errorf("request chunk: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != 206 && resp.StatusCode != 200 {
		return 0, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Copy data with buffer
	buf := make([]byte, 32*1024) // 32KB buffer
	var written int64

	for {
		select {
		case <-ctx.Done():
			return written, ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			nw, writeErr := file.Write(buf[:n])
			if writeErr != nil {
				return written, fmt.Errorf("write chunk: %w", writeErr)
			}
			written += int64(nw)
		}

		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return written, fmt.Errorf("read chunk: %w", readErr)
		}
	}

	return written, nil
}
