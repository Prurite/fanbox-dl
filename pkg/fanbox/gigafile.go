package fanbox

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GigafileDownloader handles downloading files from gigafile.nu
type GigafileDownloader struct {
	httpClient *http.Client
	userAgent  string
}

// NewGigafileDownloader creates a new GigafileDownloader
func NewGigafileDownloader(httpClient *http.Client, userAgent string) *GigafileDownloader {
	return &GigafileDownloader{
		httpClient: httpClient,
		userAgent:  userAgent,
	}
}

// ExtractGigafileURLs extracts all gigafile URLs from a string
func ExtractGigafileURLs(content string) []string {
	// Pattern: https://\d{1,2}\.gigafile\.nu/\d{4}-[a-f0-9]{40}
	// Note: The hash is 40 characters (4 digits + 32 hex chars + 4 more hex chars)
	pattern := `https://\d{1,2}\.gigafile\.nu/\d{4}-[a-f0-9]+`
	re := regexp.MustCompile(pattern)

	matches := re.FindAllString(content, -1)

	// Deduplicate
	unique := make([]string, 0)
	seen := make(map[string]bool)
	for _, match := range matches {
		if !seen[match] {
			seen[match] = true
			unique = append(unique, match)
		}
	}

	return unique
}

// DownloadFile downloads a file from a gigafile URL
func (g *GigafileDownloader) DownloadFile(ctx context.Context, gigafileURL string, saveDir string, meta DownloadStateMeta) error {
	// Convert to download URL
	// Original: https://12.gigafile.nu/1234-abcdef...
	// Download: https://12.gigafile.nu/download.php?file=1234-abcdef...

	downloadURL := strings.Replace(gigafileURL, "nu/", "nu/download.php?file=", 1)

	// Validate the URL
	parsedURL, err := url.Parse(downloadURL)
	if err != nil {
		return fmt.Errorf("parse URL: %w", err)
	}
	if meta.AssetType == "" {
		meta.AssetType = "gigafile"
	}
	if meta.AssetID == "" {
		meta.AssetID = strings.TrimPrefix(parsedURL.Query().Get("file"), "/")
	}

	// First, get the gfsid cookie from the original URL
	cookie, err := g.getCookieFromServer(ctx, gigafileURL, "gfsid")
	if err != nil {
		return fmt.Errorf("get gfsid cookie: %w", err)
	}

	// Get filename from download URL
	filename, err := g.getFilenameFromServer(ctx, downloadURL)
	if err != nil {
		return fmt.Errorf("get filename: %w", err)
	}

	if filename == "" {
		slog.Warn("File expired or filename not available", "url", gigafileURL)
		return fmt.Errorf("file expired: %s", gigafileURL)
	}

	savePath := filepath.Join(saveDir, filename)
	complete, err := isDownloadComplete(savePath)
	if err != nil {
		return fmt.Errorf("check file: %w", err)
	}
	if complete {
		slog.Info("Gigafile already downloaded", "filename", filename, "url", gigafileURL)
		return nil
	}

	rangeStart, hasTemp, err := tempFileOffset(savePath)
	if err != nil {
		return fmt.Errorf("check temp file: %w", err)
	}
	if rangeStart <= 0 {
		hasTemp = false
	}

	// Download the file
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Referer", "https://www.fanbox.cc/")
	req.Header.Set("Cookie", fmt.Sprintf("gfsid=%s", cookie))
	if hasTemp && rangeStart > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", rangeStart))
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download file: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode == http.StatusPartialContent {
		if !hasTemp || rangeStart <= 0 {
			return fmt.Errorf("unexpected partial content response")
		}
	} else if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Save the file
	meta.URL = gigafileURL
	if resp.StatusCode == http.StatusOK && hasTemp && rangeStart > 0 {
		if err := os.Remove(tempFilePath(savePath)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove temp file: %w", err)
		}
		hasTemp = false
		rangeStart = 0
	}

	if hasTemp && rangeStart > 0 {
		if err := saveReaderWithStateAt(ctx, savePath, resp.Body, meta, resp.ContentLength, 0666, rangeStart, true); err != nil {
			return fmt.Errorf("save file: %w", err)
		}
	} else if err := saveReaderWithState(ctx, savePath, resp.Body, meta, resp.ContentLength, 0666); err != nil {
		return fmt.Errorf("save file: %w", err)
	}

	slog.Info("Downloaded gigafile", "filename", filename, "url", gigafileURL)
	return nil
}

// getCookieFromServer extracts a specific cookie from the server
func (g *GigafileDownloader) getCookieFromServer(ctx context.Context, urlStr string, cookieName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Referer", "https://www.fanbox.cc/")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send head request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("head request failed with status %d", resp.StatusCode)
	}

	// Parse Set-Cookie header
	for _, cookieHeader := range resp.Cookies() {
		if cookieHeader.Name == cookieName {
			return cookieHeader.Value, nil
		}
	}

	// Also check the Set-Cookie header directly
	setCookie := resp.Header.Get("Set-Cookie")
	if setCookie != "" {
		pattern := cookieName + `=([^;]+)`
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(setCookie); len(matches) > 1 {
			return matches[1], nil
		}
	}

	return "", fmt.Errorf("cookie %s not found", cookieName)
}

// getFilenameFromServer extracts the filename from Content-Disposition header
func (g *GigafileDownloader) getFilenameFromServer(ctx context.Context, urlStr string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, urlStr, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Referer", "https://www.fanbox.cc/")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send head request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("head request failed with status %d", resp.StatusCode)
	}

	// Get filename from Content-Disposition header
	contentDisposition := resp.Header.Get("Content-Disposition")
	if contentDisposition == "" {
		return "", nil
	}

	// Parse filename*=UTF-8''filename or filename="filename"
	pattern := `filename\*?=(?:UTF-8''|")?([^";]+)(?:")?`
	re := regexp.MustCompile(pattern)
	if matches := re.FindStringSubmatch(contentDisposition); len(matches) > 1 {
		filename := matches[1]
		// URL decode if needed
		if unescaped, err := url.QueryUnescape(filename); err == nil {
			return unescaped, nil
		}
		return filename, nil
	}

	return "", fmt.Errorf("filename not found in Content-Disposition")
}
