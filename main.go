package main

import (
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"strings"
	"unicode"

	"database/sql"
	"encoding/csv"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type config struct {
	dbPath     string
	outputPath string
	useSTP     bool
}

type profile struct {
	name     string
	parentID int64
	personal bool
}

type tabGroup struct {
	id    int64
	title string
}

type bookmark struct {
	title string
	url   string
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "tabby: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg, err := parseArgs(args)
	if err != nil {
		return err
	}

	source, err := safariTabsPath(cfg)
	if err != nil {
		return err
	}

	temporary, cleanup, err := copyDatabase(source)
	if err != nil {
		return err
	}
	defer cleanup()

	outputBase, err := outputBasePath(cfg.outputPath)
	if err != nil {
		return err
	}

	db, err := sql.Open("sqlite3", temporary)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	return exportProfiles(db, outputBase)
}

func parseArgs(args []string) (config, error) {
	var cfg config

	flags := flag.NewFlagSet("tabby", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&cfg.dbPath, "db", "", "path to SafariTabs.db")
	flags.StringVar(&cfg.outputPath, "out", "", "export directory")
	flags.BoolVar(&cfg.useSTP, "stp", false, "use Safari Technology Preview")

	if err := flags.Parse(args); err != nil {
		return config{}, err
	}

	return cfg, nil
}

func safariTabsPath(cfg config) (string, error) {
	if cfg.dbPath != "" {
		return expandPath(cfg.dbPath)
	}

	app := "Safari"
	if cfg.useSTP {
		app = "SafariTechnologyPreview"
	}

	return expandPath(filepath.Join(
		"~",
		"Library",
		"Containers",
		"com.apple."+app,
		"Data",
		"Library",
		app,
		"SafariTabs.db",
	))
}

func outputBasePath(path string) (string, error) {
	if path == "" {
		path = filepath.Join("~", "Desktop")
	}

	expanded, err := expandPath(path)
	if err != nil {
		return "", err
	}

	base := filepath.Join(expanded, "tabgroups")
	if err = os.MkdirAll(base, 0o755); err != nil {
		return "", err
	}

	return base, nil
}

func expandPath(path string) (string, error) {
	if path == "" {
		return "", errors.New("path cannot be empty")
	}

	if path == "~" || strings.HasPrefix(path, "~/") {
		home := os.Getenv("HOME")
		if home == "" {
			var err error
			home, err = os.UserHomeDir()
			if err != nil {
				return "", err
			}
		}

		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}

	return filepath.Abs(path)
}

func copyDatabase(source string) (string, func(), error) {
	in, err := os.Open(source)
	if err != nil {
		return "", nil, err
	}
	defer func() { _ = in.Close() }()

	out, err := os.CreateTemp("", "SafariTabs-*.db")
	if err != nil {
		return "", nil, err
	}

	cleanup := func() {
		_ = os.Remove(out.Name())
	}

	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		cleanup()
		return "", nil, err
	}

	if err = out.Close(); err != nil {
		cleanup()
		return "", nil, err
	}

	return out.Name(), cleanup, nil
}

func exportProfiles(db *sql.DB, base string) error {
	profiles, err := loadProfiles(db)
	if err != nil {
		return err
	}

	for _, current := range profiles {
		var groups []tabGroup
		groups, err = loadTabGroups(db, current)
		if err != nil {
			return err
		}

		directory := filepath.Join(base, current.name)
		if err = os.MkdirAll(directory, 0o755); err != nil {
			return err
		}

		if err = writeCSV(db, filepath.Join(directory, "bookmarks.csv"), groups); err != nil {
			return err
		}

		if err = writeHTML(db, filepath.Join(directory, "bookmarks.html"), current.name, groups); err != nil {
			return err
		}
	}

	return nil
}

