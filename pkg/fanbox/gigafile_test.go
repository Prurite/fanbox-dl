package fanbox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// closer wraps a Reader with a no-op Close method
type closer struct {
	io.Reader
}

func (c *closer) Close() error {
	return nil
}

func TestExtractGigafileURLs(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:    "single gigafile link",
			content: "Download from: https://12.gigafile.nu/1234-abcdef",
			expected: []string{
				"https://12.gigafile.nu/1234-abcdef",
			},
		},
		{
			name: "multiple gigafile links with duplicates",
			content: `File 1: https://1.gigafile.nu/5678-123456
File 2: https://1.gigafile.nu/5678-123456
File 3: https://99.gigafile.nu/9999-ffffffff`,
			expected: []string{
				"https://1.gigafile.nu/5678-123456",
				"https://99.gigafile.nu/9999-ffffffff",
			},
		},
		{
			name:     "no gigafile links",
			content:  "This is normal text without any gigafile links",
			expected: []string(nil),
		},
		{
			name: "mixed content with drive and gigafile links",
			content: `Drive: https://drive.google.com/file/d/abc123/view
Gigafile: https://5.gigafile.nu/8888-000000`,
			expected: []string{
				"https://5.gigafile.nu/8888-000000",
			},
		},
		{
			name: "gigafile with single digit subdomain",
			content: "https://9.gigafile.nu/1111-aaaaaa",
			expected: []string{
				"https://9.gigafile.nu/1111-aaaaaa",
			},
		},
		{
			name: "gigafile with two digit subdomain",
			content: "https://50.gigafile.nu/2222-bbbbbb",
			expected: []string{
				"https://50.gigafile.nu/2222-bbbbbb",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractGigafileURLs(tt.content)
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractGigafileURLs() returned %d results, expected %d", len(result), len(tt.expected))
			}
			for i, url := range result {
				if url != tt.expected[i] {
					t.Errorf("ExtractGigafileURLs()[%d] = %v, expected %v", i, url, tt.expected[i])
				}
			}
		})
	}
}

func TestGigafileURLPattern(t *testing.T) {
	// Test that the pattern matches valid gigafile URLs
	pattern := regexp.MustCompile(`https://\d{1,2}\.gigafile\.nu/\d{4}-[a-f0-9]+`)

	validURLs := []string{
		"https://1.gigafile.nu/1234-abcdef",
		"https://99.gigafile.nu/9999-ffffffff",
		"https://50.gigafile.nu/5678-000000",
	}

	for _, testURL := range validURLs {
		if !pattern.MatchString(testURL) {
			t.Errorf("Pattern should match valid URL: %s", testURL)
		}
	}

	invalidURLs := []string{
		"https://example.com/file/1234",
		"https://gigafile.nu/1234-abcdef",
		"https://123.gigafile.nu/1234-abcdef",  // Too many digits in subdomain
		"https://1.gigafile.nu/123-abcdef",      // Not enough digits in file ID
		"https://1.gigafile.nu/12345-abcdef",   // Too many digits in file ID
		"http://1.gigafile.nu/1234-abcdef",    // Wrong scheme
		"drive.google.com/file/d/123",
	}

	for _, testURL := range invalidURLs {
		if pattern.MatchString(testURL) {
			t.Errorf("Pattern should NOT match invalid URL: %s", testURL)
		}
	}
}

type gigafileTestTransport struct {
	cookie   string
	filename string
	content  []byte
}

func (t *gigafileTestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method == http.MethodHead {
		// Return Set-Cookie header and filename
		headers := make(http.Header)
		headers.Set("Set-Cookie", "gfsid="+t.cookie)
		if t.filename != "" {
			headers.Set("Content-Disposition", "attachment; filename=\""+t.filename+"\"")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     headers,
			Body:       http.NoBody,
		}, nil
	}

	if req.Method == http.MethodGet {
		// Verify cookie is set
		cookies := req.Header.Get("Cookie")
		if !strings.Contains(cookies, "gfsid="+t.cookie) {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     make(http.Header),
				Body:       http.NoBody,
			}, nil
		}

		// Verify referer
		referer := req.Header.Get("Referer")
		if referer != "https://www.fanbox.cc/" {
			return &http.Response{
				StatusCode: http.StatusForbidden,
				Header:     make(http.Header),
				Body:       http.NoBody,
			}, nil
		}

		headers := make(http.Header)
		headers.Set("Content-Type", "application/octet-stream")
		headers.Set("Content-Disposition", "attachment; filename=\""+t.filename+"\"")

		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        headers,
			Body:          &closer{bytes.NewReader(t.content)},
			ContentLength:  int64(len(t.content)),
		}, nil
	}

	return nil, &url.Error{
		Op:  "http",
		URL:  req.URL.String(),
		Err:  errors.New("unsupported method"),
	}
}

