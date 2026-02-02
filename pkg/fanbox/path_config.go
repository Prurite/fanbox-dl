package fanbox

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// PathConfig manages custom save paths for creators
type PathConfig struct {
	configFile  string
	config      map[string]string
	defaultPath string

	mu sync.RWMutex
}

// NewPathConfig creates a new PathConfig
func NewPathConfig(configFile string) (*PathConfig, error) {
	if configFile == "" {
		configFile = filepath.Join(".", "SavePathList.json")
	}

	pc := &PathConfig{
		configFile:  configFile,
		config:      make(map[string]string),
		defaultPath: "", // Will be set from config or default
	}

	// Load existing config
	if err := pc.Load(); err != nil {
		slog.Warn("Failed to load path config, using defaults", "error", err)
		// Create default config
		pc.createDefaultConfig()
	}

	return pc, nil
}

// createDefaultConfig creates a default configuration
func (pc *PathConfig) createDefaultConfig() {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Set default save path to desktop/FanBox下载
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	pc.config["savePath"] = filepath.Join(homeDir, "Desktop", "FanBox下載")
	pc.defaultPath = "" // No category by default

	// Save default config
	_ = pc.Save()
}

// Load loads the path configuration from file
func (pc *PathConfig) Load() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	if _, err := os.Stat(pc.configFile); os.IsNotExist(err) {
		pc.config = make(map[string]string)
		return nil
	}

	data, err := os.ReadFile(pc.configFile)
	if err != nil {
		return fmt.Errorf("read SavePathList.json: %w", err)
	}

	if err := json.Unmarshal(data, &pc.config); err != nil {
		return fmt.Errorf("unmarshal SavePathList.json: %w", err)
	}

	// Set default path from config
	if defaultPath, ok := pc.config["default"]; ok {
		pc.defaultPath = defaultPath
	}

	slog.Info("Loaded path config", "file", pc.configFile)

	return nil
}

// Save saves the path configuration to file
func (pc *PathConfig) Save() error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Ensure directory exists
	dir := filepath.Dir(pc.configFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(pc.config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal path config: %w", err)
	}

	if err := os.WriteFile(pc.configFile, data, 0644); err != nil {
		return fmt.Errorf("write SavePathList.json: %w", err)
	}

	return nil
}

// GetSavePath returns the save path for a creator
func (pc *PathConfig) GetSavePath(creatorID, creatorName string) string {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	// Get base save path
	basePath := pc.config["savePath"]
	if basePath == "" {
		// Default to desktop/FanBox下載
		homeDir, err := os.UserHomeDir()
		if err != nil {
			basePath = "./downloads"
		} else {
			basePath = filepath.Join(homeDir, "Desktop", "FanBox下載")
		}
	}

	// Check for creator-specific path
	if creatorPath, ok := pc.config[creatorID]; ok {
		return filepath.Join(basePath, creatorPath, sanitizeCreatorName(creatorName))
	}

	// Use default path if configured
	if pc.defaultPath != "" {
		return filepath.Join(basePath, pc.defaultPath, sanitizeCreatorName(creatorName))
	}

	// Default: basePath/creatorName
	return filepath.Join(basePath, sanitizeCreatorName(creatorName))
}

// SetSavePath sets a custom save path for a creator
func (pc *PathConfig) SetSavePath(creatorID, path string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.config[creatorID] = path
	return pc.Save()
}

// SetBaseSavePath sets the base save path
func (pc *PathConfig) SetBaseSavePath(path string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.config["savePath"] = path
	return pc.Save()
}

// SetDefaultPath sets the default category path
func (pc *PathConfig) SetDefaultPath(path string) error {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	pc.defaultPath = path
	pc.config["default"] = path
	return pc.Save()
}

// sanitizeCreatorName sanitizes creator name for use in file path
func sanitizeCreatorName(name string) string {
	// Remove @ prefix if present
	name = strings.TrimPrefix(name, "@")

	// Replace invalid characters
	invalidChars := `<>:"/\|?*`
	for _, c := range invalidChars {
		name = strings.ReplaceAll(name, string(c), "_")
	}

	// Remove control characters
	name = strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, name)

	// Trim spaces and dots
	name = strings.Trim(name, " .")

	if name == "" {
		name = "UnknownCreator"
	}

	return name
}

// GetEnvSlash returns the appropriate path separator for the OS
func GetEnvSlash() string {
	if runtime.GOOS == "windows" {
		return "\\"
	}
	return "/"
}
