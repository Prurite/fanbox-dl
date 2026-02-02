package fanbox

import (
	"fmt"
	"html"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/hareku/go-filename"
	"github.com/hareku/go-strlimit"
)

// Language represents the language for HTML generation
type Language string

const (
	LanguageSimplifiedChinese  Language = "zh-CN"
	LanguageTraditionalChinese Language = "zh-TW"
	LanguageJapanese           Language = "ja"
	LanguageEnglish            Language = "en"
)

// HTMLGenerator generates HTML pages from FANBOX posts
type HTMLGenerator struct {
	Enable                   bool
	DirByPost                bool
	DirByPlan                bool
	RemoveUnprintableChars   bool
	SaveDir                  string
	Language                 Language
}

// i18nTexts holds translations for different languages
type i18nTexts struct {
	PublishedTime string
	Author        string
	CreatorID     string
	Fee           string
	PostID        string
	Source        string
	Images        string
	Files         string
}

// getI18nTexts returns translated texts based on the language setting
func (hg *HTMLGenerator) getI18nTexts() i18nTexts {
	switch hg.Language {
	case LanguageTraditionalChinese:
		return i18nTexts{
			PublishedTime: "發布時間",
			Author:        "作者",
			CreatorID:     "創作者ID",
			Fee:           "費用",
			PostID:        "貼文ID",
			Source:        "來源",
			Images:        "圖片",
			Files:         "文件",
		}
	case LanguageJapanese:
		return i18nTexts{
			PublishedTime: "公開日時",
			Author:        "作者",
			CreatorID:     "クリエイターID",
			Fee:           "料金",
			PostID:        "投稿ID",
			Source:        "ソース",
			Images:        "画像",
			Files:         "ファイル",
		}
	case LanguageEnglish:
		return i18nTexts{
			PublishedTime: "Published",
			Author:        "Author",
			CreatorID:     "Creator ID",
			Fee:           "Fee",
			PostID:        "Post ID",
			Source:        "Source",
			Images:        "Images",
			Files:         "Files",
		}
	default: // LanguageSimplifiedChinese
		return i18nTexts{
			PublishedTime: "发布时间",
			Author:        "作者",
			CreatorID:     "创作者ID",
			Fee:           "费用",
			PostID:        "贴文ID",
			Source:        "来源",
			Images:        "图片",
			Files:         "文件",
		}
	}
}

// limitOsSafely limits the string length for OS safely (copied from LocalStorage)
func (hg *HTMLGenerator) limitOsSafely(name string) string {
	switch runtime.GOOS {
	case "windows":
		return strlimit.LimitRunesWithEnd(name, 210, "...")
	default:
		return strlimit.LimitBytesWithEnd(name, 250, "...")
	}
}

// escapeString escapes filename to make it file system safe (copied from LocalStorage)
func (hg *HTMLGenerator) escapeString(name string) string {
	return filename.EscapeString(name, "-")
}

// cleanDisplayName removes unprintable characters from display name
func (hg *HTMLGenerator) cleanDisplayName(name string) string {
	if hg.RemoveUnprintableChars {
		name = strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}
			return -1
		}, name)
	}
	return name
}

