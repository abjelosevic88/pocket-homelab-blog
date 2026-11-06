# Backup

The words live in one SQLite file, `DATA_DIR/pocket-blog.db` (WAL mode); the pictures are files in `DATA_DIR/media/`. Take a consistent copy of the database with `pocket-blog backup` (writes `DATA_DIR/backups/pocket-blog-<time>.db` with `VACUUM INTO`) or Settings → Backup & data → Download database (admin), and back up `media/` as it is. Back up that copy, not the live file. `DATA_DIR/core.token*` is the Pocket Core registration.

Restore: stop the app, replace `pocket-blog.db` (remove `-wal`/`-shm` files) and `media/`, start.

Every person can also export their own blogs: JSON (`/api/v1/export.json`), Markdown files with front matter and the pictures (`/api/v1/export/markdown.zip`) — both go back in with Bring posts here — or one blog as a ready static website (`/api/v1/export/site.zip?blog=<id>`).
