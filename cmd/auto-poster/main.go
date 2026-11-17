// Command auto-poster polls blog RSS feeds and cross-posts new posts through
// EchoBridge.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/sathyabhat/auto-poster/internal/compose"
	"github.com/sathyabhat/auto-poster/internal/echobridge"
	"github.com/sathyabhat/auto-poster/internal/feed"
	"github.com/sathyabhat/auto-poster/internal/store"
)

// Character limits per platform. Accounts on unknown providers use the smallest.
var limits = map[string]int{
	"bluesky":  300,
	"mastodon": 500,
}

const defaultLimit = 300

type config struct {
	feeds       []string
	echoURL     string
	accountIDs  []string
	dbPath      string
	interval    time.Duration
	maxAge      time.Duration
	maxAttempts int
}

func loadConfig() (config, error) {
	c := config{
		feeds:       splitList(os.Getenv("FEEDS")),
		echoURL:     os.Getenv("ECHOBRIDGE_URL"),
		accountIDs:  splitList(os.Getenv("ACCOUNT_IDS")),
		dbPath:      envOr("DB_PATH", "./data/auto-poster.db"),
		interval:    15 * time.Minute,
		maxAge:      7 * 24 * time.Hour,
		maxAttempts: 3,
	}
	if len(c.feeds) == 0 {
		return c, errors.New("FEEDS is required (comma-separated RSS URLs)")
	}
	if c.echoURL == "" {
		return c, errors.New("ECHOBRIDGE_URL is required")
	}
	var err error
	if v := os.Getenv("POLL_INTERVAL"); v != "" {
		if c.interval, err = time.ParseDuration(v); err != nil || c.interval < time.Minute {
			return c, fmt.Errorf("POLL_INTERVAL must be a duration of at least 1m, got %q", v)
		}
	}
	if v := os.Getenv("MAX_AGE"); v != "" {
		if c.maxAge, err = time.ParseDuration(v); err != nil {
			return c, fmt.Errorf("MAX_AGE: %w", err)
		}
	}
	if v := os.Getenv("MAX_ATTEMPTS"); v != "" {
		if c.maxAttempts, err = strconv.Atoi(v); err != nil || c.maxAttempts < 1 {
			return c, fmt.Errorf("MAX_ATTEMPTS must be a positive integer, got %q", v)
		}
	}
	return c, nil
}

