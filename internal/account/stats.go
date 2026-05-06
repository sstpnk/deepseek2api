package account

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"ds2api/internal/config"
)

const (
	statsFileVersion      = 1
	statsFlushInterval    = 2 * time.Second
	statsWindowSeconds    = 60
	statsWritePermissions = 0o644
)

type statsFile struct {
	Version       int   `json:"version"`
	TotalRequests int64 `json:"total_requests"`
	UpdatedAt     int64 `json:"updated_at_unix,omitempty"`
}

type runtimeStats struct {
	mu            sync.Mutex
	path          string
	totalRequests int64
	buckets       [statsWindowSeconds]int64
	bucketUnix    [statsWindowSeconds]int64
	lastFlush     time.Time
	dirty         bool
	flushPending  atomic.Bool
}

func newRuntimeStats(path string) *runtimeStats {
	s := &runtimeStats{path: path, lastFlush: time.Now()}
	if err := s.load(); err != nil {
		config.Logger.Warn("[runtime_stats] load failed", "path", path, "error", err)
	}
	return s
}

func newInMemoryRuntimeStats() *runtimeStats {
	return &runtimeStats{lastFlush: time.Now()}
}

func (s *runtimeStats) setPathForTest(path string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = path
	s.totalRequests = 0
	s.buckets = [statsWindowSeconds]int64{}
	s.bucketUnix = [statsWindowSeconds]int64{}
	s.lastFlush = time.Now()
	s.dirty = false
}

func (s *runtimeStats) record(now time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.totalRequests++
	s.recordBucketLocked(now)
	s.dirty = true
	s.flushSoonLocked(now)
}

func (s *runtimeStats) snapshot(now time.Time) (int64, int64) {
	if s == nil {
		return 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rpm := s.windowTotalLocked(now)
	return s.totalRequests, rpm
}

func (s *runtimeStats) flush() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushLocked(time.Now())
}

func (s *runtimeStats) flushSoonLocked(now time.Time) {
	if s == nil || s.path == "" || !s.dirty {
		return
	}
	if !s.flushPending.CompareAndSwap(false, true) {
		return
	}
	delay := statsFlushInterval - now.Sub(s.lastFlush)
	if delay < 0 {
		delay = 0
	}
	go func() {
		if delay > 0 {
			time.Sleep(delay)
		}
		s.flush()
		s.flushPending.Store(false)
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.dirty {
			s.flushSoonLocked(time.Now())
		}
	}()
}

func (s *runtimeStats) load() error {
	if s == nil || s.path == "" {
		return nil
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var file statsFile
	if err := json.Unmarshal(raw, &file); err != nil {
		return err
	}
	s.mu.Lock()
	s.totalRequests = file.TotalRequests
	s.mu.Unlock()
	return nil
}

func (s *runtimeStats) recordBucketLocked(now time.Time) {
	sec := now.Unix()
	idx := int(sec % statsWindowSeconds)
	if idx < 0 {
		idx = -idx
	}
	if s.bucketUnix[idx] != sec {
		s.bucketUnix[idx] = sec
		s.buckets[idx] = 0
	}
	s.buckets[idx]++
}

func (s *runtimeStats) windowTotalLocked(now time.Time) int64 {
	cutoff := now.Unix() - statsWindowSeconds + 1
	var total int64
	for i, sec := range s.bucketUnix {
		if sec >= cutoff {
			total += s.buckets[i]
		}
	}
	return total
}

func (s *runtimeStats) flushLocked(now time.Time) {
	if s == nil || !s.dirty || s.path == "" {
		return
	}
	body, err := json.MarshalIndent(statsFile{
		Version:       statsFileVersion,
		TotalRequests: s.totalRequests,
		UpdatedAt:     now.Unix(),
	}, "", "  ")
	if err != nil {
		config.Logger.Warn("[runtime_stats] encode failed", "error", err)
		return
	}
	if err := writeStatsFileAtomic(s.path, append(body, '\n')); err != nil {
		config.Logger.Warn("[runtime_stats] write failed", "path", s.path, "error", err)
		s.lastFlush = now
		return
	}
	s.lastFlush = now
	s.dirty = false
}

func writeStatsFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create runtime stats dir: %w", err)
		}
	}
	tmp, err := os.CreateTemp(dir, ".runtime-stats-*.tmp")
	if err != nil {
		return fmt.Errorf("create runtime stats temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		if err := os.Remove(tmpPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			config.Logger.Warn("[runtime_stats] temp cleanup failed", "path", tmpPath, "error", err)
		}
	}
	if _, err := tmp.Write(body); err != nil {
		closeErr := tmp.Close()
		cleanup()
		if closeErr != nil {
			return errors.Join(fmt.Errorf("write runtime stats temp: %w", err), fmt.Errorf("close runtime stats temp: %w", closeErr))
		}
		return fmt.Errorf("write runtime stats temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close runtime stats temp: %w", err)
	}
	if err := os.Chmod(tmpPath, statsWritePermissions); err != nil {
		cleanup()
		return fmt.Errorf("chmod runtime stats temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("promote runtime stats temp: %w", err)
	}
	return nil
}
