package fanbox

import (
	"fmt"
	"html"
	"path/filepath"
	"strings"
	"time"
)

// HTMLGenerator generates HTML pages from FANBOX posts
type HTMLGenerator struct {
	Enable bool
}

// GenerateHTML generates an HTML page from a post and returns the HTML content
func (hg *HTMLGenerator) GenerateHTML(post Post, creatorName string) string {
	if !hg.Enable {
		return ""
	}

	var sb strings.Builder

	// HTML header
	sb.WriteString(`<!DOCTYPE html html lang="zh-CN">`)
	sb.WriteString("\n<html>\n<head>\n")
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
	sb.WriteString(fmt.Sprintf("<div class=\"post-meta\">\n"))
	sb.WriteString(fmt.Sprintf("发布时间: %s<br>\n", publishedTime.Format("2006/01/02 15:04")))
	sb.WriteString(fmt.Sprintf("作者: %s<br>\n", html.EscapeString(creatorName)))
	sb.WriteString(fmt.Sprintf("创作者ID: %s<br>\n", post.CreatorID))
	if post.FeeRequired > 0 {
		sb.WriteString(fmt.Sprintf("费用: %d円<br>\n", post.FeeRequired))
	}
	sb.WriteString(fmt.Sprintf("贴文ID: %s\n", post.ID))
	sb.WriteString("</div>\n")

	// Content
	sb.WriteString("<div class=\"content\">\n")

	if post.Body != nil {
		hg.generateBodyContent(&sb, post)
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
	sb.WriteString(fmt.Sprintf("<p>来源: <a href=\"https://www.fanbox.cc/@%s/posts/%s\" target=\"_blank\">https://www.fanbox.cc/@%s/posts/%s</a></p>\n",
		post.CreatorID, post.ID, post.CreatorID, post.ID))
	sb.WriteString("</div>\n")

	sb.WriteString("</body>\n</html>")

	return sb.String()
}

// generateBodyContent generates HTML content from post body
func (hg *HTMLGenerator) generateBodyContent(sb *strings.Builder, post Post) {
	if post.Body.Blocks != nil {
		hg.generateBlocksHTML(sb, post)
	}

	// Handle image-type posts
	if post.Body.Images != nil && len(*post.Body.Images) > 0 {
		sb.WriteString("<div class=\"images-section\">\n")
		sb.WriteString("<h2>图片</h2>\n")
		for i, img := range *post.Body.Images {
			imgOrder := i + 1
			imgName := fmt.Sprintf("%d.%s", imgOrder, img.Extension)
			sb.WriteString(fmt.Sprintf("<p><img src=\"%s\" alt=\"%s\"></p>\n", html.EscapeString(imgName), html.EscapeString(imgName)))
		}
		sb.WriteString("</div>\n")
	}

	// Handle file-type posts
	if post.Body.Files != nil && len(*post.Body.Files) > 0 {
		sb.WriteString("<div class=\"files-section\">\n")
		sb.WriteString("<h2>文件</h2>\n")
		for _, file := range *post.Body.Files {
			fileName := fmt.Sprintf("%s.%s", file.Name, file.Extension)
			sb.WriteString(fmt.Sprintf("<p><a href=\"%s\">%s</a></p>\n", html.EscapeString(fileName), html.EscapeString(fileName)))

			// If it's an image file, also embed it
			if isImageExtension(file.Extension) {
				sb.WriteString(fmt.Sprintf("<p><img src=\"%s\" alt=\"%s\"></p>\n", html.EscapeString(fileName), html.EscapeString(fileName)))
			}
		}
		sb.WriteString("</div>\n")
	}
}

// generateBlocksHTML generates HTML from blog-type blocks
func (hg *HTMLGenerator) generateBlocksHTML(sb *strings.Builder, post Post) {
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
					imgName := fmt.Sprintf("%s.%s", img.ID, img.Extension)
					sb.WriteString(fmt.Sprintf("<p><img src=\"%s\" alt=\"%s\"></p>\n", html.EscapeString(imgName), html.EscapeString(imgName)))
				}
			}
		case "file":
			if block.FileID != nil && post.Body.FileMap != nil {
				if file, ok := (*post.Body.FileMap)[*block.FileID]; ok {
					fileName := fmt.Sprintf("%s.%s", file.Name, file.Extension)
					sb.WriteString(fmt.Sprintf("<p><a href=\"%s\">%s</a></p>\n", html.EscapeString(fileName), html.EscapeString(fileName)))

					// If it's an image file, also embed it
					if isImageExtension(file.Extension) {
						sb.WriteString(fmt.Sprintf("<p><img src=\"%s\" alt=\"%s\"></p>\n", html.EscapeString(fileName), html.EscapeString(fileName)))
					}
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

// GetHTMLFileName generates the filename for the HTML file
func GetHTMLFileName(saveDir string, post Post, creatorName string) string {
	publishedTime, _ := time.Parse(time.RFC3339, post.PublishedDateTime)

	// Format: [yyyy-MM-dd-HHmmss] (postId) title.html
	formattedTime := publishedTime.Format("2006-01-02-150405")

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

	filename := fmt.Sprintf("[%s] (%s) %s.html", formattedTime, post.ID, title)

	return filepath.Join(saveDir, filename)
}
