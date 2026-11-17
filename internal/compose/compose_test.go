package compose

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestHashtag(t *testing.T) {
	for in, want := range map[string]string{
		"Linux":        "#Linux",
		"crossfit":     "#crossfit",
		"Self-Hosting": "#SelfHosting",
		"weekly-notes": "#WeeklyNotes",
		"Weekly Notes": "#WeeklyNotes",
		"AWS":          "#AWS",
		"c++":          "#c",
		"---":          "",
	} {
		if got := Hashtag(in); got != want {
			t.Errorf("Hashtag(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHashtagsDedupes(t *testing.T) {
	got := Hashtags([]string{"weekly-notes", "Weekly Notes", "Life"})
	if strings.Join(got, " ") != "#WeeklyNotes #Life" {
		t.Errorf("got %v", got)
	}
}

const link = "https://sathyabh.at/weekly-notes-46-2026/"

func TestPostFits(t *testing.T) {
	got := Post("Weekly Notes 46/2026", "Week 46 summary.", link, []string{"Sydney", "Weekly Notes"}, 300)
	want := "Weekly Notes 46/2026\n\nWeek 46 summary.\n\n" + link + "\n\n#Sydney #WeeklyNotes"
	if got != want {
		t.Errorf("got %q", got)
	}
}

func TestPostShortensExcerpt(t *testing.T) {
	excerpt := strings.Repeat("word ", 100)
	tags := []string{"Sydney", "Life", "Weekly Notes", "Gaming"}
	got := Post("Weekly Notes 46/2026", excerpt, link, tags, 300)
	if n := utf8.RuneCountInString(got); n > 300 {
		t.Fatalf("length %d > 300", n)
	}
	if !strings.Contains(got, "word…") || !strings.HasSuffix(got, link+"\n\n#Sydney #Life #WeeklyNotes #Gaming") {
		t.Errorf("got %q", got)
	}
}

func TestPostDropsTagsWhenNoRoom(t *testing.T) {
	title := strings.Repeat("t", 230)
	got := Post(title, "an excerpt", link, []string{"one", "two", "three", "four", "five"}, 300)
	if n := utf8.RuneCountInString(got); n > 300 {
		t.Fatalf("length %d > 300: %q", n, got)
	}
	if strings.Contains(got, "excerpt") || !strings.Contains(got, link) {
		t.Errorf("got %q", got)
	}
}

func TestPostKeepsLink(t *testing.T) {
	got := Post(strings.Repeat("t", 400), "", link, nil, 300)
	if utf8.RuneCountInString(got) > 300 || !strings.HasSuffix(got, link) {
		t.Errorf("got %q", got)
	}
}
