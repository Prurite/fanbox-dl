package fanbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractDriveURLs(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected []string
	}{
		{
			name:    "single drive link",
			content: "Download file here: https://drive.google.com/file/d/TESTFILEID123456789/view?usp=sharing",
			expected: []string{
				"https://drive.google.com/file/d/TESTFILEID123456789/view?usp=sharing",
			},
		},
		{
			name: "multiple drive links with duplicates",
			content: `File 1: https://drive.google.com/file/d/ABC123xyz/view?usp=sharing
File 2: https://drive.google.com/file/d/ABC123xyz/view?usp=sharing
File 3: https://drive.google.com/file/d/DEF456abc/view?usp=sharing`,
			expected: []string{
				"https://drive.google.com/file/d/ABC123xyz/view?usp=sharing",
				"https://drive.google.com/file/d/DEF456abc/view?usp=sharing",
			},
		},
		{
			name:     "no drive links",
			content:  "This is a normal text without any links",
			expected: []string(nil),
		},
		{
			name:    "open id link",
			content: "Open link: https://drive.google.com/open?id=XYZ_123",
			expected: []string{
				"https://drive.google.com/open?id=XYZ_123",
			},
		},
		{
			name:    "uc download link",
			content: "Direct link: https://drive.google.com/uc?id=XYZ_123&export=download",
			expected: []string{
				"https://drive.google.com/uc?id=XYZ_123&export=download",
			},
		},
		{
			name:    "no scheme link",
			content: "Plain link: drive.google.com/file/d/NOHTTPS/view",
			expected: []string{
				"drive.google.com/file/d/NOHTTPS/view",
			},
		},
		{
			name:    "mixed content with gigafile and drive links",
			content: "Gigafile: https://12.gigafile.nu/1234-abcdef123456789012345678901234 Drive: https://drive.google.com/file/d/GHI789def/view?usp=sharing",
			expected: []string{
				"https://drive.google.com/file/d/GHI789def/view?usp=sharing",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractDriveURLs(tt.content)
			if len(result) != len(tt.expected) {
				t.Errorf("ExtractDriveURLs() returned %d results, expected %d", len(result), len(tt.expected))
			}
			for i, url := range result {
				if url != tt.expected[i] {
					t.Errorf("ExtractDriveURLs()[%d] = %v, expected %v", i, url, tt.expected[i])
				}
			}
		})
	}
}

