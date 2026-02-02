package fanbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/hareku/fanbox-dl/internal/ctxval"
	"golang.org/x/net/http2"
)

// Client is struct for Client.
type Client struct {
	CheckAllPosts     bool
	DryRun            bool
	SkipFiles         bool
	SkipImages        bool
	SkipTexts         bool
	SkipOnError       bool
	OfficialAPIClient *OfficialAPIClient
	Storage           *LocalStorage
	StartDate         *time.Time
	EndDate           *time.Time

	// New features
	HTMLGenerator      *HTMLGenerator
	GigafileDownloader *GigafileDownloader
	StateManager       *StateManager
	CreatorName        string
}

func (c *Client) Run(ctx context.Context, creatorID string) error {
	ctx = ctxval.AddSlogAttrs(ctx, slog.String("creator_id", creatorID))

	var pagination Pagination
	if err := c.OfficialAPIClient.RequestAndUnwrapJSON(
		ctx, http.MethodGet,
		fmt.Sprintf("https://api.fanbox.cc/post.paginateCreator?%s", func() string {
			q := url.Values{}
			q.Set("creatorId", creatorID)
			return q.Encode()
		}()),
		&pagination,
	); err != nil {
		return fmt.Errorf("get pagination: %w", err)
	}
	slog.DebugContext(ctx, "Found pages", "pages", len(pagination.Pages))

	if c.StartDate != nil || c.EndDate != nil {
		slog.InfoContext(ctx, "Date filtering active",
			"start_date", c.formatDateOrNil(c.StartDate),
			"end_date", c.formatDateOrNil(c.EndDate))
	}

	// Check state manager for last post ID
	var shouldCheckLastPost bool
	var lastPostID int64
	if c.StateManager != nil {
		if id, exists := c.StateManager.GetLastPostID(creatorID); exists {
			lastPostID = id
			shouldCheckLastPost = true
			slog.InfoContext(ctx, "Using last post ID filter", "last_id", lastPostID)
		}
	}

	for i, page := range pagination.Pages {
		content := ListCreatorResponse{}
		err := c.OfficialAPIClient.RequestAndUnwrapJSON(ctx, http.MethodGet, page, &content)
		if err != nil {
			return fmt.Errorf("list posts of %q: %w", creatorID, err)
		}
		slog.DebugContext(ctx, "Found posts",
			"page", i+1,
			"posts", len(content.Body),
		)

		// Filter posts based on last post ID
		if shouldCheckLastPost {
			filteredPosts := make([]Post, 0)
			reachedLastPost := false
			for _, post := range content.Body {
				postID, err := strconv.ParseInt(post.ID, 10, 64)
				if err != nil {
					slog.DebugContext(ctx, "Failed to parse post ID", "id", post)
					continue
				}

				if postID <= lastPostID {
					reachedLastPost = true
					break
				}

				filteredPosts = append(filteredPosts, post)
			}
			content.Body = filteredPosts

			if reachedLastPost || len(filteredPosts) == 0 {
				slog.InfoContext(ctx, "Reached last saved post, stopping")
				break
			}
		}

		if err := c.handlePage(ctx, &content); err != nil {
			if errors.Is(err, errAlreadyDownloaded) {
				slog.DebugContext(ctx, "No more new assets")
				return nil
			}
			return fmt.Errorf("handle page: %w", err)
		}

		// Check if we can stop fetching more pages based on dates
		if c.EndDate != nil && len(content.Body) > 0 {
			// If the last post on this page is older than the start date,
			// we don't need to check further pages
			oldestPostTime, err := time.Parse(time.RFC3339, content.Body[len(content.Body)-1].PublishedDateTime)
			if err == nil && oldestPostTime.Before(*c.EndDate) {
				slog.InfoContext(ctx, "Reached posts older than end date, stopping pagination")
				break
			}
		}
	}

	// Update last post ID if state manager is enabled
	if c.StateManager != nil && len(pagination.Pages) > 0 {
		// Get the first post from the first page to update as last downloaded
		firstPage := pagination.Pages[0]
		firstPageContent := ListCreatorResponse{}
		if err := c.OfficialAPIClient.RequestAndUnwrapJSON(ctx, http.MethodGet, firstPage, &firstPageContent); err == nil {
			if len(firstPageContent.Body) > 0 {
				// Find the first non-pinned post
				for _, post := range firstPageContent.Body {
					if !post.IsPinned {
						postID, err := strconv.ParseInt(post.ID, 10, 64)
						if err == nil {
							if err := c.StateManager.UpdateLastPostID(creatorID, postID); err != nil {
								slog.Warn("Failed to update last post ID", "error", err)
							} else {
								slog.Info("Updated last post ID", "creator_id", creatorID, "post_id", postID)
							}
							break
						}
					}
				}
			}
		}
	}

	return nil
}

