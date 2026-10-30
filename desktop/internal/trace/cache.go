package trace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const defaultCacheTTL = 30 * 24 * time.Hour

type Cache struct {
	mu            sync.RWMutex
	deleted       map[string]bool
	archiveErrors map[string]bool
	projectionMu  sync.Mutex
	projections   map[string]archiveProjection
	root          string
	ttl           time.Duration
	now           func() time.Time
}

func NewCache(root string) (*Cache, error) {
	if !filepath.IsAbs(root) {
		return nil, errors.New("invalid trace cache path")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return nil, err
	}
	return &Cache{root: root, ttl: defaultCacheTTL, now: time.Now}, nil
}

func cacheKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (c *Cache) dir(sessionID, invocationID string) string {
	return filepath.Join(c.root, cacheKey(sessionID), cacheKey(invocationID))
}

func (c *Cache) LoadTrace(sessionID, invocationID string) (Trace, bool) {
	var out Trace
	if c == nil || !validID(sessionID) || !validID(invocationID) {
		return out, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.deleted[sessionID] {
		return out, false
	}
	out, found := c.loadSnapshot(sessionID, invocationID)
	if local, ok := c.archivedTrace(sessionID, invocationID); ok {
		out, found = withLocalEvents(out, local), true
	}
	if c.archiveErrors[c.dir(sessionID, invocationID)] && found {
		out.Coverage = safeMap(out.Coverage)
		out.Coverage["localArchiveStatus"] = "failed"
	}
	// A renderer upgrade must never invalidate the only surviving trace after
	// Runtime retention expires. Enrichment can add data without discarding it.
	return out, found
}

func (c *Cache) SaveTrace(sessionID, invocationID string, value Trace) error {
	_, err := c.MergeTrace(sessionID, invocationID, value)
	return err
}

func (c *Cache) loadSnapshot(sessionID, invocationID string) (Trace, bool) {
	var out Trace
	data, err := os.ReadFile(filepath.Join(c.dir(sessionID, invocationID), "trace.json"))
	if err != nil || json.Unmarshal(data, &out) != nil || out.SessionID != sessionID || out.InvocationID != invocationID {
		return Trace{}, false
	}
	return out, true
}

// MergeTrace keeps read/merge/write in one critical section so concurrent
// prefetch and manual refresh cannot discard each other's evidence.
func (c *Cache) MergeTrace(sessionID, invocationID string, value Trace) (Trace, error) {
	if c == nil || !validID(sessionID) || !validID(invocationID) {
		return value, nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deleted[sessionID] {
		return value, nil
	}
	if value.SessionID != sessionID || value.InvocationID != invocationID {
		return value, errors.New("trace identity mismatch")
	}
	if previous, found := c.loadSnapshot(sessionID, invocationID); found {
		if traceSettled(previous) && traceStillActive(value) {
			value, previous = previous, value
		}
		seen := map[string]bool{}
		for _, step := range value.Steps {
			seen[step.StepID] = true
		}
		for _, step := range previous.Steps {
			if !seen[step.StepID] {
				value.Steps = append(value.Steps, step)
			}
		}
	}
	markTraceProjectionVersion(&value)
	err := c.writeJSON(filepath.Join(c.dir(sessionID, invocationID), "trace.json"), value)
	if local, found := c.archivedTrace(sessionID, invocationID); found {
		value = withLocalEvents(value, local)
	}
	if err != nil || c.archiveErrors[c.dir(sessionID, invocationID)] {
		value.Coverage = safeMap(value.Coverage)
		value.Coverage["localArchiveStatus"] = "failed"
	}
	return value, err
}

func traceSettled(value Trace) bool {
	switch stringValue(value.Run["status"]) {
	case "success", "failed", "cancelled", "timeout":
		return true
	}
	return false
}

func (c *Cache) SaveDiagnostics(sessionID, invocationID string, value any) error {
	if c == nil || !validID(sessionID) || !validID(invocationID) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deleted[sessionID] {
		return nil
	}
	return c.writeJSON(filepath.Join(c.dir(sessionID, invocationID), "diagnostics.json"), value)
}

func (c *Cache) writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".trace-*.tmp")
	if err != nil {
		return err
	}
	tmp := file.Name()
	defer os.Remove(tmp)
	if _, err = file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Cache) DeleteSession(sessionID string) error {
	if c == nil || !validID(sessionID) {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deleted == nil {
		c.deleted = map[string]bool{}
	}
	c.deleted[sessionID] = true
	return os.RemoveAll(filepath.Join(c.root, cacheKey(sessionID)))
}

func (c *Cache) sessionDeleted(sessionID string) bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.deleted[sessionID]
}

func (c *Cache) Sweep() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	cutoff := c.now().Add(-c.ttl)
	entries, err := os.ReadDir(c.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(c.root, entry.Name())
		turns, readErr := os.ReadDir(path)
		if readErr != nil {
			continue
		}
		for _, turn := range turns {
			if !turn.IsDir() {
				continue
			}
			turnPath := filepath.Join(path, turn.Name())
			files, readErr := os.ReadDir(turnPath)
			if readErr != nil {
				continue
			}
			latest := time.Time{}
			for _, file := range files {
				info, statErr := file.Info()
				if statErr != nil {
					latest = c.now()
					break
				}
				if info.ModTime().After(latest) {
					latest = info.ModTime()
				}
			}
			if !latest.IsZero() && latest.Before(cutoff) {
				_ = os.RemoveAll(turnPath)
			}
		}
		// Remove only an empty session directory; another turn may still be fresh.
		_ = os.Remove(path)
	}
	return nil
}

func (c *Cache) StartSweeper(ctx context.Context) {
	if c == nil {
		return
	}
	go func() {
		_ = c.Sweep()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = c.Sweep()
			}
		}
	}()
}
