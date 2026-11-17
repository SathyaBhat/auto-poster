// Package store records which feed items have been posted to which accounts.
package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StatusSeeded = "seeded" // existed before the service first saw the feed
	StatusPosted = "posted"
	StatusFailed = "failed"
)

// seedAccount marks an item as seen for every account.
const seedAccount = "*"

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS feeds (
	url        TEXT PRIMARY KEY,
	created_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS deliveries (
	feed_url   TEXT NOT NULL,
	guid       TEXT NOT NULL,
	account_id TEXT NOT NULL,
	link       TEXT NOT NULL,
	status     TEXT NOT NULL,
	attempts   INTEGER NOT NULL DEFAULT 0,
	detail     TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL,
	PRIMARY KEY (feed_url, guid, account_id)
);`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// FeedKnown reports whether the feed has been seeded.
func (s *Store) FeedKnown(url string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM feeds WHERE url = ?`, url).Scan(&n)
	return n > 0, err
}

// Seed marks every given item as already handled and records the feed, in
// one transaction, so existing posts are never published.
func (s *Store) Seed(url string, items map[string]string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for guid, link := range items {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO deliveries
			(feed_url, guid, account_id, link, status, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			url, guid, seedAccount, link, StatusSeeded, now()); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO feeds (url, created_at) VALUES (?, ?)`, url, now()); err != nil {
		return err
	}
	return tx.Commit()
}

// Seeded reports whether an item was present when the feed was first seen.
func (s *Store) Seeded(feedURL, guid string) (bool, error) {
	status, _, err := s.Status(feedURL, guid, seedAccount)
	return status == StatusSeeded, err
}

// Status returns the delivery status and attempt count, or "" if none.
func (s *Store) Status(feedURL, guid, accountID string) (string, int, error) {
	var status string
	var attempts int
	err := s.db.QueryRow(`SELECT status, attempts FROM deliveries
		WHERE feed_url = ? AND guid = ? AND account_id = ?`, feedURL, guid, accountID).
		Scan(&status, &attempts)
	if err == sql.ErrNoRows {
		return "", 0, nil
	}
	return status, attempts, err
}

// Record stores a delivery outcome. Each call counts as one attempt.
func (s *Store) Record(feedURL, guid, accountID, link, status, detail string) error {
	_, err := s.db.Exec(`
INSERT INTO deliveries (feed_url, guid, account_id, link, status, attempts, detail, updated_at)
VALUES (?, ?, ?, ?, ?, 1, ?, ?)
ON CONFLICT (feed_url, guid, account_id) DO UPDATE SET
	status = excluded.status,
	attempts = deliveries.attempts + 1,
	detail = excluded.detail,
	updated_at = excluded.updated_at`,
		feedURL, guid, accountID, link, status, detail, now())
	return err
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }
