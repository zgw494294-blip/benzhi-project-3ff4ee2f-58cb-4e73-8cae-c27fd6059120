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
	return snapshot, ok
}

func (s *SQLiteStore) rememberSnapshot(snapshot domain.Snapshot) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.snapshots[snapshot.Dossier.ID] = snapshot
}
