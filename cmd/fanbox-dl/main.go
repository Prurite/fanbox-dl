package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"

	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"github.com/hareku/fanbox-dl/internal/applog"
	"github.com/hareku/fanbox-dl/internal/tlsclient"
	"github.com/hareku/fanbox-dl/pkg/fanbox"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/urfave/cli/v2"
)

func resolveSessionID(c *cli.Context) string {
	if v := c.String(sessIDFlag.Name); v != "" {
		return v
	}

	if v := os.Getenv("FANBOXSESSID"); v != "" {
		return v
	}
	if v := os.Getenv("FANBOX_COOKIE"); v != "" {
		return v
	}

	return ""
}

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var versionFlag = &cli.BoolFlag{
	Name:  "version",
	Value: false,
	Usage: "Print the version and exit.",
}
var creatorFlag = &cli.StringFlag{
	Name:     "creator",
	Usage:    "Comma separated creator IDs to download. DO NOT prepend '@' to the creator ID.",
	Required: false,
}
var ignoreCreatorFlag = &cli.StringFlag{
	Name:     "ignore-creator",
	Usage:    "Comma separated creator IDs to ignore to download.",
	Required: false,
}
var sessIDFlag = &cli.StringFlag{
	Name:     "sessid",
	Usage:    "FANBOXSESSID which is stored in Cookies. If this is not set, fanbox-dl refers FANBOXSESSID environment value.",
	Required: false,
}
var cookieFlag = &cli.StringFlag{
	Name:     "cookie",
	Usage:    "Cookie for Fanbox API. This value overrides FANBOXSESSID.",
	Required: false,
}
var userAgentFlag = &cli.StringFlag{
	Name:  "user-agent",
	Usage: "User-Agent for Fanbox API.",
	Value: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
}
var saveDirFlag = &cli.StringFlag{
	Name:  "save-dir",
	Value: "./images",
	Usage: "Directory to save images.",
}
var dirByPostFlag = &cli.BoolFlag{
	Name:  "dir-by-post",
	Value: false,
	Usage: "Whether to separate save directories by post title.",
}
var dirByPlanFlag = &cli.BoolFlag{
	Name:  "dir-by-plan",
	Value: false,
	Usage: "Whether to separate save directories by plan.",
}
var allFlag = &cli.BoolFlag{
	Name:  "all",
	Value: false,
	Usage: "Whether to check all posts. If --all=false, finish to crawling posts when found an already downloaded image.",
}
var supportingFlag = &cli.BoolFlag{
	Name:  "supporting",
	Value: true,
	Usage: "Whether to download images of supporting creators.",
}
var followingFlag = &cli.BoolFlag{
	Name:  "following",
	Value: true,
	Usage: "Whether to download images of following creators.",
}
var skipFiles = &cli.BoolFlag{
	Name:  "skip-files",
	Value: false,
	Usage: "Whether to skip downloading files (not images).",
}
var skipImages = &cli.BoolFlag{
	Name:  "skip-images",
	Value: false,
	Usage: "Whether to skip downloading images.",
}
var skipTexts = &cli.BoolFlag{
	Name:  "skip-texts",
	Value: false,
	Usage: "Whether to skip downloading post contents as text files.",
}
var dryRunFlag = &cli.BoolFlag{
	Name:  "dry-run",
	Value: false,
	Usage: "Whether to dry-run. In dry-run, fanbox-dl skip downloading files.",
}
var verboseFlag = &cli.BoolFlag{
	Name:  "verbose",
	Value: false,
	Usage: "Whether to output debug logs.",
}
var skipOnErrorFlag = &cli.BoolFlag{
	Name:  "skip-on-error",
	Value: false,
	Usage: "Whether to skip downloading instead of exiting when an error occurred.",
}
var removeUnprintableCharsFlag = &cli.BoolFlag{
	Name:  "remove-unprintable-chars",
	Value: false,
	Usage: "Whether to remove unprintable characters from file names.",
}

var rateLimitFlag = &cli.Float64Flag{
	Name:  "rate-limit",
	Value: 0,
	Usage: "Rate limit in requests per second (0 = no limit).",
}

var saveJSONFlag = &cli.BoolFlag{
	Name:  "save-json",
	Value: false,
	Usage: "Whether to save original API JSON responses.",
}

var saveHTMLFlag = &cli.BoolFlag{
	Name:  "save-html",
	Value: false,
	Usage: "Whether to generate HTML pages for posts.",
}

