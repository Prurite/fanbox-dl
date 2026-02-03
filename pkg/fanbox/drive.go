package fanbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// DriveDownloader handles downloading files from Google Drive
type DriveDownloader struct {
	httpClient *http.Client
	userAgent  string
}

const driveDownloadMaxAttempts = 10

var driveDownloadRetryDelay = 500 * time.Millisecond

var ErrDriveAccessDenied = errors.New("google drive access denied")

// NewDriveDownloader creates a new DriveDownloader
func NewDriveDownloader(httpClient *http.Client, userAgent string) *DriveDownloader {
	return &DriveDownloader{
		httpClient: httpClient,
		userAgent:  userAgent,
	}
}

// ExtractDriveURLs extracts all Google Drive file URLs from a string
func ExtractDriveURLs(content string) []string {
	// Pattern: Matches various Google Drive URL formats:
	// - https://drive.google.com/file/d/FILE_ID
	// - https://drive.google.com/file/d/FILE_ID/view
	// - https://drive.google.com/file/d/FILE_ID/view?usp=sharing
	// - drive.google.com/file/d/FILE_ID (with or without https://)
	// - https://drive.google.com/open?id=FILE_ID
	// - https://drive.google.com/uc?id=FILE_ID&export=download
	// Pattern ends at whitespace, quote, or end of string
	pattern := `(?i)(?:https?://)?drive\.google\.com/(?:file/d/[a-zA-Z0-9_-]+(?:/[^\s'"<>]*)?|open\?[^\s'"<>]*id=[a-zA-Z0-9_-]+[^\s'"<>]*|uc\?[^\s'"<>]*id=[a-zA-Z0-9_-]+[^\s'"<>]*)`
	re := regexp.MustCompile(pattern)

	matches := re.FindAllString(content, -1)

	// Deduplicate by file ID
	unique := make([]string, 0)
	seen := make(map[string]bool)
	fileIDPattern := regexp.MustCompile(`(?i)(?:/file/d/|[?&]id=)([a-zA-Z0-9_-]+)`)

	for _, match := range matches {
		// Extract file ID for deduplication
		idMatches := fileIDPattern.FindStringSubmatch(match)
		if len(idMatches) > 1 {
			fileID := idMatches[1]
			if !seen[fileID] {
				seen[fileID] = true
				unique = append(unique, match)
			}
		}
	}

	return unique
}

