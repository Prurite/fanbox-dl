# fanbox-dl: Pixiv FANBOX Downloader

`fanbox-dl` will download media of supported and followed creators on FANBOX.

This project has been enhanced with features from [PixivFanboxDownloader](https://github.com/konnokai/PixivFanboxDownloader), including:
- HTML page generation
- Gigafile auto-download
- Google Drive auto-download
- State management for tracking downloads

Caution: `fanbox-dl` is command-line-program, so it doesn't provide graphical user interface.

## Installation

The latest binary can be downloaded [here](https://github.com/hareku/fanbox-dl/releases/latest).

- Windows (64bit): `fanbox-dl_x.x.x_Windows_x86_64.exe`
- Windows (32bit): `fanbox-dl_x.x.x_Windows_i386.exe`
- Mac: `fanbox-dl_x.x.x_Darwin_x86_64`
- Mac (M1 CPU): `fanbox-dl_x.x.x_Darwin_arm64`

## Usage

1. Open a command line interpreter. For example, If you are Windows user, open `Command Prompt` or `PowerShell`. If you are Mac user, open `Terminal`.
2. Execute downloaded `fanbox-dl` binary. You can see usage by running `fanbox-dl --help`.

> [!NOTE]
>
> `--sessid` and `--cookie` can not be used together; when both are used, `--cookie` will be used.

| Command | Description | Usage | Default |
| --- | --- | --- | --- |
| sessid | Requires FANBOXSESSID which is stored in browser Cookies for login state. <br>When not provided, refers FANBOXSESSID environment value. <br>If unavailable, only free posts are downloaded when accompanied by a `creator` flag. | `--sessid xxxxx` | `NULL` |
| cookie | Cookie string to use for requests. <br>When not provided, refers to `sessid` flag. | `--cookie "name=value; name2=value2"` | `NULL` |
| creator | Comma separated Pixiv creator IDs to download contents. <br>Overrides `supporting` and `following` flags. <br>`https://www.fanbox.cc/@`**example**. <br>Only bold text needed from URL. | `--creator user1`, `--creator user1,user2` | `NULL` |
| ignore-creator | Comma separated Pixiv creator IDs to ignore to download contents. | `--ignore-creator user1,user2` | `NULL` |
| supporting | When disabled, will not download content from creators you're supporting. | `--supporting=false` | `true` |
| following | When disabled, will not download content from creators you only follow. | `--following=false` | `true` |
| dir-by-plan | Separates content saved into directories based on the plan that post belonged to. | `--dir-by-plan` | `false` |
| dir-by-post | Separates content saved into directories based on title of post. <br>Stored inside plan directory when accompanied by `dir-by-plan` flag. | `--dir-by-post` | `false` |
| all | Will ensure that all content is downloaded from creators. <br>Will also redownload content that might already be present locally. | `--all` | `false` |
| skip-files | Will skip downloading non-image files from creators. | `--skip-files` | `false` |
| skip-images | Will skip downloading images from creators. This is useful when you only want to download files. | `--skip-images` | `false` |
| skip-texts | Will skip downloading post contents as text files. | `--skip-texts` | `false` |
| skip-on-error | Will skip downloading instead of exiting when an error occurs. | `--skip-on-error` | `false` |
| dry-run | Will skip downloading all content from creators. | `--dry-run` | `false` |
| verbose | Gives more detailed information about commands being executed by application. <br>Useful for debugging errors. | `--verbose` | `false` |
| save-dir | Root directory to save content. <br>Put directory in double quotes `"` if it contains spaces. <br>Supports relative and absolute directories. | `--save-dir ./content` | `./images` |
| user-agent | User agent to use for requests. | `--user-agent "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3"` | `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3` |
| remove-unprintable-chars | Removes unprintable characters from file name. In some environments, unprintable characters are not allowed in file names. | `--remove-unprintable-chars` | `false` |
| rate-limit | Rate limit for API requests in requests per second. Set to 0 for no limit. | `--rate-limit 2.0` | `0` |
| save-json | Save original API JSON responses as `post.json` files in each post directory. | `--save-json` | `false` |
| start-date | Only download posts published on or after this date. Format: YYYY-MM-DD | `--start-date 2023-01-01` | `NULL` |
| end-date | Only download posts published on or before this date. Format: YYYY-MM-DD | `--end-date 2023-12-31` | `NULL` |
| save-html | Generate HTML pages for downloaded posts. | `--save-html` | `false` |
| html-language | Language for HTML generation (zh-CN, zh-TW, ja, en). | `--html-language ja` | `zh-CN` |
| download-gigafiles | Automatically download files from gigafile.nu links in post content. | `--download-gigafiles` | `false` |
| download-drive | Automatically download files from Google Drive links in post content. | `--download-drive` | `false` |
| download-threads | Number of concurrent download workers for assets and external links. | `--download-threads 4` | `1` |
| timeout | HTTP timeout for downloads and API requests (e.g., 30s, 5m). | `--timeout 60s` | `0` (no timeout) |
| use-state-manager | Use state manager to track downloaded posts (creates LastSavePostId.json). | `--use-state-manager` | `false` |
| check | Check downloaded content for completeness without downloading. Validates that all images, files, text, JSON, and HTML files exist. | `--check` | `false` |

### Example

If you want to re-download all images from creator `https://www.fanbox.cc/@creatornamehere`, execute `fanbox-dl --sessid xxxxx --save-dir ./content --creator creatornamehere --all`.

And you can see media in the relevant directory. `./content/creatornamehere/xxxx.jpg`.

### Acquiring your FANBOXSESSID

fanbox-dl needs your account FANBOXSESSID to download supported content, which has your login state stored in a browser Cookie.

For example, if you are using Google Chrome, you can get it by following to steps in https://developers.google.com/web/tools/chrome-devtools/storage/cookies.

### Advanced Features

#### HTML Page Generation

Generate HTML pages for downloaded posts, similar to the original FANBOX website but saved locally:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --save-html
```

This creates an HTML file with:
- Formatted post title, metadata, and content
- Embedded images and hyperlinks to downloaded files
- Responsive design with CSS styling
- Compatible with `--dir-by-post` mode

#### Gigafile Auto-Download

Automatically detect and download files from gigafile.nu links found in post content:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --save-html --download-gigafiles
```

This feature:
- Scans HTML content for gigafile.nu URLs
- Automatically downloads files to the same directory
- Handles cookies and filenames from the gigafile server

#### Google Drive Auto-Download

Automatically detect and download files from Google Drive links found in post content:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --save-html --download-drive
```

This feature:
- Scans HTML content for Google Drive URLs (e.g., `https://drive.google.com/file/d/FILE_ID/view?usp=sharing`)
- Automatically downloads files to the same directory
- Handles large files and virus scan warning pages
- Extracts filenames from Content-Disposition headers
- Supports resume from interrupted downloads via `.part` files
- Compatible with [gdown](https://github.com/wkentaro/gdown) download strategies

**Optional: Using Google Drive API for faster downloads**

For better performance and reliability, you can configure a Google Drive API key:

```bash
# Get a free API key from https://console.cloud.google.com/apis/credentials
# 1. Create a new project or select existing one
# 2. Enable Google Drive API
# 3. Create credentials > API key
# 4. Set the API key as environment variable

export GOOGLE_DRIVE_API_KEY="your-api-key-here"

# Run fanbox-dl
fanbox-dl --cookie "YOUR_COOKIE" --download-drive
```

When API key is configured, the downloader will:
- First attempt to download via Google Drive API v3 (faster, no confirmation page)
- Fall back to direct download if API fails
- Better handle large files and resume support

**Note**: Without API key, the downloader still works using direct download with confirmation page handling.

You can combine both Gigafile and Google Drive auto-download:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --save-html --download-gigafiles --download-drive
```

#### State Manager

Track downloaded posts to avoid re-downloading:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --use-state-manager
```

This creates and uses:
- `LastSavePostId.json`: Stores the last downloaded post ID for each creator
- `.state` files: Track individual download progress for resume capability
- Automatically stops when reaching already downloaded posts
- Updates the last post ID after each successful download

#### Checking Download Completeness

Verify that all content has been downloaded successfully without re-downloading:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --check
```

The check mode will:
- Scan all posts from your creators
- Verify that all expected files exist:
  - Images and files from posts
  - Text content (if `--skip-texts` was not used)
  - JSON files (if `--save-json` was enabled)
  - HTML files (if `--save-html` was enabled)
- Report any missing files with detailed information
- Display a summary of complete vs incomplete posts

**Example output:**
```
INFO Checking creator creator_id=someuser
INFO Incomplete post post_id=12345 title="Post Title"
INFO   Missing image file=0-image.jpg
INFO   Missing: JSON
WARN Check summary total=50 complete=48 incomplete=2
```

**Check specific creators:**
```bash
fanbox-dl --cookie "YOUR_COOKIE" --check --creator user1,user2
```

**Check with same options used during download:**
```bash
# If you downloaded with these options:
fanbox-dl --cookie "YOUR_COOKIE" --save-json --save-html --dir-by-post

# Check with the same options:
fanbox-dl --cookie "YOUR_COOKIE" --check --save-json --save-html --dir-by-post
```

**Note**: The `--check` flag respects all path-related options (`--save-dir`, `--dir-by-post`, `--dir-by-plan`) and content options (`--skip-files`, `--skip-images`, `--skip-texts`, `--save-json`, `--save-html`) to ensure it checks the correct locations and expected files.

**State File Management**

The state manager creates two types of files:

1. **LastSavePostId.json** - Tracks the last successfully downloaded post for each creator:
```json
{
  "creator1": 12345678,
  "creator2": 87654321
}
```

2. **Download State Files** (`.state`) - Track individual file downloads for resume:
```json
{
  "version": 1,
  "status": "downloading",
  "creatorId": "creator1",
  "postId": "12345678",
  "postTitle": "Post Title",
  "assetId": "asset123",
  "assetType": "image",
  "url": "https://...",
  "filePath": "/path/to/file.jpg",
  "bytes": 1048576,
  "totalBytes": 2097152,
  "updatedAt": "2024-01-01T12:00:00Z"
}
```

**Resume Interrupted Downloads**

If a download is interrupted, the state manager will automatically resume from where it left off:
- Partial files are saved with `.part` extension
- State files track download progress
- On restart, downloads resume using HTTP Range requests

**Cleaning Up State Files**

State files are automatically cleaned up when downloads complete successfully. If you need to manually clean up orphaned state files:

```bash
# Find and remove completed state files
find ./images -name "*.state" -type f -delete

# Remove partial download files
find ./images -name "*.part" -type f -delete
```

#### Rate Limiting

Control the rate of API requests to avoid hitting rate limits or being blocked. Use the `--rate-limit` flag:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --rate-limit 1.5
```

This limits API requests to 1.5 requests per second. Set to `0` for no limit.

#### Saving Original API JSON

Save the original JSON responses from the FANBOX API for each post. This can be useful for debugging or archiving purposes:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --save-json
```

When enabled, each post will have a `post.json` file saved alongside downloaded media files.

#### Combining All Features

You can combine multiple features:

```bash
fanbox-dl \
  --sessid "YOUR_SESSID" \
  --save-dir "./downloads" \
  --dir-by-post \
  --save-html \
  --download-gigafiles \
  --download-drive \
  --use-state-manager \
  --rate-limit 2.0 \
  --save-json \
  --verbose \
  #--check
```

This provides a complete experience:
- Download posts with HTML generation and working hyperlinks
- Auto-download gigafile links
- Auto-download Google Drive links
- Track progress with state manager
- Avoid duplicate downloads

#### Checking Downloads After Completion

After downloading, verify that all content is complete:

```bash
# Check all downloaded content
fanbox-dl --cookie "YOUR_COOKIE" --check

# Check specific directory with same options
fanbox-dl --cookie "YOUR_COOKIE" --check --save-dir "./downloads" --dir-by-post --save-html --save-json
```

This is useful for:
- Verifying after interrupted downloads
- Checking if any files are missing
- Auditing download completeness

v ./pkg/fanbox -run TestDriveDownloader
```

### Building from Source

```bash
# Build for current platform
go build -o fanbox-dl ./cmd/fanbox-dl

# Build for specific platform
GOOS=windows GOARCH=amd64 go build -o fanbox-dl.exe ./cmd/fanbox-dl
GOOS=darwin GOARCH=arm64 go build -o fanbox-dl-mac ./cmd/fanbox-dl
GOOS=linux GOARCH=amd64 go build -o fanbox-dl-linux ./cmd/fanbox-dl
```

