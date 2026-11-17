// Package feed fetches RSS feeds and extracts the fields needed for a post.
package feed

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	nethtml "golang.org/x/net/html"
)

// Item is a single blog post from a feed.
type Item struct {
	GUID      string
	Title     string
	Link      string
	Excerpt   string
	Tags      []string
	Published time.Time
}

type rss struct {
	Channel struct {
		Items []struct {
			Title       string   `xml:"title"`
			Link        string   `xml:"link"`
			GUID        string   `xml:"guid"`
			Description string   `xml:"description"`
			Categories  []string `xml:"category"`
			PubDate     string   `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

// Fetcher downloads feeds and post pages.
type Fetcher struct {
	Client    *http.Client
	UserAgent string
}

const maxBody = 10 << 20

func (f *Fetcher) get(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxBody))
}

// Fetch downloads and parses an RSS feed. Items whose feed entry has no
// categories (Hugo's default RSS template omits them) get their tags from the
// post page's article:tag meta tags, which both Ghost and Hugo themes emit.
func (f *Fetcher) Fetch(ctx context.Context, feedURL string) ([]Item, error) {
	body, err := f.get(ctx, feedURL)
	if err != nil {
		return nil, err
	}
	items, err := Parse(body)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", feedURL, err)
	}
	return items, nil
}

// FillTags loads tags from the post page when the feed didn't include any.
func (f *Fetcher) FillTags(ctx context.Context, it *Item) error {
	if len(it.Tags) > 0 || it.Link == "" {
		return nil
	}
	body, err := f.get(ctx, it.Link)
	if err != nil {
		return err
	}
	it.Tags = PageTags(body)
	return nil
}

// Parse parses an RSS 2.0 document.
func Parse(body []byte) ([]Item, error) {
	var doc rss
	if err := xml.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	items := make([]Item, 0, len(doc.Channel.Items))
	for _, raw := range doc.Channel.Items {
		it := Item{
			GUID:    strings.TrimSpace(raw.GUID),
			Title:   cleanText(raw.Title),
			Link:    strings.TrimSpace(raw.Link),
			Excerpt: cleanText(raw.Description),
		}
		if it.GUID == "" {
			it.GUID = it.Link
		}
		if it.GUID == "" {
			continue
		}
		for _, c := range raw.Categories {
			if c = strings.TrimSpace(c); c != "" {
				it.Tags = append(it.Tags, c)
			}
		}
		if t, err := time.Parse(time.RFC1123Z, strings.TrimSpace(raw.PubDate)); err == nil {
			it.Published = t
		} else if t, err := time.Parse(time.RFC1123, strings.TrimSpace(raw.PubDate)); err == nil {
			it.Published = t
		}
		items = append(items, it)
	}
	return items, nil
}

var (
	tagRe   = regexp.MustCompile(`<[^>]*>`)
	spaceRe = regexp.MustCompile(`\s+`)
)

// cleanText strips HTML tags and decodes entities. Hugo double-encodes
// entities in descriptions (&amp;rsquo;), so after XML decoding there are
// still HTML entities left to decode.
func cleanText(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	return strings.TrimSpace(spaceRe.ReplaceAllString(s, " "))
}

// PageTags returns the content of <meta property="article:tag"> elements.
func PageTags(page []byte) []string {
	z := nethtml.NewTokenizer(strings.NewReader(string(page)))
	var tags []string
	for {
		switch z.Next() {
		case nethtml.ErrorToken:
			return tags
		case nethtml.StartTagToken, nethtml.SelfClosingTagToken:
			tok := z.Token()
			if tok.Data == "body" {
				return tags
			}
			if tok.Data != "meta" {
				continue
			}
			var prop, content string
			for _, a := range tok.Attr {
				switch a.Key {
				case "property":
					prop = a.Val
				case "content":
					content = a.Val
				}
			}
			if prop == "article:tag" && strings.TrimSpace(content) != "" {
				tags = append(tags, strings.TrimSpace(content))
			}
		}
	}
}