// formatDateOrNil returns a formatted date string or "nil" if the date is nil
func (c *Client) formatDateOrNil(date *time.Time) string {
	if date == nil {
		return "nil"
	}
	return date.Format("2006-01-02")
}

func (c *Client) handlePage(ctx context.Context, content *ListCreatorResponse) error {
	for _, item := range content.Body {
		if err := c.handlePost(ctx, item); err != nil {
			// pinned posts are maybe not latest, we should check next posts
			if errors.Is(err, errAlreadyDownloaded) && item.IsPinned {
				continue
			}
			return fmt.Errorf("handle post: %w", err)
		}
	}
	return nil
}

func (c *Client) handlePost(ctx context.Context, item Post) error {
	ctx = ctxval.AddSlogAttrs(ctx, slog.String("title", item.Title), slog.String("published_at", item.PublishedDateTime))

	// Skip if post is outside of date range
	if !c.isWithinDateRange(item.PublishedDateTime) {
		slog.DebugContext(ctx, "Skipping post outside date range")
		return nil
	}

	if item.IsRestricted {
		slog.DebugContext(ctx, "Skipping restricted post")
		return nil
	}

	postResp := PostInfoResponse{}
	if err := c.OfficialAPIClient.RequestAndUnwrapJSON(
		ctx, http.MethodGet,
		fmt.Sprintf("https://api.fanbox.cc/post.info?%s", func() string {
			q := url.Values{}
			q.Set("postId", item.ID)
			return q.Encode()
		}()),
		&postResp,
	); err != nil {
		return fmt.Errorf("get post: %w", err)
	}
	post := postResp.Body

	// Handle saving JSON if enabled
	if err := c.handlePostJSON(ctx, post); err != nil {
		return fmt.Errorf("handle post json: %w", err)
	}

	// Handle text content
	if err := c.handlePostText(ctx, post); err != nil {
		return fmt.Errorf("handle post text: %w", err)
	}

	// Handle HTML generation
	if err := c.handlePostHTML(ctx, post); err != nil {
		return fmt.Errorf("handle post html: %w", err)
	}

	// for backward-compatibility, split downloadable file's order into two types
	var (
		nextImgOrder  int
		nextFileOrder int
	)
	for i, d := range post.ListDownloadable() {
		var (
			order     int
			assetType string
		)

		switch d.(type) {
		case Image:
			assetType = "image"
			order = nextImgOrder
			nextImgOrder++
		case File:
			assetType = "file"
			order = nextFileOrder
			nextFileOrder++
		default:
			return fmt.Errorf("unsupported asset type: %+v", d)
		}

		if err := c.handleAsset(
			ctxval.AddSlogAttrs(ctx, slog.Int("i", i), slog.String("asset_type", assetType)),
			post, order, d,
		); err != nil {
			if errors.Is(err, errAlreadyDownloaded) && c.CheckAllPosts {
				continue
			}
			return fmt.Errorf("handle %s: %w", assetType, err)
		}
	}

	return nil
}

