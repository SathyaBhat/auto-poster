// Package compose builds the social post text for a blog post.
package compose

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Hashtag turns a tag name into a hashtag. Words split by spaces, hyphens or
// other punctuation are joined in CamelCase ("Self-Hosting" -> #SelfHosting,
// "weekly-notes" -> #WeeklyNotes), because Mastodon and Bluesky both end a
// hashtag at a hyphen. Returns "" if nothing usable is left.
func Hashtag(tag string) string {
	words := strings.FieldsFunc(tag, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('#')
	for _, w := range words {
		r, size := utf8.DecodeRuneInString(w)
		if len(words) > 1 {
			b.WriteRune(unicode.ToUpper(r))
		} else {
			b.WriteRune(r)
		}
		b.WriteString(w[size:])
	}
	return b.String()
}

// Hashtags converts tags to unique hashtags, keeping order.
func Hashtags(tags []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range tags {
		h := Hashtag(t)
		key := strings.ToLower(h)
		if h == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, h)
	}
	return out
}

// Post builds the post text and fits it within limit characters (runes).
// It shortens the excerpt first, then drops hashtags from the end, then
// shortens the title. The link is never shortened.
func Post(title, excerpt, link string, tags []string, limit int) string {
	hashtags := Hashtags(tags)

	text := render(title, excerpt, link, hashtags)
	if length(text) <= limit {
		return text
	}

	// Shorten the excerpt to whatever room is left.
	without := render(title, "", link, hashtags)
	room := limit - length(without) - 2 // "\n\n" separating the excerpt
	if excerpt != "" && room >= minExcerpt {
		return render(title, truncate(excerpt, room), link, hashtags)
	}

	// No useful room for an excerpt: drop it, then hashtags from the end.
	for len(hashtags) > 0 && length(render(title, "", link, hashtags)) > limit {
		hashtags = hashtags[:len(hashtags)-1]
	}
	text = render(title, "", link, hashtags)
	if length(text) <= limit {
		return text
	}

	room = limit - length(render("", "", link, nil)) - 2
	if room < 1 {
		return link
	}
	return render(truncate(title, room), "", link, nil)
}

// minExcerpt is the shortest excerpt worth keeping; below this, hashtags win.
const minExcerpt = 40

func render(title, excerpt, link string, hashtags []string) string {
	var parts []string
	for _, p := range []string{title, excerpt, link, strings.Join(hashtags, " ")} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "\n\n")
}

// length counts runes. Bluesky counts graphemes and Mastodon counts URLs as 23
// characters, so a rune count is never lower than what either platform counts.
func length(s string) int { return utf8.RuneCountInString(s) }

// truncate cuts s to at most n runes at a word boundary and adds an ellipsis.
func truncate(s string, n int) string {
	if length(s) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	r := []rune(s)[:n-1]
	cut := string(r)
	if i := strings.LastIndexFunc(cut, unicode.IsSpace); i > len(cut)/2 {
		cut = cut[:i]
	}
	return strings.TrimRightFunc(cut, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r)
	}) + "…"
}
