package store

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"strata-proof/internal/domain"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db        *sql.DB
	cacheMu   sync.RWMutex
	snapshots map[string]domain.Snapshot
}

func Open(path string) (*SQLiteStore, error) {
	dsn := path
	if path != ":memory:" {
		dsn = "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开 SQLite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接 SQLite: %w", err)
	}
	store := &SQLiteStore{db: db, snapshots: map[string]domain.Snapshot{}}
	if err := store.migrate(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error { return s.db.Close() }

func (s *SQLiteStore) cachedSnapshot(id string) (domain.Snapshot, bool) {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	snapshot, ok := s.snapshots[id]
	if !ok {
		return domain.Snapshot{}, false
	}
	// 返回深拷贝，避免调用方在保存失败前就地修改缓存中的单位/关系等切片底层数组，
	// 从而导致缓存状态与持久化状态不一致。
	return deepCopySnapshot(snapshot), true
}

func (s *SQLiteStore) rememberSnapshot(snapshot domain.Snapshot) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.snapshots[snapshot.Dossier.ID] = deepCopySnapshot(snapshot)
}

// deepCopySnapshot 返回一份与原快照无共享切片底层数组的拷贝。
// 应用层会在变更回调中对 Units/Relations/Findings/CheckBatches/
// RemediationItems/Credentials/Audit 等切片做就地修改或 append，
// 若直接缓存原对象，保存失败时缓存会被未落库的修改污染。
func deepCopySnapshot(snapshot domain.Snapshot) domain.Snapshot {
	out := snapshot
	out.Units = copySlice(snapshot.Units)
	out.Relations = copySlice(snapshot.Relations)
	out.Findings = copySlice(snapshot.Findings)
	out.CheckBatches = copySlice(snapshot.CheckBatches)
	out.RemediationItems = copySlice(snapshot.RemediationItems)
	out.Credentials = copySlice(snapshot.Credentials)
	out.Audit = copySlice(snapshot.Audit)
	if snapshot.Manifest != nil {
		manifest := *snapshot.Manifest
		out.Manifest = &manifest
	}
	if snapshot.Review != nil {
		review := *snapshot.Review
		review.RemediationItems = copySlice(snapshot.Review.RemediationItems)
		out.Review = &review
	}
	return out
}

func copySlice[T any, S ~[]T](items S) []T {
	if items == nil {
		return nil
	}
	out := make([]T, len(items))
	copy(out, items)
	return out
}