func TestGigafileDownloaderDownloadFile(t *testing.T) {
	transport := &gigafileTestTransport{
		cookie:   "test-gfsid-12345",
		filename: "test-file.bin",
		content:  []byte("test content from gigafile"),
	}

	client := &http.Client{
		Transport: transport,
	}

	downloader := NewGigafileDownloader(client, "test-agent")
	saveDir := t.TempDir()

	gigafileURL := "https://12.gigafile.nu/1234-abcdef1234567890123456789012345678"

	err := downloader.DownloadFile(context.Background(), gigafileURL, saveDir, DownloadStateMeta{})
	if err != nil {
		t.Fatalf("DownloadFile() error: %v", err)
	}

	// Verify file was saved
	data, err := os.ReadFile(filepath.Join(saveDir, "test-file.bin"))
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}

	if string(data) != string(transport.content) {
		t.Errorf("downloaded content = %q, expected %q", string(data), string(transport.content))
	}
}

func TestGigafileDownloaderDownloadFileAlreadyDownloaded(t *testing.T) {
	transport := &gigafileTestTransport{
		cookie:   "test-gfsid-12345",
		filename: "already-downloaded.bin",
		content:  []byte("content"),
	}

	client := &http.Client{
		Transport: transport,
	}

	downloader := NewGigafileDownloader(client, "test-agent")
	saveDir := t.TempDir()

	// Create the file beforehand
	savePath := filepath.Join(saveDir, "already-downloaded.bin")
	err := os.WriteFile(savePath, []byte("pre-existing content"), 0644)
	if err != nil {
		t.Fatalf("create pre-existing file: %v", err)
	}

	gigafileURL := "https://12.gigafile.nu/1234-abcdef1234567890123456789012345678"

	err = downloader.DownloadFile(context.Background(), gigafileURL, saveDir, DownloadStateMeta{})
	if err != nil {
		t.Fatalf("DownloadFile() error: %v", err)
	}

	// Verify original file was not overwritten
	data, err := os.ReadFile(savePath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if string(data) != "pre-existing content" {
		t.Errorf("file was overwritten: %q", string(data))
	}
}

func TestGigafileDownloaderInvalidURL(t *testing.T) {
	client := &http.Client{}
	downloader := NewGigafileDownloader(client, "test-agent")
	saveDir := t.TempDir()

	invalidURL := "https://example.com/file/123"

	err := downloader.DownloadFile(context.Background(), invalidURL, saveDir, DownloadStateMeta{})
	if err == nil {
		t.Fatal("DownloadFile() expected error for invalid URL, got nil")
	}

	// Verify no files were created
	entries, err := os.ReadDir(saveDir)
	if err != nil {
		t.Fatalf("read directory: %v", err)
	}

	if len(entries) > 0 {
		t.Errorf("expected no files, got %d", len(entries))
	}
}

func TestGigafileDownloaderExpiredFile(t *testing.T) {
	// Server that returns empty filename (file expired)
	transport := &gigafileTestTransport{
		cookie:   "test-gfsid",
		filename: "", // Empty filename indicates expired file
		content:  []byte{},
	}

	client := &http.Client{
		Transport: transport,
	}

	downloader := NewGigafileDownloader(client, "test-agent")
	saveDir := t.TempDir()

	gigafileURL := "https://12.gigafile.nu/1234-abcdef1234567890123456789012345678"

	err := downloader.DownloadFile(context.Background(), gigafileURL, saveDir, DownloadStateMeta{})
	if err == nil {
		t.Fatal("DownloadFile() expected error for expired file, got nil")
	}

	expectedErrMsg := "file expired"
	if !strings.Contains(err.Error(), expectedErrMsg) {
		t.Errorf("error = %q, expected to contain %q", err.Error(), expectedErrMsg)
	}
}