// DownloadFile downloads a file from a Google Drive URL
func (g *DriveDownloader) DownloadFile(ctx context.Context, driveURL string, saveDir string, meta DownloadStateMeta) error {
	// Extract file ID from URL
	fileID, err := g.extractFileID(driveURL)
	if err != nil {
		return fmt.Errorf("extract file ID: %w", err)
	}
	if meta.AssetType == "" {
		meta.AssetType = "drive"
	}
	if meta.AssetID == "" {
		meta.AssetID = fileID
	}

	// Try official Google Drive API first
	apiDownloader, err := NewDriveAPIDownloader(g.httpClient, g.userAgent)
	if err == nil {
		slog.Debug("Trying Google Drive API v3", "file_id", fileID)
		if apiErr := apiDownloader.DownloadFile(ctx, fileID, saveDir, meta); apiErr == nil {
			return nil
		} else {
			slog.Warn("Google Drive API download failed, falling back to direct download", "error", apiErr)
		}
	} else {
		slog.Debug("Could not initialize Google Drive API, using direct download", "error", err)
	}

	// Convert to download URL
	// Try multiple download URL formats:
	// 1. usercontent API (often bypasses confirmation)
	// 2. Standard uc endpoint
	downloadURLs := []string{
		fmt.Sprintf("https://drive.usercontent.google.com/download?id=%s&export=download&confirm=t", fileID),
		fmt.Sprintf("https://drive.google.com/uc?id=%s&export=download&confirm=t", fileID),
		fmt.Sprintf("https://drive.google.com/uc?id=%s&export=download", fileID),
	}

	var filename string
	var body io.ReadCloser
	var lastErr error
	var lastDownloadErr error
	var totalBytes int64
	var acceptsRange bool
	var resumePath string
	var resumeOffset int64

	if path, offset, ok, err := findStateByURL(saveDir, driveURL); err == nil && ok {
		resumePath = path
		if size, hasTemp, err := tempFileOffset(path); err == nil && hasTemp {
			resumeOffset = size
		} else {
			resumeOffset = offset
		}
	}

	// Try each download URL format
	for urlIdx, downloadURL := range downloadURLs {
		slog.Debug("Trying Google Drive download URL", "url", downloadURL, "format", urlIdx+1, "of", len(downloadURLs))

		for attempt := 1; attempt <= driveDownloadMaxAttempts; attempt++ {
			rangeStart := resumeOffset
			filename, body, totalBytes, acceptsRange, lastDownloadErr = g.downloadWithConfirmation(ctx, downloadURL, driveURL, rangeStart)
			if lastDownloadErr == nil && rangeStart > 0 && !acceptsRange {
				if resumePath != "" {
					_ = os.Remove(tempFilePath(resumePath))
				}
				resumeOffset = 0
				rangeStart = 0
				filename, body, totalBytes, acceptsRange, lastDownloadErr = g.downloadWithConfirmation(ctx, downloadURL, driveURL, rangeStart)
			}
			if lastDownloadErr == nil {
				lastErr = nil
			} else {
				lastErr = lastDownloadErr
			}

			if lastErr == nil {
				if filename == "" && resumePath == "" {
					// Fallback to using file ID as filename
					filename = fileID + ".bin"
				}

				// Save the file
				savePath := resumePath
				if savePath == "" {
					savePath = filepath.Join(saveDir, filename)
				}
				complete, err := isDownloadComplete(savePath)
				if err != nil {
					lastErr = fmt.Errorf("check file: %w", err)
				} else if complete {
					_, _ = io.Copy(io.Discard, body)
					_ = body.Close()
					slog.Info("Google Drive file already downloaded", "filename", filename, "url", driveURL)
					return nil
				} else {
					meta.URL = driveURL
					meta.AssetID = fileID
					if resumeOffset > 0 && acceptsRange {
						if err := saveReaderWithStateAt(ctx, savePath, body, meta, totalBytes, 0666, resumeOffset, true); err != nil {
							lastErr = fmt.Errorf("save file: %w", err)
						} else {
							slog.Info("Downloaded Google Drive file", "filename", filepath.Base(savePath), "url", driveURL)
							return nil
						}
					} else if err := saveReaderWithState(ctx, savePath, body, meta, totalBytes, 0666); err != nil {
						lastErr = fmt.Errorf("save file: %w", err)
					} else {
						slog.Info("Downloaded Google Drive file", "filename", filepath.Base(savePath), "url", driveURL)
						return nil
					}
				}
				if body != nil {
					_ = body.Close()
				}
			}

			if errors.Is(lastErr, ErrDriveAccessDenied) {
				return fmt.Errorf("download file: %w", lastErr)
			}
			if attempt < driveDownloadMaxAttempts {
				slog.Warn("Google Drive download failed, retrying...", "url", driveURL, "attempt", attempt, "error", lastErr)
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if driveDownloadRetryDelay > 0 {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(driveDownloadRetryDelay):
					}
				}
			}
		}

		// If we exhausted all attempts with this URL, try the next one
		if lastErr != nil && urlIdx < len(downloadURLs)-1 {
			slog.Info("Trying alternative Google Drive download URL", "url_format", urlIdx+2)
		}
	}
	return fmt.Errorf("download file: %w", lastErr)
}

