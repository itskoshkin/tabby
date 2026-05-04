require "spec_helper"
require "csv"
require "fileutils"
require "tmpdir"
require_relative "support/safari_db_factory"

RSpec.describe "Tabby" do
  let(:script)     { File.expand_path("../tabby.rb", __dir__) }
  let(:db_dir)     { Dir.mktmpdir("tabby-db") }
  let(:db_path)    { File.join(db_dir, "SafariTabs.db") }
  let(:output_dir) { Dir.mktmpdir("tabby-out") }

  after do
    FileUtils.remove_entry(db_dir)     if File.exist?(db_dir)
    FileUtils.remove_entry(output_dir) if File.exist?(output_dir)
  end

  def run_tabby(*args, env: {})
    ok = system(env, "ruby", script, *args, out: File::NULL, err: File::NULL)
    raise "tabby.rb exited non-zero" unless ok
  end

  describe "can export tab groups" do
    let(:csv_path)  { File.join(output_dir, "tabgroups", "personal", "bookmarks.csv") }
    let(:html_path) { File.join(output_dir, "tabgroups", "personal", "bookmarks.html") }

    before do
      SafariDbFactory.build(db_path)
      run_tabby("--db", db_path, "--out", output_dir)
    end

    describe "to CSV" do
      it "creates the CSV file" do
        expect(File).to exist(csv_path)
      end

      it "includes a header row" do
        expect(CSV.read(csv_path).first).to eq([ "Tab Group", "Bookmark", "URL" ])
      end

      it "includes one row per non-reserved bookmark" do
        titles = CSV.read(csv_path)[1..].map { |row| row[1] }
        expect(titles).to include("Anthropic", "Hacker News", "NYT")
      end

      it "filters out 'TopScopedBookmarkList', 'Untitled', and 'Start Page'" do
        titles = CSV.read(csv_path)[1..].map { |row| row[1] }
        expect(titles).not_to include("TopScopedBookmarkList", "Untitled", "Start Page")
      end

      it "preserves bookmark order from order_index" do
        rows = CSV.read(csv_path)[1..]
        reading = rows.select { |row| row[0] == "Reading" }.map { |row| row[1] }
        expect(reading).to eq([ "Hacker News", "Anthropic" ])
      end

      it "orders tab groups newest-first by id" do
        groups = CSV.read(csv_path)[1..].map { |row| row[0] }.uniq
        expect(groups).to eq([ "News", "Reading" ])
      end

      it "writes the bookmark URL in the URL column" do
        rows = CSV.read(csv_path)[1..]
        anthropic = rows.find { |row| row[1] == "Anthropic" }
        expect(anthropic[2]).to eq("https://www.anthropic.com")
      end
    end

    describe "to HTML" do
      it "creates the HTML file" do
        expect(File).to exist(html_path)
      end

      it "starts with the Netscape bookmark doctype" do
        expect(File.read(html_path)).to start_with("<!DOCTYPE NETSCAPE-Bookmark-file-1>")
      end

      it "includes a TITLE and H1 with the capitalized profile name" do
        html = File.read(html_path)
        expect(html).to include("<TITLE>Personal Bookmarks</TITLE>")
        expect(html).to include("<H1>Personal Bookmarks</H1>")
      end

      it "renders each tab group as a <dt><h3>...</h3></dt>" do
        html = File.read(html_path)
        expect(html).to match(%r{<dt>\s*<h3>Reading</h3>\s*</dt>}m)
        expect(html).to match(%r{<dt>\s*<h3>News</h3>\s*</dt>}m)
      end

      it "renders each bookmark as an anchor with title and href" do
        html = File.read(html_path)
        expect(html).to include('<a href="https://www.anthropic.com">Anthropic</a>')
        expect(html).to include('<a href="https://news.ycombinator.com">Hacker News</a>')
      end

      it "omits reserved-title bookmarks" do
        html = File.read(html_path)
        expect(html).not_to include("TopScopedBookmarkList")
        expect(html).not_to include("Untitled")
        expect(html).not_to include("Start Page")
      end
    end
  end

  describe "can export one or more profiles" do
    describe "single profile" do
      before do
        SafariDbFactory.build(db_path)
        run_tabby("--db", db_path, "--out", output_dir)
      end

      it "creates the personal profile directory" do
        expect(Dir).to exist(File.join(output_dir, "tabgroups", "personal"))
      end

      it "does not create any other profile directories" do
        profile_dirs = Dir.children(File.join(output_dir, "tabgroups"))
        expect(profile_dirs).to eq([ "personal" ])
      end

      it "writes the personal profile's CSV" do
        expect(File).to exist(File.join(output_dir, "tabgroups", "personal", "bookmarks.csv"))
      end

      it "writes the personal profile's HTML with the capitalized profile name" do
        html = File.read(File.join(output_dir, "tabgroups", "personal", "bookmarks.html"))
        expect(html).to include("<H1>Personal Bookmarks</H1>")
      end
    end

    describe "multiple profiles" do
      let(:fixture_db) { File.expand_path("fixtures/SafariTabs.db", __dir__) }

      before do
        run_tabby("--db", fixture_db, "--out", output_dir)
      end

      it "exports the personal profile" do
        expect(Dir).to exist(File.join(output_dir, "tabgroups", "personal"))
      end

      it "exports every additional profile (lower-cased)" do
        expect(Dir).to exist(File.join(output_dir, "tabgroups", "work"))
        expect(Dir).to exist(File.join(output_dir, "tabgroups", "space"))
      end

      it "writes the personal profile's tab groups" do
        groups = CSV.read(File.join(output_dir, "tabgroups", "personal", "bookmarks.csv"))[1..].map { |row| row[0] }.uniq
        expect(groups).to include("Reference", "Technology")
      end

      it "writes each additional profile's CSV with its bookmarks" do
        work_titles  = CSV.read(File.join(output_dir, "tabgroups", "work",  "bookmarks.csv"))[1..].map { |row| row[1] }
        space_titles = CSV.read(File.join(output_dir, "tabgroups", "space", "bookmarks.csv"))[1..].map { |row| row[1] }
        expect(work_titles).to include("YouTube", "Twitch")
        expect(space_titles).to include("SpaceX", "Home | Blue Origin")
      end

      it "writes each additional profile's HTML with the capitalized profile name" do
        work_html  = File.read(File.join(output_dir, "tabgroups", "work",  "bookmarks.html"))
        space_html = File.read(File.join(output_dir, "tabgroups", "space", "bookmarks.html"))
        expect(work_html).to include("<H1>Work Bookmarks</H1>")
        expect(work_html).to include('<a href="https://www.youtube.com/">YouTube</a>')
        expect(space_html).to include("<H1>Space Bookmarks</H1>")
        expect(space_html).to include('<a href="https://www.spacex.com/">SpaceX</a>')
      end
    end
  end

  describe "has command-line flags" do
    describe "--db" do
      it "reads tab groups from the given database path" do
        SafariDbFactory.build(db_path)
        run_tabby("--db", db_path, "--out", output_dir)
        rows = CSV.read(File.join(output_dir, "tabgroups", "personal", "bookmarks.csv"))
        expect(rows.length).to be > 1
      end
    end

    describe "--out" do
      it "writes everything under <out>/tabgroups/" do
        SafariDbFactory.build(db_path)
        run_tabby("--db", db_path, "--out", output_dir)
        expect(Dir).to exist(File.join(output_dir, "tabgroups", "personal"))
      end

      it "expands a path containing '~' relative to HOME" do
        fake_home = Dir.mktmpdir("tabby-home")
        SafariDbFactory.build(db_path)
        run_tabby("--db", db_path, "--out", "~/exports", env: { "HOME" => fake_home })
        expect(Dir).to exist(File.join(fake_home, "exports", "tabgroups", "personal"))
      ensure
        FileUtils.remove_entry(fake_home) if fake_home && File.exist?(fake_home)
      end
    end

    describe "-stp" do
      it "reads from the SafariTechnologyPreview Library when no --db is given" do
        fake_home = Dir.mktmpdir("tabby-home")
        stp_dir = File.join(fake_home,
          "Library", "Containers", "com.apple.SafariTechnologyPreview",
          "Data", "Library", "SafariTechnologyPreview")
        FileUtils.mkdir_p(stp_dir)
        SafariDbFactory.build(File.join(stp_dir, "SafariTabs.db"))

        run_tabby("-stp", "--out", output_dir, env: { "HOME" => fake_home })

        rows = CSV.read(File.join(output_dir, "tabgroups", "personal", "bookmarks.csv"))
        expect(rows[1..].map { |row| row[1] }).to include("Anthropic")
      ensure
        FileUtils.remove_entry(fake_home) if fake_home && File.exist?(fake_home)
      end
    end
  end
end