// GenerateHTML generates an HTML page from a post and returns the HTML content
func (hg *HTMLGenerator) GenerateHTML(post Post, creatorName string) string {
	if !hg.Enable {
		return ""
	}

	// Get language-specific texts
	texts := hg.getI18nTexts()

	// Set default language if not specified
	if hg.Language == "" {
		hg.Language = LanguageSimplifiedChinese
	}

	var sb strings.Builder

	// HTML header with language attribute
	sb.WriteString(fmt.Sprintf(`<!DOCTYPE html>
<html lang="%s">`, hg.Language))
	sb.WriteString("\n<head>\n")
	sb.WriteString(`<meta charset="UTF-8">`)
	sb.WriteString("\n<meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">")
	sb.WriteString(fmt.Sprintf("\n<title>%s - %s</title>", html.EscapeString(creatorName), html.EscapeString(post.Title)))
	sb.WriteString("\n<style>\n")
	sb.WriteString(`
body {
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
    line-height: 1.6;
    color: #333;
    max-width: 900px;
    margin: 0 auto;
    padding: 20px;
    background-color: #f9f9f9;
}
.post-container {
    background-color: white;
    padding: 30px;
    border-radius: 8px;
    box-shadow: 0 2px 4px rgba(0,0,0,0.1);
}
h1 {
    color: #2c3e50;
    border-bottom: 2px solid #3498db;
    padding-bottom: 10px;
}
h2 {
    color: #34495e;
    margin-top: 30px;
}
.post-meta {
    color: #7f8c8d;
    font-size: 14px;
    margin: 10px 0 20px;
}
.content p {
    margin: 15px 0;
}
.content img {
    max-width: 100%;
    height: auto;
    border-radius: 4px;
    margin: 10px 0;
}
.content a {
    color: #3498db;
    text-decoration: none;
}
.content a:hover {
    text-decoration: underline;
}
.code-block {
    background-color: #f4f4f4;
    padding: 15px;
    border-radius: 4px;
    overflow-x: auto;
}
.info-box {
    background-color: #e8f4fd;
    border-left: 4px solid #3498db;
    padding: 15px;
    margin: 20px 0;
}
`)
	sb.WriteString("\n</style>\n</head>\n<body>\n")

	// Main container
	sb.WriteString("<div class=\"post-container\">\n")

	// Title
	sb.WriteString(fmt.Sprintf("<h1>%s</h1>\n", html.EscapeString(post.Title)))

	// Meta information
	publishedTime, _ := time.Parse(time.RFC3339, post.PublishedDateTime)
	fmt.Fprintln(&sb, "<div class=\"post-meta\">")
	sb.WriteString(fmt.Sprintf("%s: %s<br>\n", texts.PublishedTime, publishedTime.Format("2006/01/02 15:04")))
	sb.WriteString(fmt.Sprintf("%s: %s<br>\n", texts.Author, html.EscapeString(creatorName)))
	sb.WriteString(fmt.Sprintf("%s: %s<br>\n", texts.CreatorID, post.CreatorID))
	if post.FeeRequired > 0 {
		sb.WriteString(fmt.Sprintf("%s: %d円<br>\n", texts.Fee, post.FeeRequired))
	}
	sb.WriteString(fmt.Sprintf("%s: %s\n", texts.PostID, post.ID))
	sb.WriteString("</div>\n")

	// Content
	sb.WriteString("<div class=\"content\">\n")

	if post.Body != nil {
		hg.generateBodyContent(&sb, post, texts)
	}

	// Add plain text if available
	if post.Body != nil && post.Body.Text != "" {
		sb.WriteString("<div class=\"text-block\">\n")
		sb.WriteString(fmt.Sprintf("<p>%s</p>\n", html.EscapeString(post.Body.Text)))
		sb.WriteString("</div>\n")
	}

	sb.WriteString("</div>\n") // End content
	sb.WriteString("</div>\n") // End container

	// Footer
	sb.WriteString(`<div class="footer">`)
	sb.WriteString(fmt.Sprintf("<p>%s: <a href=\"https://www.fanbox.cc/@%s/posts/%s\" target=\"_blank\">https://www.fanbox.cc/@%s/posts/%s</a></p>\n",
		texts.Source, post.CreatorID, post.ID, post.CreatorID, post.ID))
	sb.WriteString("</div>\n")

	sb.WriteString("</body>\n</html>")

	return sb.String()
}

// generateBodyContent generates HTML content from post body
func (hg *HTMLGenerator) generateBodyContent(sb *strings.Builder, post Post, texts i18nTexts) {
	if post.Body.Blocks != nil {
		hg.generateBlocksHTML(sb, post, texts)
	}

	// Handle image-type posts
	if post.Body.Images != nil && len(*post.Body.Images) > 0 {
		sb.WriteString("<div class=\"images-section\">\n")
		sb.WriteString(fmt.Sprintf("<h2>%s</h2>\n", texts.Images))
		for i, img := range *post.Body.Images {
			imgOrder := i
			imgName := hg.getImageFileName(post, imgOrder, img)
			imgPath := hg.getRelativePath(imgName)
			fmt.Fprintf(sb, "<p><img src=\"%s\" alt=\"%s\"></p>\n", html.EscapeString(imgPath), html.EscapeString(filepath.Base(imgName)))
		}
		sb.WriteString("</div>\n")
	}

	// Handle file-type posts
	if post.Body.Files != nil && len(*post.Body.Files) > 0 {
		sb.WriteString("<div class=\"files-section\">\n")
		sb.WriteString(fmt.Sprintf("<h2>%s</h2>\n", texts.Files))
		for i, file := range *post.Body.Files {
			fileOrder := i
			fileName := hg.getFileFileName(post, fileOrder, file)
			// Get the display name for the link text
			displayName := fmt.Sprintf("%s.%s", hg.cleanDisplayName(file.Name), file.Extension)
			filePath := hg.getRelativePath(fileName)
			fmt.Fprintf(sb, "<p><a href=\"%s\">%s</a></p>\n", html.EscapeString(filePath), html.EscapeString(displayName))

			// If it's an image file, also embed it
			if isImageExtension(file.Extension) {
				fmt.Fprintf(sb, "<p><img src=\"%s\" alt=\"%s\"></p>\n", html.EscapeString(filePath), html.EscapeString(displayName))
			}
		}
		sb.WriteString("</div>\n")
	}
}

// generateBlocksHTML generates HTML from blog-type blocks
func (hg *HTMLGenerator) generateBlocksHTML(sb *strings.Builder, post Post, texts i18nTexts) {
	// Track order for images and files separately
	imageOrder := 0
	fileOrder := 0

	for _, block := range *post.Body.Blocks {
		switch block.Type {
		case "header":
			if block.Text != "" {
				sb.WriteString(fmt.Sprintf("<h2>%s</h2>\n", html.EscapeString(block.Text)))
			}
		case "p":
			if block.Text != "" {
				sb.WriteString(fmt.Sprintf("<p>%s</p>\n", html.EscapeString(block.Text)))
			}
		case "image":
			if block.ImageID != nil && post.Body.ImageMap != nil {
				if img, ok := (*post.Body.ImageMap)[*block.ImageID]; ok {
					imgName := hg.getImageFileName(post, imageOrder, img)
					imgPath := hg.getRelativePath(imgName)
					sb.WriteString(fmt.Sprintf("<p><img src=\"%s\" alt=\"%s\"></p>\n", html.EscapeString(imgPath), html.EscapeString(filepath.Base(imgName))))
					imageOrder++
				}
			}
		case "file":
			if block.FileID != nil && post.Body.FileMap != nil {
				if file, ok := (*post.Body.FileMap)[*block.FileID]; ok {
					fileName := hg.getFileFileName(post, fileOrder, file)
					displayName := fmt.Sprintf("%s.%s", hg.cleanDisplayName(file.Name), file.Extension)
					filePath := hg.getRelativePath(fileName)
					sb.WriteString(fmt.Sprintf("<p><a href=\"%s\">%s</a></p>\n", html.EscapeString(filePath), html.EscapeString(displayName)))

					// If it's an image file, also embed it
					if isImageExtension(file.Extension) {
						sb.WriteString(fmt.Sprintf("<p><img src=\"%s\" alt=\"%s\"></p>\n", html.EscapeString(filePath), html.EscapeString(displayName)))
					}
					fileOrder++
				}
			}
		}
	}
}

// isImageExtension checks if the file extension is an image
func isImageExtension(ext string) bool {
	ext = strings.ToLower(ext)
	return ext == "png" || ext == "jpg" || ext == "jpeg" || ext == "gif" || ext == "webp" || ext == "bmp" || ext == "svg"
}

// getRelativePath returns the relative path for a file based on DirByPost setting
func (hg *HTMLGenerator) getRelativePath(fileName string) string {
	if hg.DirByPost {
		// When DirByPost is true, HTML is in the root of creator/planDir/
		// but files are in creator/planDir/postDir/
		// So we need to use just the basename since they're in the same directory
		return filepath.Base(fileName)
	}
	// When DirByPost is false, both HTML and files are in creator/planDir/
	// So we can use just the filename
	return filepath.Base(fileName)
}

// getImageFileName generates the filename for an image (matching LocalStorage logic)
func (hg *HTMLGenerator) getImageFileName(post Post, order int, img Image) string {
	date, err := time.Parse(time.RFC3339, post.PublishedDateTime)
	if err != nil {
		return fmt.Sprintf("%d-%s.%s", order, img.ID, img.Extension)
	}

	title := strings.TrimSpace(hg.escapeString(post.Title))
	title = hg.cleanDisplayName(title)

	displayName := hg.escapeString(img.ID)
	displayName = hg.cleanDisplayName(displayName)

	if hg.DirByPost {
		// Files are in the post directory, so just return the filename
		return fmt.Sprintf("%d-%s.%s", order, displayName, img.Extension)
	}

	// Files are in the creator directory with full naming
	return hg.limitOsSafely(fmt.Sprintf(
		"%s-%s-%d-%s",
		date.UTC().Format("2006-01-02"),
		title,
		order,
		displayName,
	)) + "." + img.Extension
}

// getFileFileName generates the filename for a file (matching LocalStorage logic)
func (hg *HTMLGenerator) getFileFileName(post Post, order int, file File) string {
	date, err := time.Parse(time.RFC3339, post.PublishedDateTime)
	if err != nil {
		return fmt.Sprintf("file-%d-%s.%s", order, file.ID, file.Extension)
	}

	title := strings.TrimSpace(hg.escapeString(post.Title))
	title = hg.cleanDisplayName(title)

	displayName := hg.escapeString(file.ID)
	displayName = hg.cleanDisplayName(displayName)

	if hg.DirByPost {
		// Files are in the post directory, so just return the filename
		return fmt.Sprintf("file-%d-%s.%s", order, displayName, file.Extension)
	}

	// Files are in the creator directory with full naming
	return hg.limitOsSafely(fmt.Sprintf(
		"%s-%s-file-%d-%s",
		date.UTC().Format("2006-01-02"),
		title,
		order,
		displayName,
	)) + "." + file.Extension
}

// GetHTMLFileName generates the filename for the HTML file
func (hg *HTMLGenerator) GetHTMLFileName(saveDir string, post Post, creatorName string) string {
	publishedTime, _ := time.Parse(time.RFC3339, post.PublishedDateTime)

	// Sanitize title for filename
	title := strings.TrimSpace(post.Title)
	title = strings.Map(func(r rune) rune {
		if filepath.IsAbs(string(r)) || filepath.Separator == r {
			return '-'
		}
		if r == '\\' || r == '/' || r == ':' || r == '*' || r == '?' || r == '"' || r == '<' || r == '>' || r == '|' {
			return '-'
		}
		return r
	}, title)

	if len(title) > 100 {
		title = title[:100]
	}

	planDir := ""
	if hg.DirByPlan {
		planDir = fmt.Sprintf("%dyen", post.FeeRequired)
	}

	if hg.DirByPost {
		// [SaveDirectory]/[CreatorID]/[PlanDir]/[PostDir]/post.html
		postDir := hg.limitOsSafely(fmt.Sprintf("%s-%s", publishedTime.UTC().Format("2006-01-02"), title))
		return filepath.Join(saveDir, post.CreatorID, planDir, postDir, "post.html")
	}

	// [SaveDirectory]/[CreatorID]/[PlanDir]/[yyyy-MM-dd-HHmmss] (postId) title.html
	formattedTime := publishedTime.Format("2006-01-02-150405")
	filename := fmt.Sprintf("[%s] (%s) %s.html", formattedTime, post.ID, title)

	return filepath.Join(saveDir, post.CreatorID, planDir, filename)
}