// handlePostJSON handles saving JSON response of a post.
func (c *Client) handlePostJSON(ctx context.Context, post Post) error {
	if !c.Storage.EnableSaveJSON {
		slog.DebugContext(ctx, "Skip saving JSON")
		return nil
	}

	// Check if JSON already exists
	jsonExists, err := c.Storage.JSONExists(post)
	if err != nil {
		if c.SkipOnError {
			slog.ErrorContext(ctx, "Skip checking JSON existence due to error", "error", err)
			return nil
		}
		return fmt.Errorf("check whether JSON already saved: %w", err)
	}

	if jsonExists {
		slog.DebugContext(ctx, "JSON already saved")
		return nil
	}

	if c.DryRun {
		slog.InfoContext(ctx, "Skip saving JSON due to dry-run mode")
		return nil
	}

	slog.InfoContext(ctx, "Saving JSON")
	// Get JSON from post - we need to marshal it
	jsonData, err := json.Marshal(post)
	if err != nil {
		return fmt.Errorf("marshal post to JSON: %w", err)
	}

	if err := c.Storage.SaveJSON(post, jsonData); err != nil {
		if c.SkipOnError {
			slog.ErrorContext(ctx, "Skip saving JSON due to error", "error", err)
			return nil
		}
		return fmt.Errorf("save JSON: %w", err)
	}

	return nil
}

// handlePostText handles saving text content of a post.
func (c *Client) handlePostText(ctx context.Context, post Post) error {
	if c.SkipTexts {
		slog.DebugContext(ctx, "Skip saving text content")
		return nil
	}

	textContent := post.GetTextContent()
	if textContent == "" {
		slog.DebugContext(ctx, "No text content to save")
		return nil
	}

	isDownloaded, err := c.Storage.TextExists(post)
	if err != nil {
		if c.SkipOnError {
			slog.ErrorContext(ctx, "Skip saving text due to error", "error", err)
			return nil
		}
		return fmt.Errorf("check whether text already saved: %w", err)
	}

	if isDownloaded {
		slog.DebugContext(ctx, "Text content already saved")
		return nil
	}

	if c.DryRun {
		slog.InfoContext(ctx, "Skip saving text content due to dry-run mode")
		return nil
	}

	slog.InfoContext(ctx, "Saving text content")
	if err := c.Storage.SaveText(post); err != nil {
		if c.SkipOnError {
			slog.ErrorContext(ctx, "Skip saving text due to error", "error", err)
			return nil
		}
		return fmt.Errorf("save text content: %w", err)
	}

	return nil
}

// handlePostHTML handles saving HTML content of a post.
func (c *Client) handlePostHTML(ctx context.Context, post Post) error {
	if c.HTMLGenerator == nil || !c.HTMLGenerator.Enable {
		slog.DebugContext(ctx, "Skip saving HTML")
		return nil
	}

	// Check if HTML already exists
	htmlExists, err := c.Storage.HTMLExists(post)
	if err != nil {
		if c.SkipOnError {
			slog.ErrorContext(ctx, "Skip checking HTML existence due to error", "error", err)
			return nil
		}
		return fmt.Errorf("check whether HTML already saved: %w", err)
	}

	if htmlExists {
		slog.DebugContext(ctx, "HTML already saved")
		return nil
	}

	if c.DryRun {
		slog.InfoContext(ctx, "Skip saving HTML due to dry-run mode")
		return nil
	}

	slog.InfoContext(ctx, "Saving HTML")

	// Generate HTML content
	htmlContent := c.HTMLGenerator.GenerateHTML(post, c.CreatorName)

	// Save HTML
	if err := c.Storage.SaveHTML(post, htmlContent); err != nil {
		if c.SkipOnError {
			slog.ErrorContext(ctx, "Skip saving HTML due to error", "error", err)
			return nil
		}
		return fmt.Errorf("save HTML: %w", err)
	}

	// Handle gigafile downloads if enabled
	if c.GigafileDownloader != nil {
		c.handleGigafiles(ctx, post, htmlContent)
	}

	return nil
}

// handleGigafiles extracts and downloads gigafile links from HTML content
func (c *Client) handleGigafiles(ctx context.Context, post Post, htmlContent string) {
	gigafileURLs := ExtractGigafileURLs(htmlContent)
	if len(gigafileURLs) == 0 {
		return
	}

	slog.InfoContext(ctx, "Found gigafile links", "count", len(gigafileURLs))

	for _, url := range gigafileURLs {
		slog.InfoContext(ctx, "Downloading gigafile", "url", url)
		// Extract save directory from storage
		saveDir := c.Storage.SaveDir + "/" + post.CreatorID
		if err := c.GigafileDownloader.DownloadFile(ctx, url, saveDir); err != nil {
			slog.ErrorContext(ctx, "Failed to download gigafile", "url", url, "error", err)
		}
	}
}

