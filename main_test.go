package main

import (
	"os"
	"slices"
	"strings"
	"testing"

	"database/sql"
	"encoding/csv"
	"path/filepath"
)

type testBookmark struct {
	title string
	url   string
}

type testGroup struct {
	name      string
	bookmarks []testBookmark
}

type testProfile struct {
	name   string
	groups []testGroup
}

var defaultPersonalGroups = []testGroup{
	{
		name: "Reading",
		bookmarks: []testBookmark{
			{title: "Hacker News", url: "https://news.ycombinator.com"},
			{title: "Anthropic", url: "https://www.anthropic.com"},
			{title: "Untitled", url: "https://example.com/untitled"},
			{title: "TopScopedBookmarkList", url: "https://example.com/reserved"},
		},
	},
	{
		name: "News",
		bookmarks: []testBookmark{
			{title: "NYT", url: "https://www.nytimes.com"},
			{title: "Start Page", url: "https://example.com/start"},
		},
	},
}

func TestExportsPersonalTabGroups(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "SafariTabs.db")
	outputDir := t.TempDir()

	buildSafariDB(t, dbPath, defaultPersonalGroups, nil)

	if err := run([]string{"--db", dbPath, "--out", outputDir}); err != nil {
		t.Fatalf("run tabby: %v", err)
	}

	csvPath := filepath.Join(outputDir, "tabgroups", "personal", "bookmarks.csv")
	htmlPath := filepath.Join(outputDir, "tabgroups", "personal", "bookmarks.html")

	rows := readCSV(t, csvPath)
	if rows[0][0] != "Tab Group" || rows[0][1] != "Bookmark" || rows[0][2] != "URL" {
		t.Fatalf("unexpected header row: %#v", rows[0])
	}

	titles := column(rows[1:], 1)
	for _, want := range []string{"Anthropic", "Hacker News", "NYT"} {
		if !slices.Contains(titles, want) {
			t.Fatalf("expected CSV titles to include %q, got %#v", want, titles)
		}
	}
	for _, reserved := range []string{"TopScopedBookmarkList", "Untitled", "Start Page"} {
		if slices.Contains(titles, reserved) {
			t.Fatalf("expected CSV titles to omit %q, got %#v", reserved, titles)
		}
	}

	if got := bookmarksInGroup(rows[1:], "Reading"); !slices.Equal(got, []string{"Hacker News", "Anthropic"}) {
		t.Fatalf("unexpected Reading bookmark order: %#v", got)
	}

	if got := unique(column(rows[1:], 0)); !slices.Equal(got, []string{"News", "Reading"}) {
		t.Fatalf("unexpected tab group order: %#v", got)
	}

	if got := urlFor(rows[1:], "Anthropic"); got != "https://www.anthropic.com" {
		t.Fatalf("unexpected Anthropic URL: %q", got)
	}

	html := readFile(t, htmlPath)
	mustStartWith(t, html, "<!DOCTYPE NETSCAPE-Bookmark-file-1>")
	mustContain(t, html, "<TITLE>Personal Bookmarks</TITLE>")
	mustContain(t, html, "<H1>Personal Bookmarks</H1>")
	mustContain(t, html, "<h3>Reading</h3>")
	mustContain(t, html, "<h3>News</h3>")
	mustContain(t, html, `<a href="https://www.anthropic.com">Anthropic</a>`)
	mustContain(t, html, `<a href="https://news.ycombinator.com">Hacker News</a>`)
	mustNotContain(t, html, "TopScopedBookmarkList")
	mustNotContain(t, html, "Untitled")
	mustNotContain(t, html, "Start Page")
}

func TestExportsMultipleProfiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "SafariTabs.db")
	outputDir := t.TempDir()

	profiles := []testProfile{
		{
			name: "Work",
			groups: []testGroup{
				{
					name: "Media",
					bookmarks: []testBookmark{
						{title: "YouTube", url: "https://www.youtube.com/"},
						{title: "Twitch", url: "https://www.twitch.tv/"},
					},
				},
			},
		},
		{
			name: "Space",
			groups: []testGroup{
				{
					name: "Launches",
					bookmarks: []testBookmark{
						{title: "SpaceX", url: "https://www.spacex.com/"},
						{title: "Home | Blue Origin", url: "https://www.blueorigin.com/"},
					},
				},
			},
		},
	}
	buildSafariDB(t, dbPath, []testGroup{
		{
			name: "Reference",
			bookmarks: []testBookmark{
				{title: "Apple", url: "https://www.apple.com/"},
			},
		},
		{
			name: "Technology",
			bookmarks: []testBookmark{
				{title: "Go", url: "https://go.dev/"},
			},
		},
	}, profiles)

	if err := run([]string{"--db", dbPath, "--out", outputDir}); err != nil {
		t.Fatalf("run tabby: %v", err)
	}

	for _, dir := range []string{"personal", "work", "space"} {
		if _, err := os.Stat(filepath.Join(outputDir, "tabgroups", dir)); err != nil {
			t.Fatalf("expected %s profile directory: %v", dir, err)
		}
	}

	personalGroups := unique(column(readCSV(t, filepath.Join(outputDir, "tabgroups", "personal", "bookmarks.csv"))[1:], 0))
	if !slices.Contains(personalGroups, "Reference") || !slices.Contains(personalGroups, "Technology") {
		t.Fatalf("unexpected personal groups: %#v", personalGroups)
	}

	workTitles := column(readCSV(t, filepath.Join(outputDir, "tabgroups", "work", "bookmarks.csv"))[1:], 1)
	if !slices.Contains(workTitles, "YouTube") || !slices.Contains(workTitles, "Twitch") {
		t.Fatalf("unexpected work titles: %#v", workTitles)
	}

	spaceTitles := column(readCSV(t, filepath.Join(outputDir, "tabgroups", "space", "bookmarks.csv"))[1:], 1)
	if !slices.Contains(spaceTitles, "SpaceX") || !slices.Contains(spaceTitles, "Home | Blue Origin") {
		t.Fatalf("unexpected space titles: %#v", spaceTitles)
	}

	workHTML := readFile(t, filepath.Join(outputDir, "tabgroups", "work", "bookmarks.html"))
	mustContain(t, workHTML, "<H1>Work Bookmarks</H1>")
	mustContain(t, workHTML, `<a href="https://www.youtube.com/">YouTube</a>`)

	spaceHTML := readFile(t, filepath.Join(outputDir, "tabgroups", "space", "bookmarks.html"))
	mustContain(t, spaceHTML, "<H1>Space Bookmarks</H1>")
	mustContain(t, spaceHTML, `<a href="https://www.spacex.com/">SpaceX</a>`)
}

func TestExportsExistingFixtureDatabase(t *testing.T) {
	outputDir := t.TempDir()

	if err := run([]string{"--db", filepath.Join("testdata", "SafariTabs.db"), "--out", outputDir}); err != nil {
		t.Fatalf("run tabby: %v", err)
	}

	workTitles := column(readCSV(t, filepath.Join(outputDir, "tabgroups", "work", "bookmarks.csv"))[1:], 1)
	if !slices.Contains(workTitles, "YouTube") || !slices.Contains(workTitles, "Twitch") {
		t.Fatalf("unexpected fixture work titles: %#v", workTitles)
	}

	spaceHTML := readFile(t, filepath.Join(outputDir, "tabgroups", "space", "bookmarks.html"))
	mustContain(t, spaceHTML, "<H1>Space Bookmarks</H1>")
	mustContain(t, spaceHTML, `<a href="https://www.spacex.com/">SpaceX</a>`)
}

func TestExpandsOutputPathRelativeToHome(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "SafariTabs.db")
	fakeHome := t.TempDir()
	buildSafariDB(t, dbPath, defaultPersonalGroups, nil)
	t.Setenv("HOME", fakeHome)

	if err := run([]string{"--db", dbPath, "--out", "~/exports"}); err != nil {
		t.Fatalf("run tabby: %v", err)
	}

	if _, err := os.Stat(filepath.Join(fakeHome, "exports", "tabgroups", "personal")); err != nil {
		t.Fatalf("expected expanded output directory: %v", err)
	}
}