func (g *DriveDownloader) downloadWithConfirmation(ctx context.Context, downloadURL string, driveURL string, rangeStart int64) (string, io.ReadCloser, int64, bool, error) {
	filename, body, cookies, confirmToken, confirmURL, needsConfirm, contentLength, acceptsRange, err := g.downloadOnce(ctx, downloadURL, nil, rangeStart)
	if err != nil {
		return "", nil, 0, false, err
	}
	confirmAttempts := 0
	for needsConfirm {
		confirmAttempts++
		if confirmAttempts > 3 {
			return "", nil, 0, false, fmt.Errorf("download confirmation required but not satisfied")
		}
		nextURL := confirmURL
		if nextURL == "" {
			if confirmToken == "" {
				return "", nil, 0, false, fmt.Errorf("download confirmation required but token not found")
			}
			var err error
			nextURL, err = g.addConfirmParam(downloadURL, confirmToken)
			if err != nil {
				return "", nil, 0, false, fmt.Errorf("build confirm URL: %w", err)
			}
		}
		slog.Warn("Google Drive requires confirmation, retrying...", "url", driveURL)
		var newCookies []*http.Cookie
		var accepts bool
		filename, body, newCookies, confirmToken, confirmURL, needsConfirm, contentLength, accepts, err = g.downloadOnce(ctx, nextURL, cookies, rangeStart)
		if err != nil {
			return "", nil, 0, false, err
		}
		acceptsRange = accepts
		cookies = mergeCookies(cookies, newCookies)
	}
	return filename, body, contentLength, acceptsRange, nil
}

// extractFileID extracts the file ID from a Google Drive URL
func (g *DriveDownloader) extractFileID(urlStr string) (string, error) {
	pattern := `(?i)drive\.google\.com/(?:file/d/([a-zA-Z0-9_-]+)|[^\s]*?[?&]id=([a-zA-Z0-9_-]+))`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(urlStr)
	if len(matches) > 2 {
		if matches[1] != "" {
			return matches[1], nil
		}
		if matches[2] != "" {
			return matches[2], nil
		}
	}
	return "", fmt.Errorf("invalid Google Drive URL format")
}

func (g *DriveDownloader) downloadOnce(ctx context.Context, urlStr string, cookies []*http.Cookie, rangeStart int64) (string, io.ReadCloser, []*http.Cookie, string, string, bool, int64, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return "", nil, nil, "", "", false, 0, false, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("User-Agent", g.userAgent)
	req.Header.Set("Referer", "https://drive.google.com/")
	if rangeStart > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", rangeStart))
	}

	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", nil, nil, "", "", false, 0, false, fmt.Errorf("http request: %w", err)
	}

	acceptsRange := resp.StatusCode == http.StatusPartialContent
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		// Read body to check for virus scan warning
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return "", nil, nil, "", "", false, 0, false, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	respCookies := resp.Cookies()

	// Get filename from Content-Disposition header
	contentDisposition := resp.Header.Get("Content-Disposition")
	filename := ""
	if contentDisposition != "" {
		filename = g.parseFilename(contentDisposition)
		return filename, resp.Body, respCookies, "", "", false, resp.ContentLength, acceptsRange, nil
	}

	contentType := resp.Header.Get("Content-Type")
	peek := make([]byte, 512)
	n, readErr := resp.Body.Read(peek)
	if readErr != nil && readErr != io.EOF {
		resp.Body.Close()
		return "", nil, nil, "", "", false, 0, false, fmt.Errorf("read response: %w", readErr)
	}
	peek = peek[:n]

	if g.looksLikeHTML(contentType, peek) {
		bodyBytes, err := io.ReadAll(io.MultiReader(bytes.NewReader(peek), resp.Body))
		resp.Body.Close()
		if err != nil {
			return "", nil, nil, "", "", false, 0, false, fmt.Errorf("read HTML response: %w", err)
		}
		htmlContent := string(bodyBytes)

		// Debug: Save HTML to temp file for inspection
		if tmpFile, err := os.CreateTemp("", "gdrive-response-*.html"); err == nil {
			_, _ = tmpFile.WriteString(htmlContent)
			tmpFile.Close()
			slog.Debug("Saved Google Drive HTML response", "file", tmpFile.Name())
		}

		if g.isAccessDenied(htmlContent) {
			return "", nil, respCookies, "", "", false, 0, false, ErrDriveAccessDenied
		}
		confirmToken := g.extractConfirmToken(htmlContent, respCookies)
		confirmURL := g.extractDownloadURL(htmlContent)

		slog.Debug("Extracted confirmation info", "token", confirmToken, "url", confirmURL)

		if confirmToken != "" || confirmURL != "" {
			return "", nil, respCookies, confirmToken, confirmURL, true, 0, acceptsRange, nil
		}
		return "", nil, respCookies, "", "", false, 0, false, fmt.Errorf("unexpected HTML response from Google Drive")
	}

	return filename, io.NopCloser(io.MultiReader(bytes.NewReader(peek), resp.Body)), respCookies, "", "", false, resp.ContentLength, acceptsRange, nil
}

