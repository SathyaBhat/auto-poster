package feed

import (
	"os"
	"slices"
	"testing"
)

func load(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseGhost(t *testing.T) {
	items, err := Parse(load(t, "ghost.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items", len(items))
	}
	it := items[0]
	if it.Title != "Weekly Notes 46/2026" || it.GUID != "6af7a31553e570000139683e" ||
		it.Link != "https://sathyabh.at/weekly-notes-46-2026/" {
		t.Errorf("unexpected item: %+v", it)
	}
	if it.Excerpt != "Week 46 summary - meeting an old friend, and trying a new sport." {
		t.Errorf("excerpt = %q", it.Excerpt)
	}
	if want := []string{"Sydney", "Life", "Weekly Notes", "Gaming"}; !slices.Equal(it.Tags, want) {
		t.Errorf("tags = %v", it.Tags)
	}
	if it.Published.IsZero() {
		t.Error("published not parsed")
	}
}

func TestParseHugoDecodesEntities(t *testing.T) {
	items, err := Parse(load(t, "hugo.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := items[1].Title; got != "It's Always DNS Part N - Incorrect LAN Interface Edition" {
		t.Errorf("title = %q", got)
	}
	if got := items[1].Excerpt; got != "Here’s how a LAN cable swap broke my Internet connection." {
		t.Errorf("excerpt = %q", got)
	}
	if len(items[0].Tags) != 0 || items[0].Published.IsZero() {
		t.Errorf("unexpected item: %+v", items[0])
	}
}

func TestPageTags(t *testing.T) {
	got := PageTags(load(t, "hugo-post.html"))
	want := []string{"Linux", "Arch", "Networking", "E1000e", "Networkmanager", "Self-Hosting"}
	if !slices.Equal(got, want) {
		t.Errorf("tags = %v", got)
	}
}