// isWithinDateRange checks if post's published date is within the specified date range
func (c *Client) isWithinDateRange(publishedDateTimeStr string) bool {
	// If no date filters are set, include all posts
	if c.StartDate == nil && c.EndDate == nil {
		return true
	}

	publishedTime, err := time.Parse(time.RFC3339, publishedDateTimeStr)
	if err != nil {
		// If we can't parse the date, include the post by default
		return true
	}

	// Check if post is after start date (if specified)
	if c.StartDate != nil && publishedTime.Before(*c.StartDate) {
		return false
	}

	// Check if post is before end date (if specified)
	if c.EndDate != nil && publishedTime.After(*c.EndDate) {
		return false
	}

	return true
}

var errAlreadyDownloaded = errors.New("already downloaded")

func (c *Client) handleAsset(ctx context.Context, post Post, order int, d Downloadable) error {
	if _, ok := d.(File); ok && c.SkipFiles {
		slog.DebugContext(ctx, "Skip downloading files")
		return nil
	}
	if _, ok := d.(Image); ok && c.SkipImages {
		slog.DebugContext(ctx, "Skip downloading images")
		return nil
	}

	if d.GetID() == "" {
		slog.DebugContext(ctx, "Asset ID is empty")
		return nil
	}

	isDownloaded, err := c.Storage.Exist(post, order, d)
	if err != nil {
		if c.SkipOnError {
			slog.ErrorContext(ctx, "Skip downloading due to error", "error", err)
			return nil
		}

		return fmt.Errorf("check whether downloaded: %w", err)
	}

	if isDownloaded {
		slog.DebugContext(ctx, "Already downloaded")
		return errAlreadyDownloaded
	}

	if c.DryRun {
		slog.InfoContext(ctx, "Skip downloading due to dry-run mode")
		return nil
	}

	slog.InfoContext(ctx, "Downloading")
	if err := c.downloadWithRetry(ctx, post, order, d); err != nil {
		if c.SkipOnError {
			slog.ErrorContext(ctx, "Skip downloading due to error", "error", err)
			return nil
		}
		return fmt.Errorf("download: %w", err)
	}

	return nil
}

func (c *Client) downloadWithRetry(ctx context.Context, post Post, order int, d Downloadable) error {
	shouldRetry := func(err error) bool {
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return true
		}

		var opErr *net.OpError
		if errors.As(err, &opErr) {
			return true
		}

		var goAwayErr *http2.GoAwayError
		return errors.As(err, &goAwayErr)
	}

	waitDur := time.Second
	for retry := 0; retry < 10; retry++ {
		if err := c.download(ctx, post, order, d); err != nil {
			if !shouldRetry(err) {
				return fmt.Errorf("download error: %w", err)
			}

			slog.ErrorContext(ctx, "Download error, retrying", "error", err, "wait", waitDur)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(waitDur):
			}
			continue
		}
		break
	}
	return nil
}

var ErrStatusForbidden = errors.New("status code 403")

func (c *Client) download(ctx context.Context, post Post, order int, d Downloadable) error {
	var resp *http.Response

	resp, err := c.OfficialAPIClient.Request(ctx, http.MethodGet, d.GetURL())
	if err != nil {
		if errors.Is(err, ErrFailedToThumbnailing) {
			slog.InfoContext(ctx, "The original file is not available (maybe it's a too large), so download a thumbnail instead", "original_file", d.GetURL())
			tu, ok := d.GetThumbnailURL()
			if !ok {
				return fmt.Errorf("thumbnail URL is not found")
			}
			slog.InfoContext(ctx, "Downloading a thumbnail", "thumbnail_url", tu)

			resp, err = c.OfficialAPIClient.Request(ctx, http.MethodGet, tu)
			if err != nil {
				return fmt.Errorf("request error (%s): %w", tu, err)
			}
		} else {
			return fmt.Errorf("request error (%s): %w", d.GetURL(), err)
		}
	}

	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != 200 {
		if resp.StatusCode == 403 {
			return ErrStatusForbidden
		}
		return fmt.Errorf("status code %d", resp.StatusCode)
	}

	if err := c.Storage.Save(post, order, d, resp.Body); err != nil {
		return fmt.Errorf("save a file: %w", err)
	}

	return nil
}