func TestUsesSafariTechnologyPreviewPath(t *testing.T) {
	fakeHome := t.TempDir()
	stpDir := filepath.Join(
		fakeHome,
		"Library",
		"Containers",
		"com.apple.SafariTechnologyPreview",
		"Data",
		"Library",
		"SafariTechnologyPreview",
	)
	dbPath := filepath.Join(stpDir, "SafariTabs.db")
	outputDir := t.TempDir()
	buildSafariDB(t, dbPath, defaultPersonalGroups, nil)
	t.Setenv("HOME", fakeHome)

	if err := run([]string{"-stp", "--out", outputDir}); err != nil {
		t.Fatalf("run tabby: %v", err)
	}

	rows := readCSV(t, filepath.Join(outputDir, "tabgroups", "personal", "bookmarks.csv"))
	if !slices.Contains(column(rows[1:], 1), "Anthropic") {
		t.Fatalf("expected STP export to include Anthropic, got %#v", rows)
	}
}

func buildSafariDB(t *testing.T, path string, personal []testGroup, profiles []testProfile) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create db dir: %v", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove existing db: %v", err)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
		CREATE TABLE bookmarks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT,
			url TEXT,
			type INTEGER,
			parent INTEGER,
			subtype INTEGER,
			num_children INTEGER,
			hidden INTEGER,
			order_index INTEGER
		);
	`)
	if err != nil {
		t.Fatalf("create bookmarks table: %v", err)
	}

	seedGroups(t, db, 0, personal)
	for _, p := range profiles {
		profileID := insertProfile(t, db, p.name)
		seedGroups(t, db, profileID, p.groups)
	}
}

func seedGroups(t *testing.T, db *sql.DB, parentID int64, groups []testGroup) {
	t.Helper()

	for _, group := range groups {
		groupID := insertGroup(t, db, parentID, group.name)
		for index, item := range group.bookmarks {
			insertBookmark(t, db, groupID, item.title, item.url, index+1)
		}
	}
}

func insertGroup(t *testing.T, db *sql.DB, parentID int64, title string) int64 {
	t.Helper()

	result, err := db.Exec(
		"INSERT INTO bookmarks (title, type, parent, subtype, num_children, hidden) VALUES (?, ?, ?, ?, ?, ?)",
		title, 1, parentID, 0, 1, 0,
	)
	if err != nil {
		t.Fatalf("insert group: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read group id: %v", err)
	}
	return id
}

func insertBookmark(t *testing.T, db *sql.DB, parentID int64, title string, url string, orderIndex int) {
	t.Helper()

	if _, err := db.Exec(
		"INSERT INTO bookmarks (title, url, type, parent, subtype, hidden, order_index) VALUES (?, ?, ?, ?, ?, ?, ?)",
		title, url, 0, parentID, 0, 0, orderIndex,
	); err != nil {
		t.Fatalf("insert bookmark: %v", err)
	}
}

func insertProfile(t *testing.T, db *sql.DB, title string) int64 {
	t.Helper()

	result, err := db.Exec(
		"INSERT INTO bookmarks (title, type, parent, subtype, num_children, hidden) VALUES (?, ?, ?, ?, ?, ?)",
		title, 1, 0, 2, 1, 0,
	)
	if err != nil {
		t.Fatalf("insert profile: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read profile id: %v", err)
	}
	return id
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open CSV: %v", err)
	}
	defer func() { _ = file.Close() }()

	rows, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatalf("read CSV: %v", err)
	}
	return rows
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(content)
}

func column(rows [][]string, index int) []string {
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		values = append(values, row[index])
	}
	return values
}

func bookmarksInGroup(rows [][]string, group string) []string {
	var values []string
	for _, row := range rows {
		if row[0] == group {
			values = append(values, row[1])
		}
	}
	return values
}

func unique(values []string) []string {
	var result []string
	for _, value := range values {
		if !slices.Contains(result, value) {
			result = append(result, value)
		}
	}
	return result
}

func urlFor(rows [][]string, title string) string {
	for _, row := range rows {
		if row[1] == title {
			return row[2]
		}
	}
	return ""
}

func mustContain(t *testing.T, text string, fragment string) {
	t.Helper()

	if !strings.Contains(text, fragment) {
		t.Fatalf("expected %q to contain %q", text, fragment)
	}
}

func mustNotContain(t *testing.T, text string, fragment string) {
	t.Helper()

	if strings.Contains(text, fragment) {
		t.Fatalf("expected %q not to contain %q", text, fragment)
	}
}

func mustStartWith(t *testing.T, text string, prefix string) {
	t.Helper()

	if !strings.HasPrefix(text, prefix) {
		t.Fatalf("expected %q to start with %q", text, prefix)
	}
}
