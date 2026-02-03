package fanbox

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"google.golang.org/api/drive/v3"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// DriveAPIDownloader uses Google Drive API v3 for downloading files
type DriveAPIDownloader struct {
	service   *drive.Service
	userAgent string
	apiKey    bool // whether API key is configured
}

// NewDriveAPIDownloader creates a new API-based downloader
// This requires no authentication for public files
// API key can be provided via environment variable GOOGLE_DRIVE_API_KEY
func NewDriveAPIDownloader(httpClient *http.Client, userAgent string) (*DriveAPIDownloader, error) {
	ctx := context.Background()

	// Check for API key in environment
	apiKey := os.Getenv("GOOGLE_DRIVE_API_KEY")

	// Build options for Drive service
	opts := []option.ClientOption{
		option.WithHTTPClient(httpClient),
		option.WithUserAgent(userAgent),
	}
	if apiKey != "" {
		opts = append(opts, option.WithAPIKey(apiKey))
	}

	// Create Drive service with custom HTTP client
	service, err := drive.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create drive service: %w", err)
	}

	return &DriveAPIDownloader{
		service:   service,
		userAgent: userAgent,
		apiKey:    apiKey != "",
	}, nil
}

// DownloadFile downloads a file using Google Drive API v3
func (d *DriveAPIDownloader) DownloadFile(ctx context.Context, fileID string, saveDir string, meta DownloadStateMeta) error {
	if d.apiKey {
		slog.Info("Attempting download via Google Drive API with API key", "file_id", fileID)
	} else {
		slog.Debug("Attempting download via Google Drive API (no API key)", "file_id", fileID)
	}

	// Get file metadata first
	file, err := d.service.Files.Get(fileID).
		Context(ctx).
		Fields("id, name, size, mimeType").
		SupportsAllDrives(true).
		Do()
	if err != nil {
		if apiErr, ok := err.(*googleapi.Error); ok {
			if apiErr.Code == 404 {
				return fmt.Errorf("file not found or not accessible: %w", err)
			}
			if apiErr.Code == 403 {
				if d.apiKey {
					return fmt.Errorf("access denied (file may be private): %w", err)
				}
				return fmt.Errorf("API key required (set GOOGLE_DRIVE_API_KEY environment variable): %w", err)
			}
		}
		return fmt.Errorf("get file metadata: %w", err)
	}

	slog.Info("Retrieved file metadata",
		"filename", file.Name,
		"size", formatBytes(file.Size),
		"mime_type", file.MimeType)

	// Check if file is a Google Docs file (needs export)
	if isGoogleDocsFile(file.MimeType) {
		return fmt.Errorf("Google Docs files (Docs/Sheets/Slides) are not supported for direct download")
	}

	// Prepare save path
	filename := file.Name
	if filename == "" {
		filename = fileID + ".bin"
	}
	savePath := filepath.Join(saveDir, filename)

	// Check if already downloaded
	complete, err := isDownloadComplete(savePath)
	if err != nil {
		return fmt.Errorf("check file: %w", err)
	}
	if complete {
		slog.Info("Google Drive file already downloaded", "filename", filename)
		return nil
	}

	// Check for resume capability
	var resumeOffset int64
	if size, hasTemp, err := tempFileOffset(savePath); err == nil && hasTemp {
		resumeOffset = size
		slog.Info("Resuming download from offset", "offset", formatBytes(resumeOffset))
	}

	// Download file content
	req := d.service.Files.Get(fileID).
		Context(ctx).
		SupportsAllDrives(true)

	resp, err := req.Download()
	if err != nil {
		return fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Update metadata
	if meta.AssetType == "" {
		meta.AssetType = "drive"
	}
	if meta.AssetID == "" {
		meta.AssetID = fileID
	}

	// Save file with progress tracking and resume support
	if resumeOffset > 0 && resp.StatusCode == http.StatusPartialContent {
		if err := saveReaderWithStateAt(ctx, savePath, resp.Body, meta, file.Size, 0666, resumeOffset, true); err != nil {
			return fmt.Errorf("save file: %w", err)
		}
	} else {
		if err := saveReaderWithState(ctx, savePath, resp.Body, meta, file.Size, 0666); err != nil {
			return fmt.Errorf("save file: %w", err)
		}
	}

	slog.Info("Downloaded Google Drive file via API", "filename", filename, "size", formatBytes(file.Size))
	return nil
}

func isGoogleDocsFile(mimeType string) bool {
	googleDocsMimeTypes := []string{
		"application/vnd.google-apps.document",
		"application/vnd.google-apps.spreadsheet",
		"application/vnd.google-apps.presentation",
		"application/vnd.google-apps.drawing",
		"application/vnd.google-apps.form",
		"application/vnd.google-apps.site",
	}
	for _, t := range googleDocsMimeTypes {
		if mimeType == t {
			return true
		}
	}
	return false
}