func main() {
	once := flag.Bool("once", false, "run a single poll and exit")
	dryRun := flag.Bool("dry-run", false, "print what would be posted without posting or recording anything")
	flag.Parse()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.dbPath), 0o755); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(cfg.dbPath)
	if err != nil {
		log.Fatalf("open %s: %v", cfg.dbPath, err)
	}
	defer st.Close()

	httpClient := &http.Client{Timeout: 30 * time.Second}
	p := &poster{
		cfg:    cfg,
		store:  st,
		feeds:  &feed.Fetcher{Client: httpClient, UserAgent: "auto-poster/1.0"},
		echo:   &echobridge.Client{BaseURL: cfg.echoURL, HTTP: httpClient},
		dryRun: *dryRun,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Printf("watching %d feed(s), every %s, dry-run=%v", len(cfg.feeds), cfg.interval, *dryRun)
	p.poll(ctx)
	if *once || *dryRun {
		return
	}
	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

type poster struct {
	cfg    config
	store  *store.Store
	feeds  *feed.Fetcher
	echo   *echobridge.Client
	dryRun bool
}

func (p *poster) poll(ctx context.Context) {
	accounts, err := p.accounts(ctx)
	if err != nil {
		log.Printf("echobridge: %v", err)
		return
	}
	for _, url := range p.cfg.feeds {
		if err := p.pollFeed(ctx, url, accounts); err != nil {
			log.Printf("%s: %v", url, err)
		}
	}
}

// accounts returns the EchoBridge accounts to post to: ACCOUNT_IDS if set,
// otherwise every connected account.
func (p *poster) accounts(ctx context.Context) ([]echobridge.Account, error) {
	all, err := p.echo.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	if len(p.cfg.accountIDs) == 0 {
		return all, nil
	}
	var out []echobridge.Account
	for _, id := range p.cfg.accountIDs {
		i := slices.IndexFunc(all, func(a echobridge.Account) bool { return a.ID == id })
		if i < 0 {
			return nil, fmt.Errorf("account %s from ACCOUNT_IDS is not connected in EchoBridge", id)
		}
		out = append(out, all[i])
	}
	return out, nil
}

func (p *poster) pollFeed(ctx context.Context, url string, accounts []echobridge.Account) error {
	items, err := p.feeds.Fetch(ctx, url)
	if err != nil {
		return err
	}

	known, err := p.store.FeedKnown(url)
	if err != nil {
		return err
	}
	if !known {
		return p.seed(ctx, url, items, accounts)
	}

	// Oldest first, so posts go out in publishing order.
	slices.Reverse(items)
	for i := range items {
		if err := p.handleItem(ctx, url, &items[i], accounts); err != nil {
			log.Printf("%s: %v", items[i].Link, err)
		}
	}
	return nil
}

// seed marks everything currently in a new feed as handled, so the backlog
// is never posted.
func (p *poster) seed(ctx context.Context, url string, items []feed.Item, accounts []echobridge.Account) error {
	if p.dryRun {
		log.Printf("%s: first run, would mark %d existing post(s) as already posted", url, len(items))
		if len(items) > 0 {
			log.Printf("preview of the newest post:")
			p.preview(ctx, &items[0], accounts)
		}
		return nil
	}
	seen := make(map[string]string, len(items))
	for _, it := range items {
		seen[it.GUID] = it.Link
	}
	if err := p.store.Seed(url, seen); err != nil {
		return err
	}
	log.Printf("%s: first run, marked %d existing post(s) as already posted", url, len(items))
	return nil
}

func (p *poster) handleItem(ctx context.Context, url string, it *feed.Item, accounts []echobridge.Account) error {
	seeded, err := p.store.Seeded(url, it.GUID)
	if err != nil || seeded {
		return err
	}

	// Find accounts that still need this post.
	var pending []echobridge.Account
	for _, a := range accounts {
		status, attempts, err := p.store.Status(url, it.GUID, a.ID)
		if err != nil {
			return err
		}
		if status == store.StatusPosted || (status == store.StatusFailed && attempts >= p.cfg.maxAttempts) {
			continue
		}
		pending = append(pending, a)
	}
	if len(pending) == 0 {
		return nil
	}

	// Guard against old posts reappearing (changed GUID, re-dated post).
	if !it.Published.IsZero() && time.Since(it.Published) > p.cfg.maxAge {
		log.Printf("%s: skipping, published %s which is older than MAX_AGE", it.Link, it.Published.Format(time.DateOnly))
		if p.dryRun {
			return nil
		}
		return p.store.Seed(url, map[string]string{it.GUID: it.Link})
	}

	if err := p.feeds.FillTags(ctx, it); err != nil {
		// Tags are nice to have; post without them rather than not at all.
		log.Printf("%s: could not load tags from page: %v", it.Link, err)
	}

	for _, a := range pending {
		text := compose.Post(it.Title, it.Excerpt, it.Link, it.Tags, limitFor(a))
		if p.dryRun {
			printPost(a, text)
			continue
		}
		res, err := p.echo.Post(ctx, a.ID, text)
		switch {
		case err != nil:
			// The request failed before EchoBridge returned a per-account
			// result, e.g. a network error. Count it as an attempt.
			log.Printf("%s -> %s: %v", it.Link, label(a), err)
			if err := p.store.Record(url, it.GUID, a.ID, it.Link, store.StatusFailed, err.Error()); err != nil {
				return err
			}
		case !res.Success:
			log.Printf("%s -> %s: %s", it.Link, label(a), res.Error)
			if err := p.store.Record(url, it.GUID, a.ID, it.Link, store.StatusFailed, res.Error); err != nil {
				return err
			}
		default:
			log.Printf("%s -> %s: posted %s", it.Link, label(a), res.PostURL)
			if err := p.store.Record(url, it.GUID, a.ID, it.Link, store.StatusPosted, res.PostURL); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *poster) preview(ctx context.Context, it *feed.Item, accounts []echobridge.Account) {
	if err := p.feeds.FillTags(ctx, it); err != nil {
		log.Printf("%s: could not load tags from page: %v", it.Link, err)
	}
	for _, a := range accounts {
		printPost(a, compose.Post(it.Title, it.Excerpt, it.Link, it.Tags, limitFor(a)))
	}
}

func printPost(a echobridge.Account, text string) {
	fmt.Printf("----- %s (%d chars) -----\n%s\n\n", label(a), len([]rune(text)), text)
}

func limitFor(a echobridge.Account) int {
	if n, ok := limits[a.Provider]; ok {
		return n
	}
	return defaultLimit
}

func label(a echobridge.Account) string { return a.Provider + ":" + a.DisplayName }

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
