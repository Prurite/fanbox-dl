package fanbox

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// StateManager manages the download state (last post IDs, supporting list)
type StateManager struct {
	stateDir       string
	lastPostIDs    map[string]int64
	supportingList map[string][]SupportingCreator

	mu sync.RWMutex
}

// SupportingCreator represents a creator being supported
type SupportingCreator struct {
	CreatorID string `json:"creatorId"`
	Name      string `json:"name"`
	Fee       int    `json:"fee"`
}

// NewStateManager creates a new StateManager
func NewStateManager(stateDir string) (*StateManager, error) {
	if stateDir == "" {
		stateDir = "."
	}

	// Ensure state directory exists
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	sm := &StateManager{
		stateDir:       stateDir,
		lastPostIDs:    make(map[string]int64),
		supportingList: make(map[string][]SupportingCreator),
	}

	// Load existing state
	if err := sm.LoadLastPostIDs(); err != nil {
		slog.Warn("Failed to load last post IDs", "error", err)
	}

	if err := sm.LoadSupportingList(); err != nil {
		slog.Warn("Failed to load supporting list", "error", err)
	}

	return sm, nil
}

// LoadLastPostIDs loads the last saved post IDs from LastSavePostId.json
func (sm *StateManager) LoadLastPostIDs() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	path := filepath.Join(sm.stateDir, "LastSavePostId.json")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		sm.lastPostIDs = make(map[string]int64)
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read LastSavePostId.json: %w", err)
	}

	var lastPostIDs map[string]int64
	if err := json.Unmarshal(data, &lastPostIDs); err != nil {
		return fmt.Errorf("unmarshal LastSavePostId.json: %w", err)
	}

	sm.lastPostIDs = lastPostIDs
	slog.Info("Loaded last post IDs", "count", len(sm.lastPostIDs))

	return nil
}

// SaveLastPostIDs saves the last post IDs to LastSavePostId.json
func (sm *StateManager) SaveLastPostIDs() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	path := filepath.Join(sm.stateDir, "LastSavePostId.json")

	data, err := json.MarshalIndent(sm.lastPostIDs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal last post IDs: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write LastSavePostId.json: %w", err)
	}

	return nil
}

// GetLastPostID returns the last saved post ID for a creator
func (sm *StateManager) GetLastPostID(creatorID string) (int64, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	postID, exists := sm.lastPostIDs[creatorID]
	return postID, exists
}

// UpdateLastPostID updates the last saved post ID for a creator
func (sm *StateManager) UpdateLastPostID(creatorID string, postID int64) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.lastPostIDs[creatorID] = postID
	return sm.SaveLastPostIDs()
}

// ShouldSkipPost checks if a post should be skipped based on last saved ID
func (sm *StateManager) ShouldSkipPost(creatorID string, postID int64) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	lastID, exists := sm.lastPostIDs[creatorID]
	if !exists {
		return false
	}

	// Skip if this post ID is equal to or less than the last saved ID
	return postID <= lastID
}

// GetFirstPostIDToDownload returns the first post ID to download (last saved + 1)
func (sm *StateManager) GetFirstPostIDToDownload(creatorID string) int64 {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	lastID, exists := sm.lastPostIDs[creatorID]
	if !exists {
		return 0 // Download all posts
	}

	return lastID + 1
}

// LoadSupportingList loads the supporting list from SupportList.json
func (sm *StateManager) LoadSupportingList() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	path := filepath.Join(sm.stateDir, "SupportList.json")

	if _, err := os.Stat(path); os.IsNotExist(err) {
		sm.supportingList = make(map[string][]SupportingCreator)
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read SupportList.json: %w", err)
	}

	var supportingList map[string][]SupportingCreator
	if err := json.Unmarshal(data, &supportingList); err != nil {
		return fmt.Errorf("unmarshal SupportList.json: %w", err)
	}

	sm.supportingList = supportingList
	slog.Info("Loaded supporting list", "accounts", len(sm.supportingList))

	return nil
}

// SaveSupportingList saves the supporting list to SupportList.json
func (sm *StateManager) SaveSupportingList() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	path := filepath.Join(sm.stateDir, "SupportList.json")

	data, err := json.MarshalIndent(sm.supportingList, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal supporting list: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write SupportList.json: %w", err)
	}

	return nil
}

// AddSupportingCreator adds a creator to the supporting list for an account
func (sm *StateManager) AddSupportingCreator(accountID string, creator SupportingCreator) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if already exists
	for _, c := range sm.supportingList[accountID] {
		if c.CreatorID == creator.CreatorID {
			return nil
		}
	}

	sm.supportingList[accountID] = append(sm.supportingList[accountID], creator)
	return sm.SaveSupportingList()
}

// UpdateSupportingList updates the complete supporting list for an account
func (sm *StateManager) UpdateSupportingList(accountID string, creators []SupportingCreator) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.supportingList[accountID] = creators
	return sm.SaveSupportingList()
}

// GetSupportingList returns the supporting list for all accounts
func (sm *StateManager) GetSupportingList() map[string][]SupportingCreator {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	result := make(map[string][]SupportingCreator)
	for k, v := range sm.supportingList {
		result[k] = append([]SupportingCreator{}, v...)
	}

	return result
}
