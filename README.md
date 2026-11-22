# auto-poster

Polls blog RSS feeds and cross-posts new posts through [EchoBridge](https://github.com/sathyabhat/echobridge).

Each post contains the title, the excerpt, the link and the post's tags as hashtags:

```
Weekly Notes 46/2026

Week 46 summary - meeting an old friend, and trying a new sport.

https://sathyabh.at/weekly-notes-46-2026/

#Sydney #Life #WeeklyNotes #Gaming
```

## How it works

- **Tags** come from the feed's `<category>` elements (Ghost). If an item has none, as with Hugo's default RSS template, they're read from the post page's `<meta property="article:tag">` tags.
- **Hashtags:** multi-word and hyphenated tags become CamelCase (`Self-Hosting` → `#SelfHosting`), because Mastodon and Bluesky end a hashtag at a hyphen.
- **Length:** each account gets its own post sized to its platform: 300 characters for Bluesky, 500 for Mastodon. If the text is too long, the excerpt is shortened first. If there's no room for at least 40 characters of excerpt, the excerpt is dropped, then hashtags from the end. The link is always kept.
- **First run:** the first time a feed is seen, every post in it is marked as already posted, so the backlog is never published.
- **State:** SQLite records each post per account. If one account fails, only that account is retried on the next poll, up to `MAX_ATTEMPTS` times.
- **Old posts:** posts older than `MAX_AGE` that show up later are skipped, e.g. when an old post's GUID changes.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `FEEDS` | required | Comma-separated RSS feed URLs |
| `ECHOBRIDGE_URL` | required | EchoBridge base URL including any path prefix, e.g. `https://host/echobridge` |
| `ECHOBRIDGE_API_TOKEN` | none | EchoBridge API token, sent as `Authorization: Bearer <token>`. Must match the server's `ECHOBRIDGE_API_TOKEN` |
| `ACCOUNT_IDS` | all accounts | Comma-separated EchoBridge account IDs to post to |
| `DB_PATH` | `./data/auto-poster.db` | SQLite state file |
| `POLL_INTERVAL` | `15m` | Time between polls (minimum `1m`) |
| `MAX_AGE` | `168h` | Skip posts published longer ago than this |
| `MAX_ATTEMPTS` | `3` | Failed attempts per account before giving up on a post |

To turn on the EchoBridge API token, deploy auto-poster with `ECHOBRIDGE_API_TOKEN` set first, then set the same token on EchoBridge. A server without a token ignores the header, but once the server requires one, every request without it fails with 401.

## Running

```bash
# Preview what would be posted; posts nothing and writes no state
FEEDS=... ECHOBRIDGE_URL=... go run ./cmd/auto-poster --dry-run

# Single poll, then exit
go run ./cmd/auto-poster --once

# Run continuously
go run ./cmd/auto-poster
```

`docker-compose.yml` is set up for Dokploy. Set `FEEDS`, `ECHOBRIDGE_URL` and, if EchoBridge requires it, `ECHOBRIDGE_API_TOKEN` in the app's environment.

```bash
go test ./...
```
