package fanbox

import (
	"context"
	"fmt"
	"io"
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
	// Pattern: https://\d{1,2}\.gigafile\.nu/\d{4}-[a-f0-9]{32}
	pattern := `https://\d{1,2}\.gigafile\.nu/\d{4}-[a-f0-9]{32}`
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
func (g *GigafileDownloader) DownloadFile(ctx context.Context, gigafileURL string, saveDir string) error {
	// Convert to download URL
	// Original: https://12.gigafile.nu/1234-abcdef...
	// Download: https://12.gigafile.nu/download.php?file=1234-abcdef...

	downloadURL := strings.Replace(gigafileURL, "nu/", "nu/download.php?file=", 1)

	// Validate the URL
	if _, err := url.Parse(downloadURL); err != nil {
		return fmt.Errorf("parse URL: %w", err)
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

	// Download the file
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Referer", "https://www.fanbox.cc/")
	req.Header.Set("Cookie", fmt.Sprintf("gfsid=%s", cookie))

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Save the file
	savePath := saveDir + "/" + filename
	outFile, err := createFile(savePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, resp.Body)
	if err != nil {
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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

// createFile is a helper to create a file (can be mocked for testing)
var createFile = func(path string) (*os.File, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return os.Create(path)
}