func TestExtractFileID(t *testing.T) {
	downloader := NewDriveDownloader(nil, "")

	tests := []struct {
		name        string
		url         string
		expectedID  string
		expectError bool
	}{
		{
			name:       "valid drive url",
			url:        "https://drive.google.com/file/d/TESTFILEID123456789/view?usp=sharing",
			expectedID: "TESTFILEID123456789",
		},
		{
			name:       "valid drive url without query",
			url:        "https://drive.google.com/file/d/ABC123xyz",
			expectedID: "ABC123xyz",
		},
		{
			name:       "open id url",
			url:        "https://drive.google.com/open?id=OPEN123xyz",
			expectedID: "OPEN123xyz",
		},
		{
			name:       "uc download url",
			url:        "https://drive.google.com/uc?id=UC123xyz&export=download",
			expectedID: "UC123xyz",
		},
		{
			name:       "no scheme drive url",
			url:        "drive.google.com/file/d/NOSCHEME123xyz/view",
			expectedID: "NOSCHEME123xyz",
		},
		{
			name:        "invalid url",
			url:         "https://example.com/file/d/ABC123",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := downloader.extractFileID(tt.url)
			if tt.expectError {
				if err == nil {
					t.Errorf("extractFileID() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("extractFileID()() unexpected error: %v", err)
			}
			if id != tt.expectedID {
				t.Errorf("extractFileID()() = %v, expected %v", id, tt.expectedID)
			}
		})
	}
}

type rewriteTransport struct {
	base *url.URL
	rt   http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL = &url.URL{}
	*cloned.URL = *req.URL
	cloned.URL.Scheme = t.base.Scheme
	cloned.URL.Host = t.base.Host
	return t.rt.RoundTrip(cloned)
}

func TestDriveDownloaderDownloadFileWithConfirmation(t *testing.T) {
	origRetryDelay := driveDownloadRetryDelay
	driveDownloadRetryDelay = 0
	t.Cleanup(func() {
		driveDownloadRetryDelay = origRetryDelay
	})

	const (
		fileID     = "FILEID123"
		token      = "confirmToken123"
		cookieName = "download_warning_abc"
	)

	var seenConfirm bool
	var seenCookie bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/uc" {
			http.NotFound(w, r)
			return
		}

		confirm := r.URL.Query().Get("confirm")
		if confirm == "" {
			http.SetCookie(w, &http.Cookie{Name: cookieName, Value: token})
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><body>Virus scan warning <a href=\"/uc?export=download&id=" + fileID + "&confirm=" + token + "\">download</a></body></html>"))
			return
		}

		if confirm != token {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}
		seenConfirm = true
		if strings.Contains(r.Header.Get("Cookie"), cookieName+"="+token) {
			seenCookie = true
		}

		w.Header().Set("Content-Disposition", "attachment; filename=\"test.txt\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	client := &http.Client{
		Transport: &rewriteTransport{
			base: baseURL,
			rt:   http.DefaultTransport,
		},
	}

	downloader := NewDriveDownloader(client, "test-agent")
	saveDir := t.TempDir()

	driveURL := "https://drive.google.com/file/d/" + fileID + "/view?usp=sharing"
	if err := downloader.DownloadFile(context.Background(), driveURL, saveDir, DownloadStateMeta{}); err != nil {
		t.Fatalf("DownloadFile() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(saveDir, "test.txt"))
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected file content: %s", string(data))
	}
	if !seenConfirm {
		t.Fatalf("confirm token was not used")
	}
	if !seenCookie {
		t.Fatalf("confirmation cookie was not forwarded")
	}
}

func TestDriveDownloaderDownloadFileRetries(t *testing.T) {
	origRetryDelay := driveDownloadRetryDelay
	driveDownloadRetryDelay = 0
	t.Cleanup(func() {
		driveDownloadRetryDelay = origRetryDelay
	})

	const fileID = "RETRY2333"

	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if r.URL.Path != "/uc" && r.URL.Path != "/download" {
			http.NotFound(w, r)
			return
		}
		// Fail first URL format (usercontent) completely, succeed on second format
		if strings.Contains(r.Host, "usercontent") || r.URL.Query().Get("confirm") == "t" {
			http.NotFound(w, r)
			return
		}
		if attempts < driveDownloadMaxAttempts+1 {
			http.Error(w, "temporary error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Disposition", "attachment; filename=\"retry.txt\"")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	client := &http.Client{
		Transport: &rewriteTransport{
			base: baseURL,
			rt:   http.DefaultTransport,
		},
	}

	downloader := NewDriveDownloader(client, "test-agent")
	saveDir := t.TempDir()

	driveURL := "https://drive.google.com/file/d/" + fileID + "/view?usp=sharing"
	if err := downloader.DownloadFile(context.Background(), driveURL, saveDir, DownloadStateMeta{}); err != nil {
		t.Fatalf("DownloadFile() error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(saveDir, "retry.txt"))
	if err != nil {
		t.Fatalf("read downloaded file: %v", err)
	}
	if string(data) != "ok" {
		t.Fatalf("unexpected file content: %s", string(data))
	}

	// With multiple URL formats, we expect more attempts
	// First URL format: 10 attempts (all fail with 404)
	// Second URL format: 1 successful attempt
	if attempts < driveDownloadMaxAttempts {
		t.Fatalf("expected at least %d attempts, got %d", driveDownloadMaxAttempts, attempts)
	}
}
