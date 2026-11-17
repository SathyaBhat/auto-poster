package store

import (
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const feed = "https://example.com/rss"

	if known, _ := s.FeedKnown(feed); known {
		t.Fatal("feed known before seeding")
	}
	if err := s.Seed(feed, map[string]string{"old": "https://example.com/old"}); err != nil {
		t.Fatal(err)
	}
	if known, _ := s.FeedKnown(feed); !known {
		t.Fatal("feed not known after seeding")
	}
	if seeded, _ := s.Seeded(feed, "old"); !seeded {
		t.Error("old item not seeded")
	}
	if seeded, _ := s.Seeded(feed, "new"); seeded {
		t.Error("new item seeded")
	}

	_ = s.Record(feed, "new", "acct", "l", StatusFailed, "boom")
	_ = s.Record(feed, "new", "acct", "l", StatusPosted, "url")
	status, attempts, err := s.Status(feed, "new", "acct")
	if err != nil || status != StatusPosted || attempts != 2 {
		t.Errorf("got %q %d %v", status, attempts, err)
	}
	if status, _, _ := s.Status(feed, "new", "other"); status != "" {
		t.Errorf("other account status = %q", status)
	}
}
