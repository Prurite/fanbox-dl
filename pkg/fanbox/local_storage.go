package fanbox

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"

	"github.com/hareku/go-filename"
	"github.com/hareku/go-strlimit"
)

type LocalStorage struct {
	SaveDir   string
	DirByPost bool
	DirByPlan bool

	RemoveUnprintableChars bool
	EnableSaveJSON         bool
	EnableSaveHTML         bool
}

func (s *LocalStorage) Save(post Post, order int, d Downloadable, r io.Reader) error {
	name := s.makeFileName(post, order, d)

	dir := filepath.Dir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0775)
		if err != nil {
			return fmt.Errorf("create a directory (%s): %w", dir, err)
		}
	}

	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE, 0775)
	if err != nil {
		return fmt.Errorf("open a file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = io.Copy(file, r)
	if err != nil {
		// Remove the crashed file
		fileName := file.Name()
		_ = file.Close()

		if removeRrr := os.Remove(fileName); removeRrr != nil {
			return fmt.Errorf("file copying error and couldn't remove a crashed file (%s): %w", file.Name(), removeRrr)
		}

		return fmt.Errorf("file copying error: %w", err)
	}

	return nil
}

func (s *LocalStorage) Exist(post Post, order int, d Downloadable) (bool, error) {
	_, err := os.Stat(s.makeFileName(post, order, d))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat file: %w", err)
	}

	return true, nil
}

// SaveText saves the text content of a post to a text file
func (s *LocalStorage) SaveText(post Post) error {
	textContent := post.GetTextContent()
	if textContent == "" {
		return nil
	}

	name := s.makeTextFileName(post)

	dir := filepath.Dir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0775)
		if err != nil {
			return fmt.Errorf("create a directory (%s): %w", dir, err)
		}
	}

	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE, 0775)
	if err != nil {
		return fmt.Errorf("open file to save text: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	_, err = file.WriteString(textContent)
	if err != nil {
		return fmt.Errorf("write text content: %w", err)
	}

	return nil
}

// TextExists checks if the text file already exists
func (s *LocalStorage) TextExists(post Post) (bool, error) {
	_, err := os.Stat(s.makeTextFileName(post))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat file: %w", err)
	}

	return true, nil
}

// JSONExists checks if the JSON file already exists
func (s *LocalStorage) JSONExists(post Post) (bool, error) {
	if !s.EnableSaveJSON {
		return false, nil
	}

	_, err := os.Stat(s.makeJSONFileName(post))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat file: %w", err)
	}

	return true, nil
}

// limitOsSafely limits the string length for OS safely.
func (s *LocalStorage) limitOsSafely(name string) string {
	switch runtime.GOOS {
	case "windows":
		return strlimit.LimitRunesWithEnd(name, 210, "...")
	default:
		return strlimit.LimitBytesWithEnd(name, 250, "...")
	}
}

func (s *LocalStorage) makeFileName(post Post, order int, d Downloadable) string {
	date, err := time.Parse(time.RFC3339, post.PublishedDateTime)
	if err != nil {
		panic(fmt.Errorf("parse post published date time %s: %w", post.PublishedDateTime, err))
	}

	title := strings.TrimSpace(filename.EscapeString(post.Title, "-"))
	if s.RemoveUnprintableChars {
		title = strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}
			return -1
		}, title)
	}

	// Get display name (file name or ID)
	displayName := d.GetName()
	// Escape the display name to make it file system safe
	displayName = filename.EscapeString(displayName, "-")
	if s.RemoveUnprintableChars {
		displayName = strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}
			return -1
		}, displayName)
	}

	fileType := ""
	// for backward-compatibility, insert "-file-" identifier
	if _, ok := d.(File); ok {
		fileType = "file-"
	}

	planDir := ""
	if s.DirByPlan {
		planDir = fmt.Sprintf("%dyen", post.FeeRequired)
	}

	if s.DirByPost {
		// [SaveDirectory]/[CreatorID]/2006-01-02-[Post Title]/[Order]-[Name].[Extension]
		return filepath.Join(
			s.SaveDir,
			post.CreatorID,
			planDir,
			s.limitOsSafely(fmt.Sprintf("%s-%s", date.UTC().Format("2006-01-02"), title)),
			fmt.Sprintf("%s%d-%s.%s", fileType, order, displayName, d.GetExtension()),
		)
	}

	// [SaveDirectory]/[CreatorID]/2006-01-02-[Post Title]-[Order]-[Name].[Extension]
	return filepath.Join(
		s.SaveDir,
		post.CreatorID,
		planDir,
		fmt.Sprintf(
			"%s.%s",
			s.limitOsSafely(
				fmt.Sprintf(
					"%s-%s-%s%d-%s",
					date.UTC().Format("2006-01-02"),
					title,
					fileType,
					order,
					displayName,
				),
			),
			d.GetExtension(),
		),
	)
}

// makeTextFileName generates the filename for the text content
func (s *LocalStorage) makeTextFileName(post Post) string {
	date, err := time.Parse(time.RFC3339, post.PublishedDateTime)
	if err != nil {
		panic(fmt.Errorf("parse post published date time %s: %w", post.PublishedDateTime, err))
	}

	title := strings.TrimSpace(filename.EscapeString(post.Title, "-"))
	if s.RemoveUnprintableChars {
		title = strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}
			return -1
		}, title)
	}

	planDir := ""
	if s.DirByPlan {
		planDir = fmt.Sprintf("%dyen", post.FeeRequired)
	}

	if s.DirByPost {
		// [SaveDirectory]/[CreatorID]/2006-01-02-[Post Title]/post.txt
		return filepath.Join(
			s.SaveDir,
			post.CreatorID,
			planDir,
			s.limitOsSafely(fmt.Sprintf("%s-%s", date.UTC().Format("2006-01-02"), title)),
			"post.txt",
		)
	}

	// [SaveDirectory]/[CreatorID]/2006-01-02-[Post Title]-post.txt
	return filepath.Join(
		s.SaveDir,
		post.CreatorID,
		planDir,
		s.limitOsSafely(fmt.Sprintf("%s-%s-post.txt", date.UTC().Format("2006-01-02"), title)),
	)
}

// makeJSONFileName generates the filename for the JSON response
func (s *LocalStorage) makeJSONFileName(post Post) string {
	if !s.EnableSaveJSON {
		return ""
	}

	date, err := time.Parse(time.RFC3339, post.PublishedDateTime)
	if err != nil {
		return ""
	}

	title := strings.TrimSpace(filename.EscapeString(post.Title, "-"))
	if s.RemoveUnprintableChars {
		title = strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}
			return -1
		}, title)
	}

	planDir := ""
	if s.DirByPlan {
		planDir = fmt.Sprintf("%dyen", post.FeeRequired)
	}

	if s.DirByPost {
		// [SaveDirectory]/[CreatorID]/2006-01-02-[Post Title]/post.json
		return filepath.Join(
			s.SaveDir,
			post.CreatorID,
			planDir,
			s.limitOsSafely(fmt.Sprintf("%s-%s", date.UTC().Format("2006-01-02"), title)),
			"post.json",
		)
	}

	// [SaveDirectory]/[CreatorID]/2006-01-02-[Post Title]-post.json
	return filepath.Join(
		s.SaveDir,
		post.CreatorID,
		planDir,
		s.limitOsSafely(fmt.Sprintf("%s-%s-post.json", date.UTC().Format("2006-01-02"), title)),
	)
}

// SaveJSON saves the original JSON response from the API
func (s *LocalStorage) SaveJSON(post Post, jsonData []byte) error {
	if !s.EnableSaveJSON {
		return nil
	}

	name := s.makeJSONFileName(post)
	if name == "" {
		return nil
	}

	dir := filepath.Dir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0775)
		if err != nil {
			return fmt.Errorf("create directory (%s): %w", dir, err)
		}
	}

	return os.WriteFile(name, jsonData, 0664)
}

// HTMLExists checks if the HTML file already exists
func (s *LocalStorage) HTMLExists(post Post) (bool, error) {
	if !s.EnableSaveHTML {
		return false, nil
	}

	_, err := os.Stat(s.makeHTMLFileName(post))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat file: %w", err)
	}

	return true, nil
}

// makeHTMLFileName generates the filename for the HTML file
func (s *LocalStorage) makeHTMLFileName(post Post) string {
	if !s.EnableSaveHTML {
		return ""
	}

	date, err := time.Parse(time.RFC3339, post.PublishedDateTime)
	if err != nil {
		return ""
	}

	title := strings.TrimSpace(filename.EscapeString(post.Title, "-"))
	if s.RemoveUnprintableChars {
		title = strings.Map(func(r rune) rune {
			if unicode.IsPrint(r) {
				return r
			}
			return -1
		}, title)
	}

	planDir := ""
	if s.DirByPlan {
		planDir = fmt.Sprintf("%dyen", post.FeeRequired)
	}

	if s.DirByPost {
		// [SaveDirectory]/[CreatorID]/[PlanDir]/[PostDir]/post.html
		return filepath.Join(
			s.SaveDir,
			post.CreatorID,
			planDir,
			s.limitOsSafely(fmt.Sprintf("%s-%s", date.UTC().Format("2006-01-02"), title)),
			"post.html",
		)
	}

	// DirByPost=false: [SaveDirectory]/[CreatorID]/[PlanDir]/[yyyy]-[MM]-[dd]-[HHmmss] (postId) title.html
	return filepath.Join(
		s.SaveDir,
		post.CreatorID,
		planDir,
		fmt.Sprintf("[%s] (%s) %s.html",
			date.UTC().Format("2006-01-02-150405"),
			post.ID,
			s.limitOsSafely(title)),
	)
}

// SaveHTML saves the HTML content of a post
func (s *LocalStorage) SaveHTML(post Post, htmlContent string) error {
	if !s.EnableSaveHTML {
		return nil
	}

	name := s.makeHTMLFileName(post)
	if name == "" {
		return nil
	}

	dir := filepath.Dir(name)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0775)
		if err != nil {
			return fmt.Errorf("create directory (%s): %w", dir, err)
		}
	}

	return os.WriteFile(name, []byte(htmlContent), 0664)
}
