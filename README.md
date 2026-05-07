# Tabby

Safari tab groups are an awesome way to manage your tabs. But, unfortunately, at this moment Safari does not provide any way to export them.

Tabby can:
- Export tab groups as HTML or CSV
- Export tab groups from any selected other profiles
- Export tab groups in current order (including tab order within groups)

## Run

0. [Install Go 1.26 or newer](https://go.dev/doc/install)
1. Clone repository
    ```bash
    git clone https://github.com/itskoshkin/tabby.git && cd tabby
    ```
2. Run
    ```bash
    go run .
    ```
3. Build a reusable binary
    ```
    go build -o tabby
    ./tabby
    ```

## Args
- Customize the export location
  ```
  go run . --out ~/Library/Backup
  ```
- Export a different `SafariTabs.db` file
  ```
  go run . --db path/to/SafariTabs.db
  ```
- Export tabs from the Safari Technology Preview app
  ```
  go run . -stp
  ```
- Export tabs regularly with cron
  ```
  crontab -e
  ```

## Cat

![Sebbie](https://github.com/itskoshkin/tabby/blob/origin/sebbie.jpg)