func (g *DriveDownloader) looksLikeHTML(contentType string, peek []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		return true
	}
	trimmed := strings.TrimSpace(strings.ToLower(string(peek)))
	return strings.HasPrefix(trimmed, "<!doctype html") ||
		strings.HasPrefix(trimmed, "<html") ||
		strings.HasPrefix(trimmed, "<head") ||
		strings.HasPrefix(trimmed, "<body")
}

func (g *DriveDownloader) extractConfirmToken(htmlContent string, cookies []*http.Cookie) string {
	// Try URL query parameter pattern: confirm=xxx
	pattern := `(?i)confirm=([a-zA-Z0-9_-]+)`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(htmlContent)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try input field pattern: name="confirm" value="xxx"
	pattern = `(?i)name=["']confirm["']\s*+value=["']([a-zA-Z0-9_-]+)["']`
	re = regexp.MustCompile(pattern)
	matches = re.FindStringSubmatch(htmlContent)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try alternate input pattern: value="xxx" name="confirm"
	pattern = `(?i)value=["']([a-zA-Z0-9_-]+)["']\s*+name=["']confirm["']`
	re = regexp.MustCompile(pattern)
	matches = re.FindStringSubmatch(htmlContent)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try data-confirm attribute pattern
	pattern = `(?i)data-confirm=["']([a-zA-Z0-9_-]+)["']`
	re = regexp.MustCompile(pattern)
	matches = re.FindStringSubmatch(htmlContent)
	if len(matches) > 1 {
		return matches[1]
	}

	// Try id="confirm" value="xxx" pattern
	pattern = `(?i)id=["']confirm["']\s*+value=["']([a-zA-Z0-9_-]+)["']`
	re = regexp.MustCompile(pattern)
	matches = re.FindStringSubmatch(htmlContent)
	if len(matches) > 1 {
		return matches[1]
	}

	// Check cookies for download_warning
	for _, cookie := range cookies {
		if strings.HasPrefix(cookie.Name, "download_warning") {
			return cookie.Value
		}
	}

	// Debug: Log a sample of the HTML if no token found
	slog.Debug("Confirm token not found in HTML", "preview", htmlContent[:min(500, len(htmlContent))])
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mergeCookies(base []*http.Cookie, extra []*http.Cookie) []*http.Cookie {
	if len(base) == 0 {
		return extra
	}
	if len(extra) == 0 {
		return base
	}

	merged := make([]*http.Cookie, 0, len(base)+len(extra))
	seen := make(map[string]struct{})
	for _, cookie := range extra {
		key := cookie.Name
		seen[key] = struct{}{}
		merged = append(merged, cookie)
	}
	for _, cookie := range base {
		key := cookie.Name
		if _, ok := seen[key]; ok {
			continue
		}
		merged = append(merged, cookie)
	}
	return merged
}

func (g *DriveDownloader) extractDownloadURL(htmlContent string) string {
	unescaped := g.normalizeHTML(htmlContent)

	// First, try to find data-url attributes or JS variables
	// Pattern for data-url attribute
	dataURLPattern := `(?i)data-url=["']([^"']+)["']`
	re := regexp.MustCompile(dataURLPattern)
	if match := re.FindStringSubmatch(unescaped); len(match) > 1 {
		decoded := html.UnescapeString(match[1])
		if strings.HasPrefix(decoded, "http") {
			return decoded
		}
	}

	// gdown pattern: JSON "downloadUrl":"..."
	jsonURLPattern := `"downloadUrl":"([^"]+)`
	if match := regexp.MustCompile(jsonURLPattern).FindStringSubmatch(unescaped); len(match) > 1 {
		url := match[1]
		url = strings.ReplaceAll(url, `\u003d`, "=")
		url = strings.ReplaceAll(url, `\u0026`, "&")
		if strings.Contains(url, "drive.google.com") || strings.Contains(url, "drive.usercontent.google.com") {
			return url
		}
	}

	// gdown pattern: href="/uc?export=download..."
	hrefPattern := `(?i)href="(\/uc\?export=download[^"]+)`
	if match := regexp.MustCompile(hrefPattern).FindStringSubmatch(unescaped); len(match) > 1 {
		return "https://docs.google.com" + html.UnescapeString(match[1])
	}

	// Pattern for href in download link: href="/uc?..."
	hrefPattern2 := `(?i)href=["'](/uc\?[^"']+)["']`
	if match := regexp.MustCompile(hrefPattern2).FindStringSubmatch(unescaped); len(match) > 1 {
		return "https://drive.google.com" + html.UnescapeString(match[1])
	}

	// Pattern for download link to usercontent: href="https://drive.usercontent.google.com/..."
	usercontentPattern := `(?i)href=["'](https://drive\.usercontent\.google\.com/[^"']+)["']`
	if match := regexp.MustCompile(usercontentPattern).FindStringSubmatch(unescaped); len(match) > 1 {
		return html.UnescapeString(match[1])
	}

	// Standard URL patterns
	patterns := []string{
		`https://drive\.usercontent\.google\.com/download[^"'\s<>]+`,
		`https://drive\.google\.com/uc\?[^"'\s<>]+`,
		`http://drive\.google\.com/uc\?[^"'\s<>]+`,
		`https://drive\.google\.com/uc\?[^"'\s<>]+`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		match := re.FindString(unescaped)
		if match != "" {
			// Clean up the match - stop at quote or tag end
			for i, c := range match {
				if c == '"' || c == '\'' || c == '<' || c == '>' {
					match = match[:i]
					break
				}
			}
			return match
		}
	}

	// Try escaped JSON patterns
	jsonPattern := `url["']:["']([^"']+)["']`
	if match := regexp.MustCompile(jsonPattern).FindStringSubmatch(unescaped); len(match) > 1 {
		decoded := html.UnescapeString(match[1])
		if strings.Contains(decoded, "drive.google.com") || strings.Contains(decoded, "drive.usercontent.google.com") {
			return decoded
		}
	}

	// Debug: log a sample if no URL found
	slog.Debug("Download URL not found in HTML", "preview", unescaped[:min(500, len(unescaped))])
	return ""
}

func (g *DriveDownloader) normalizeHTML(htmlContent string) string {
	unescaped := html.UnescapeString(htmlContent)
	replacements := []struct {
		from string
		to   string
	}{
		{`\\u003d`, `=`},
		{`\\u0026`, `&`},
		{`\\u003f`, `?`},
		{`\\u002f`, `/`},
		{`\\u002d`, `-`},
		{`\\u003a`, `:`},
		{`\\u003b`, `;`},
		{`\\u0025`, `%`},
		{`\\/`, `/`},
	}
	for _, r := range replacements {
		unescaped = strings.ReplaceAll(unescaped, r.from, r.to)
	}
	return unescaped
}

func (g *DriveDownloader) isAccessDenied(htmlContent string) bool {
	lower := strings.ToLower(htmlContent)
	return strings.Contains(lower, "you need access") ||
		strings.Contains(lower, "request access") ||
		strings.Contains(lower, "sign in") ||
		strings.Contains(lower, "accounts.google.com") ||
		strings.Contains(lower, "access denied")
}

func (g *DriveDownloader) addConfirmParam(urlStr string, token string) (string, error) {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return "", err
	}
	query := parsedURL.Query()
	query.Set("confirm", token)
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}

// parseFilename extracts the filename from Content-Disposition header
func (g *DriveDownloader) parseFilename(contentDisposition string) string {
	// Parse filename*=UTF-8''filename or filename="filename"
	pattern := `filename\*?=(?:UTF-8''|")?([^";]+)(?:")?`
	re := regexp.MustCompile(pattern)
	if matches := re.FindStringSubmatch(contentDisposition); len(matches) > 1 {
		filename := matches[1]
		// Remove trailing semicolon if present
		filename = strings.TrimSuffix(filename, ";")
		if unescaped, err := url.QueryUnescape(filename); err == nil {
			return unescaped
		}
		return filename
	}
	return ""
}