func loadProfiles(db *sql.DB) ([]profile, error) {
	profiles := []profile{{name: "personal", personal: true}}

	rows, err := db.Query("SELECT id, COALESCE(title, '') FROM bookmarks WHERE subtype = 2 AND COALESCE(title, '') != '' ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var id int64
		var title string
		if err = rows.Scan(&id, &title); err != nil {
			return nil, err
		}

		profiles = append(profiles, profile{name: strings.ToLower(title), parentID: id})
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return profiles, nil
}

func loadTabGroups(db *sql.DB, current profile) ([]tabGroup, error) {
	if current.personal {
		return queryTabGroups(db, "SELECT id, COALESCE(title, '') FROM bookmarks WHERE type = 1 AND parent = 0 AND subtype = 0 AND num_children > 0 AND hidden = 0 ORDER BY id DESC")
	}

	return queryTabGroups(db, "SELECT id, COALESCE(title, '') FROM bookmarks WHERE parent = ? AND subtype = 0 AND num_children > 0 ORDER BY id DESC", current.parentID)
}

func queryTabGroups(db *sql.DB, query string, args ...any) ([]tabGroup, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var groups []tabGroup
	for rows.Next() {
		var group tabGroup
		if err = rows.Scan(&group.id, &group.title); err != nil {
			return nil, err
		}

		groups = append(groups, group)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return groups, nil
}

func loadBookmarks(db *sql.DB, groupID int64) ([]bookmark, error) {
	rows, err := db.Query(`
		SELECT COALESCE(title, ''), COALESCE(url, '')
		FROM bookmarks
		WHERE parent = ?
			AND title NOT IN ('TopScopedBookmarkList', 'Untitled', 'Start Page')
		ORDER BY order_index ASC
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var bookmarks []bookmark
	for rows.Next() {
		var item bookmark
		if err = rows.Scan(&item.title, &item.url); err != nil {
			return nil, err
		}

		bookmarks = append(bookmarks, item)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return bookmarks, nil
}

func writeCSV(db *sql.DB, path string, groups []tabGroup) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	writer := csv.NewWriter(file)
	if err = writer.Write([]string{"Tab Group", "Bookmark", "URL"}); err != nil {
		return err
	}

	for _, group := range groups {
		var bookmarks []bookmark
		bookmarks, err = loadBookmarks(db, group.id)
		if err != nil {
			return err
		}

		for _, item := range bookmarks {
			if err = writer.Write([]string{group.title, item.title, item.url}); err != nil {
				return err
			}
		}
	}

	writer.Flush()
	return writer.Error()
}

func writeHTML(db *sql.DB, path string, profileName string, groups []tabGroup) error {
	var page strings.Builder

	title := capitalize(profileName)
	_, _ = fmt.Fprintln(&page, "<!DOCTYPE NETSCAPE-Bookmark-file-1>")
	_, _ = fmt.Fprintln(&page, `<META HTTP-EQUIV="Content-Type" CONTENT="text/html; charset=UTF-8">`)
	_, _ = fmt.Fprintf(&page, "<TITLE>%s Bookmarks</TITLE>\n", html.EscapeString(title))
	_, _ = fmt.Fprintf(&page, "<H1>%s Bookmarks</H1>\n", html.EscapeString(title))

	for _, group := range groups {
		bookmarks, err := loadBookmarks(db, group.id)
		if err != nil {
			return err
		}

		_, _ = fmt.Fprintf(&page, "<dt>\n  <h3>%s</h3>\n</dt>\n", html.EscapeString(group.title))
		_, _ = fmt.Fprintln(&page, "<dd>")
		_, _ = fmt.Fprintln(&page, "  <dl>")
		for _, item := range bookmarks {
			_, _ = fmt.Fprintf(&page, "    <dt><a href=\"%s\">%s</a></dt>\n", html.EscapeString(item.url), html.EscapeString(item.title))
		}
		_, _ = fmt.Fprintln(&page, "  </dl>")
		_, _ = fmt.Fprintln(&page, "</dd>")
	}

	return os.WriteFile(path, []byte(page.String()), 0o644)
}

func capitalize(value string) string {
	if value == "" {
		return ""
	}

	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