var htmlLanguageFlag = &cli.StringFlag{
	Name:  "html-language",
	Value: "zh-CN",
	Usage: "Language for HTML generation (zh-CN, zh-TW, ja, en). Default: zh-CN",
}

var downloadGigafilesFlag = &cli.BoolFlag{
	Name:  "download-gigafiles",
	Value: false,
	Usage: "Whether to automatically download files from gigafile.nu links.",
}

var useStateManagerFlag = &cli.BoolFlag{
	Name:  "use-state-manager",
	Value: false,
	Usage: "Whether to use state manager to track downloaded posts (creates LastSavePostId.json).",
}

var startDateFlag = &cli.StringFlag{
	Name:  "start-date",
	Usage: "Only download posts published after this date (format: YYYY-MM-DD).",
	Value: "",
}

var endDateFlag = &cli.StringFlag{
	Name:  "end-date",
	Usage: "Only download posts published before this date (format: YYYY-MM-DD).",
	Value: "",
}

var app = &cli.App{
	Name:  "fanbox-dl",
	Usage: "This CLI downloads images of supporting and following creators.",
	Flags: []cli.Flag{
		versionFlag,
		creatorFlag,
		ignoreCreatorFlag,
		sessIDFlag,
		cookieFlag,
		saveDirFlag,
		dirByPostFlag,
		dirByPlanFlag,
		userAgentFlag,
		allFlag,
		supportingFlag,
		followingFlag,
		skipFiles,
		skipImages,
		skipTexts,
		dryRunFlag,
		verboseFlag,
		skipOnErrorFlag,
		removeUnprintableCharsFlag,
		rateLimitFlag,
		saveJSONFlag,
		saveHTMLFlag,
		htmlLanguageFlag,
		downloadGigafilesFlag,
		useStateManagerFlag,
		startDateFlag,
		endDateFlag,
	},
	Action: func(c *cli.Context) error {
		applog.InitLogger(c.Bool(verboseFlag.Name))
		slog.Info("Launching Pixiv FANBOX Downloader!", "version", version, "commit", commit, "date", date)
		if c.Bool(versionFlag.Name) {
			return nil
		}

		var cookieStr string
		if sessID := resolveSessionID(c); sessID != "" {
			slog.Debug("Using session ID", "sessid_bytes", len(sessID))
			cookieStr = fmt.Sprintf("FANBOXSESSID=%s", sessID)
		}
		if v := c.String(cookieFlag.Name); v != "" {
			if cookieStr != "" {
				slog.Warn("session ID and cookie are set, cookie option overrides session ID option")
			}
			slog.Debug("Using cookie", "cookie_bytes", len(v))
			cookieStr = v
		}

		// Parse date ranges if provided
		var startDate, endDate *time.Time
		if startDateStr := c.String(startDateFlag.Name); startDateStr != "" {
			parsedTime, err := time.Parse("2006-01-02", startDateStr)
			if err != nil {
				return fmt.Errorf("invalid start date format (use YYYY-MM-DD): %w", err)
			}
			startDate = &parsedTime
		}

		if endDateStr := c.String(endDateFlag.Name); endDateStr != "" {
			parsedTime, err := time.Parse("2006-01-02", endDateStr)
			if err != nil {
				return fmt.Errorf("invalid end date format (use YYYY-MM-DD): %w", err)
			}
			// Set end date to the end of the specified day
			parsedTime = parsedTime.Add(24*time.Hour - time.Second)
			endDate = &parsedTime
		}

		httpClient := retryablehttp.NewClient()
		httpClient.Logger = slog.Default()
		httpClient.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
			if err != nil {
				return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
			}
			b, err := fanbox.IsFailedToThumbnailingErr(resp)
			if err == nil && b {
				return false, fanbox.ErrFailedToThumbnailing
			}
			return retryablehttp.DefaultRetryPolicy(ctx, resp, nil)
		}

		tlsTransp, err := tlsclient.NewTransportWithOptions(tls_client.NewNoopLogger(), tls_client.WithClientProfile(profiles.Chrome_131))
		if err != nil {
			return fmt.Errorf("create tls transport: %w", err)
		}
		httpClient.HTTPClient.Transport = tlsTransp

		storage := &fanbox.LocalStorage{
			SaveDir:                c.String(saveDirFlag.Name),
			DirByPost:              c.Bool(dirByPostFlag.Name),
			DirByPlan:              c.Bool(dirByPlanFlag.Name),
			RemoveUnprintableChars: c.Bool(removeUnprintableCharsFlag.Name),
			EnableSaveJSON:         c.Bool(saveJSONFlag.Name),
			EnableSaveHTML:         c.Bool(saveHTMLFlag.Name),
		}

		api := &fanbox.OfficialAPIClient{
			HTTPClient: httpClient,
			Cookie:     cookieStr,
			UserAgent:  c.String(userAgentFlag.Name),
		}

		// Set rate limit if specified
		if rps := c.Float64(rateLimitFlag.Name); rps > 0 {
			api.SetRateLimit(rps)
			slog.Info("Rate limit enabled", "requests_per_second", rps)
		}

		// Initialize new features
		var htmlGenerator *fanbox.HTMLGenerator
		if c.Bool(saveHTMLFlag.Name) {
			htmlGenerator = &fanbox.HTMLGenerator{
				Enable:                 true,
				DirByPost:              c.Bool(dirByPostFlag.Name),
				DirByPlan:              c.Bool(dirByPlanFlag.Name),
				RemoveUnprintableChars: c.Bool(removeUnprintableCharsFlag.Name),
				SaveDir:                c.String(saveDirFlag.Name),
				Language:               fanbox.Language(c.String(htmlLanguageFlag.Name)),
			}
			slog.Info("HTML generation enabled", "language", c.String(htmlLanguageFlag.Name))
		}

		var gigafileDownloader *fanbox.GigafileDownloader
		if c.Bool(downloadGigafilesFlag.Name) {
			gigafileDownloader = fanbox.NewGigafileDownloader(httpClient.HTTPClient, c.String(userAgentFlag.Name))
			slog.Info("Gigafile auto-download enabled")
		}

		var stateManager *fanbox.StateManager
		if c.Bool(useStateManagerFlag.Name) {
			sm, err := fanbox.NewStateManager(".")
			if err != nil {
				return fmt.Errorf("create state manager: %w", err)
			}
			stateManager = sm
			slog.Info("State manager enabled")
		}

		client := &fanbox.Client{
			CheckAllPosts:      c.Bool(allFlag.Name),
			DryRun:             c.Bool(dryRunFlag.Name),
			SkipFiles:          c.Bool(skipFiles.Name),
			SkipImages:         c.Bool(skipImages.Name),
			SkipTexts:          c.Bool(skipTexts.Name),
			SkipOnError:        c.Bool(skipOnErrorFlag.Name),
			OfficialAPIClient:  api,
			StartDate:          startDate,
			EndDate:            endDate,
			Storage:            storage,
			HTMLGenerator:      htmlGenerator,
			GigafileDownloader: gigafileDownloader,
			StateManager:       stateManager,
		}

		ctx := c.Context
		startedAt := time.Now()

		idLister := &fanbox.CreatorIDLister{
			OfficialAPIClient: api,
		}

		in := &fanbox.CreatorIDListerDoInput{
			IncludeSupporting: c.Bool(supportingFlag.Name),
			IncludeFollowing:  c.Bool(followingFlag.Name),
		}
		if c.String(creatorFlag.Name) != "" {
			in.InputCreatorIDs = strings.Split(c.String(creatorFlag.Name), ",")
		}
		if c.String(ignoreCreatorFlag.Name) != "" {
			in.IgnoreCreatorIDs = strings.Split(c.String(ignoreCreatorFlag.Name), ",")
		}

		ids, err := idLister.Do(ctx, in)
		if err != nil {
			return fmt.Errorf("resolve creator IDs: %w", err)
		}
		for _, id := range ids {
			slog.InfoContext(ctx, "Start downloading", "creator_id", id)
			if err := client.Run(ctx, id); err != nil {
				return fmt.Errorf("failed downloading of %q: %w", id, err)
			}
		}

		slog.InfoContext(ctx, "Completed.", "duration", time.Since(startedAt).Round(time.Millisecond*100))
		return nil
	},
}

func main() {
	if err := run(); err != nil {
		slog.Error("fanbox-dl Error", "error", err)
		slog.Error("The error log seems a bug, please open an issue on GitHub", "url", "https://github.com/hareku/fanbox-dl/issues")

		if errors.Is(err, fanbox.ErrStatusForbidden) {
			slog.Error("This 403 error may occur when connecting from an IP address outside of Japan. Please try again from VPN or other IP addresses in Japan.")
		}
		os.Exit(1)
	}
	os.Exit(0)
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := app.RunContext(ctx, os.Args); err != nil {
		return err
	}
	return nil
}
