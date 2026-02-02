# fanbox-dl: Pixiv FANBOX Downloader

`fanbox-dl` will download media of supported and followed creators on FANBOX.

This project has been enhanced with features from [PixivFanboxDownloader](https://github.com/konnokai/PixivFanboxDownloader), including:
- HTML page generation
- Gigafile auto-download
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
2. Execute the downloaded `fanbox-dl` binary. You can see usage by running `fanbox-dl --help`.

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
| dir-by-plan | Separates content saved into directories based on the plan that the post belonged to. | `--dir-by-plan` | `false` |
| dir-by-post | Separates content saved into directories based on the title of the post. <br>Stored inside the plan directory when accompanied by `dir-by-plan` flag. | `--dir-by-post` | `false` |
| all | Will ensure that all content is downloaded from creators. <br>Will also redownload content that might already be present locally. | `--all` | `false` |
| skip-files | Will skip downloading non-image files from creators. | `--skip-files` | `false` |
| skip-images | Will skip downloading images from creators. This is useful when you only want to download files. | `--skip-images` | `false` |
| skip-texts | Will skip downloading post contents as text files. | `--skip-texts` | `false` |
| skip-on-error | Will skip downloading instead of exiting when an error occurs. | `--skip-on-error` | `false` |
| dry-run | Will skip downloading all content from creators. | `--dry-run` | `false` |
| verbose | Gives more detailed information about commands being executed by the application. <br>Useful for debugging errors. | `--verbose` | `false` |
| save-dir | Root directory to save content. <br>Put directory in double quotes `"` if it contains spaces. <br>Supports relative and absolute directories. | `--save-dir ./content` | `./images` |
| user-agent | User agent to use for requests. | `--user-agent "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3"` | `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3` |
| remove-unprintable-chars | Removes unprintable characters from file name. In some environments, unprintable characters are not allowed in file names. | `--remove-unprintable-chars` | `false` |
| rate-limit | Rate limit for API requests in requests per second. Set to 0 for no limit. | `--rate-limit 2.0` | `0` |
| save-json | Save original API JSON responses as `post.json` files in each post directory. | `--save-json` | `false` |
| start-date | Only download posts published on or after this date. Format: YYYY-MM-DD | `--start-date 2023-01-01` | `NULL` |
| end-date | Only download posts published on or before this date. Format: YYYY-MM-DD | `--end-date 2023-12-31` | `NULL` |
| save-html | Generate HTML pages for downloaded posts. | `--save-html` | `false` |
| download-gigafiles | Automatically download files from gigafile.nu links in post content. | `--download-gigafiles` | `false` |
| use-state-state-manager | Use state manager to track downloaded posts (creates LastSavePostId.json). | `--use-state-manager` | `false` |

### Example

If you want to re-download all images from the creator `https://www.fanbox.cc/@creatornamehere`, execute `fanbox-dl --sessid xxxxx --save-dir ./content --creator creatornamehere --all`.

And you can see media in the relevant directory. `./content/creatornamehere/xxxx.jpg`.

### Acquiring your FANBOXSESSID

fanbox-dl needs your account FANBOXSESSID to download supported content, which has your login state stored in a browser Cookie.

For example, if you are using Google Chrome, you can get it by following to steps in https://developers.google.com/web/tools/chrome-devtools/storage/cookies.

### Advanced Features

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

When enabled, each post will have a `post.json` file saved alongside the downloaded media files.

#### Combining Features

You can combine multiple features:

```bash
fanbox-dl \
  --cookie "YOUR_COOKIE" \
  --save-dir "./downloads" \
  --dir-by-post \
  --rate-limit 2.0 \
  --save-json \
  --verbose
```

### New Features (from PixivFanboxDownloader)

#### HTML Page Generation

Generate HTML pages for downloaded posts, similar to the original FANBOX website but saved locally:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --save-html
```

This creates a `post.html` file in each post directory with:
- Formatted post title, metadata, and content
- Embedded images and links to downloaded files
- Responsive design with CSS styling
- Link to the original FANBOX post

#### Gigafile Auto-Download

Automatically detect and download files from gigafile.nu links found in post content:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --save-html --download-gigafiles
```

This feature:
- Scans HTML content for gigafile.nu URLs
- Automatically downloads files to the same directory
- Handles cookies and filenames from gigafile server

#### State Manager

Track downloaded posts to avoid re-downloading:

```bash
fanbox-dl --cookie "YOUR_COOKIE" --use-state-manager
```

This creates and uses:
- `LastSavePostId.json`: Stores the last downloaded post ID for each creator
- Automatically stops when reaching already downloaded posts
- Updates the last post ID after each successful download

#### Combining All New Features

```bash
fanbox-dl \
  --cookie "YOUR_COOKIE" \
  --save-dir "./downloads" \
  --dir-by-post \
  --save-html \
  --download-gigafiles \
  --use-state-manager \
  --verbose
```

This combination provides a complete experience similar to PixivFanboxDownloader:
- Download posts with HTML generation
- Auto-download gigafile links
- Track progress with state manager
- Avoid duplicate downloads

## Contribution

Please open an issue or pull request.
