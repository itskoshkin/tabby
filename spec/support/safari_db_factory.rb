require "fileutils"
require "sqlite3"

module SafariDbFactory
  module_function

  DEFAULT_PERSONAL = {
    "Reading" => [
      { title: "Hacker News",            url: "https://news.ycombinator.com" },
      { title: "Anthropic",              url: "https://www.anthropic.com" },
      { title: "Untitled",               url: "https://example.com/untitled" },
      { title: "TopScopedBookmarkList",  url: "https://example.com/reserved" },
    ],
    "News" => [
      { title: "NYT",        url: "https://www.nytimes.com" },
      { title: "Start Page", url: "https://example.com/start" },
    ],
  }.freeze

  def build(path, personal: DEFAULT_PERSONAL, profiles: {})
    FileUtils.mkdir_p(File.dirname(path))
    File.delete(path) if File.exist?(path)

    db = SQLite3::Database.new(path)
    db.execute_batch(<<~SQL)
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
    SQL

    seed_groups(db, parent: 0, groups: personal)

    profiles.each do |profile_name, profile_groups|
      profile_id = insert_profile(db, title: profile_name)
      seed_groups(db, parent: profile_id, groups: profile_groups)
    end
  ensure
    db&.close
  end

  def seed_groups(db, parent:, groups:)
    groups.each do |group_name, bookmarks|
      group_id = insert_group(db, parent: parent, title: group_name)
      bookmarks.each_with_index do |bookmark, i|
        insert_bookmark(db,
          parent: group_id,
          title: bookmark[:title],
          url: bookmark[:url],
          order_index: i + 1)
      end
    end
  end

  def insert_group(db, parent:, title:)
    db.execute(
      "INSERT INTO bookmarks (title, type, parent, subtype, num_children, hidden) VALUES (?, ?, ?, ?, ?, ?)",
      [title, 1, parent, 0, 1, 0]
    )
    db.last_insert_row_id
  end

  def insert_bookmark(db, parent:, title:, url:, order_index:)
    db.execute(
      "INSERT INTO bookmarks (title, url, type, parent, subtype, hidden, order_index) VALUES (?, ?, ?, ?, ?, ?, ?)",
      [title, url, 0, parent, 0, 0, order_index]
    )
  end

  def insert_profile(db, title:)
    db.execute(
      "INSERT INTO bookmarks (title, type, parent, subtype, num_children, hidden) VALUES (?, ?, ?, ?, ?, ?)",
      [title, 1, 0, 2, 1, 0]
    )
    db.last_insert_row_id
  end
end
