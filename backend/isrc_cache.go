package backend

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	isrcCacheDBFile = "isrc_cache.db"
	isrcCacheBucket = "SpotifyTrackISRC"
)

type isrcCacheEntry struct {
	TrackID   string `json:"track_id"`
	ISRC      string `json:"isrc"`
	UpdatedAt int64  `json:"updated_at"`
}

var (
	isrcCacheDB    *bolt.DB
	isrcCacheDBMu  sync.RWMutex
	memoryISRC     = make(map[string]string)
	memoryISRCMu   sync.RWMutex
	isrcDBDisabled bool
)

func InitISRCCacheDB() error {
	isrcCacheDBMu.Lock()
	defer isrcCacheDBMu.Unlock()

	if isrcCacheDB != nil || isrcDBDisabled {
		return nil
	}

	appDir, err := EnsureAppDir()
	if err != nil {
		isrcDBDisabled = true
		return nil
	}

	dbPath := filepath.Join(appDir, isrcCacheDBFile)
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 500 * time.Millisecond})
	if err != nil {
		fmt.Printf("Warning: ISRC cache DB locked, using in-memory cache: %v\n", err)
		isrcDBDisabled = true
		return nil
	}

	if err := db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(isrcCacheBucket))
		return err
	}); err != nil {
		_ = db.Close()
		isrcDBDisabled = true
		return nil
	}

	isrcCacheDB = db
	return nil
}

func CloseISRCCacheDB() {
	isrcCacheDBMu.Lock()
	defer isrcCacheDBMu.Unlock()

	if isrcCacheDB != nil {
		_ = isrcCacheDB.Close()
		isrcCacheDB = nil
	}
}

func GetCachedISRC(trackID string) (string, error) {
	normalizedTrackID := strings.TrimSpace(trackID)
	if normalizedTrackID == "" {
		return "", nil
	}

	memoryISRCMu.RLock()
	if cached, ok := memoryISRC[normalizedTrackID]; ok {
		memoryISRCMu.RUnlock()
		return cached, nil
	}
	memoryISRCMu.RUnlock()

	if err := InitISRCCacheDB(); err != nil || isrcCacheDB == nil {
		return "", nil
	}

	isrcCacheDBMu.RLock()
	defer isrcCacheDBMu.RUnlock()

	if isrcCacheDB == nil {
		return "", nil
	}

	var cachedISRC string
	_ = isrcCacheDB.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(isrcCacheBucket))
		if bucket == nil {
			return nil
		}

		value := bucket.Get([]byte(normalizedTrackID))
		if len(value) == 0 {
			return nil
		}

		var entry isrcCacheEntry
		if err := json.Unmarshal(value, &entry); err != nil {
			return err
		}

		cachedISRC = strings.ToUpper(strings.TrimSpace(entry.ISRC))
		return nil
	})

	if cachedISRC != "" {
		memoryISRCMu.Lock()
		memoryISRC[normalizedTrackID] = cachedISRC
		memoryISRCMu.Unlock()
	}

	return cachedISRC, nil
}

func PutCachedISRC(trackID string, isrc string) error {
	normalizedTrackID := strings.TrimSpace(trackID)
	normalizedISRC := strings.ToUpper(strings.TrimSpace(isrc))
	if normalizedTrackID == "" || normalizedISRC == "" {
		return nil
	}

	memoryISRCMu.Lock()
	memoryISRC[normalizedTrackID] = normalizedISRC
	memoryISRCMu.Unlock()

	if isrcDBDisabled {
		return nil
	}

	if err := InitISRCCacheDB(); err != nil || isrcCacheDB == nil {
		return nil
	}

	entry := isrcCacheEntry{
		TrackID:   normalizedTrackID,
		ISRC:      normalizedISRC,
		UpdatedAt: time.Now().Unix(),
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to encode ISRC cache entry: %w", err)
	}

	isrcCacheDBMu.RLock()
	db := isrcCacheDB
	isrcCacheDBMu.RUnlock()

	if db == nil {
		return nil
	}

	return db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte(isrcCacheBucket))
		if err != nil {
			return err
		}
		return bucket.Put([]byte(normalizedTrackID), payload)
	})
}
