CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL DEFAULT '',
  password_hash TEXT NOT NULL DEFAULT '',
  role          TEXT NOT NULL DEFAULT 'member',
  created_at    TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  name         TEXT NOT NULL DEFAULT '',
  token_hash   TEXT NOT NULL UNIQUE,
  prefix       TEXT NOT NULL DEFAULT '',
  last_used_at TEXT,
  created_at   TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- Box-wide settings (title, setup flag); everything else is personal.
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL DEFAULT ''
);

CREATE TABLE user_settings (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  key     TEXT NOT NULL,
  value   TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (user_id, key)
);

CREATE TABLE cache (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL DEFAULT '',
  fetched_at TEXT NOT NULL
);


-- ---------- Pocket Blog: everything below is per person ----------

-- A blog: its name, its address (handle; box-wide unique, readers open
-- /b/<handle>/), its look and who may read it.
CREATE TABLE blogs (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  handle         TEXT NOT NULL UNIQUE,
  title          TEXT NOT NULL DEFAULT '',
  tagline        TEXT NOT NULL DEFAULT '',
  about          TEXT NOT NULL DEFAULT '',             -- Markdown, on the home page
  author_name    TEXT NOT NULL DEFAULT '',
  author_bio     TEXT NOT NULL DEFAULT '',
  avatar         TEXT NOT NULL DEFAULT '',             -- media file name
  theme          TEXT NOT NULL DEFAULT 'classic',      -- classic | magazine | minimal | journal
  accent         TEXT NOT NULL DEFAULT '#3157d5',
  font           TEXT NOT NULL DEFAULT 'serif',        -- serif | sans
  language       TEXT NOT NULL DEFAULT 'en',
  visibility     TEXT NOT NULL DEFAULT 'public',       -- public | unlisted | private
  comments       TEXT NOT NULL DEFAULT 'moderated',    -- off | moderated | open
  per_page       INTEGER NOT NULL DEFAULT 10,
  show_reading   INTEGER NOT NULL DEFAULT 1,           -- "4 min read"
  footer         TEXT NOT NULL DEFAULT '',
  nav            TEXT NOT NULL DEFAULT '[]',           -- [{label, url}] extra menu links
  socials        TEXT NOT NULL DEFAULT '{}',           -- {"instagram": "https://…", …}
  domain         TEXT NOT NULL DEFAULT '',             -- own web address (host name), '' = none
  created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX blogs_user ON blogs(user_id, id);
CREATE UNIQUE INDEX blogs_domain ON blogs(domain) WHERE domain <> '';

-- A post or a page (About, Contact…): Markdown written in the editor.
CREATE TABLE posts (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id         INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  blog_id         INTEGER NOT NULL REFERENCES blogs(id) ON DELETE CASCADE,
  kind            TEXT NOT NULL DEFAULT 'post',        -- post | page
  title           TEXT NOT NULL DEFAULT '',
  slug            TEXT NOT NULL,                       -- the last part of its address
  body            TEXT NOT NULL DEFAULT '',            -- Markdown
  excerpt         TEXT NOT NULL DEFAULT '',            -- the short version in lists and shares
  cover           TEXT NOT NULL DEFAULT '',            -- media file name
  cover_alt       TEXT NOT NULL DEFAULT '',
  tags            TEXT NOT NULL DEFAULT '',
  status          TEXT NOT NULL DEFAULT 'draft',       -- draft | scheduled | published
  publish_at      TEXT NOT NULL DEFAULT '',            -- RFC 3339 UTC: when a scheduled post goes out
  published_at    TEXT NOT NULL DEFAULT '',            -- RFC 3339 UTC, shown as its date
  featured        INTEGER NOT NULL DEFAULT 0,          -- pinned on top of the home page
  in_menu         INTEGER NOT NULL DEFAULT 0,          -- pages: listed in the menu
  menu_order      INTEGER NOT NULL DEFAULT 0,
  comments        INTEGER NOT NULL DEFAULT 1,          -- comments allowed on this one
  seo_description TEXT NOT NULL DEFAULT '',
  preview_key     TEXT NOT NULL DEFAULT '',            -- share a draft: /b/<handle>/<slug>?preview=<key>
  words           INTEGER NOT NULL DEFAULT 0,
  views           INTEGER NOT NULL DEFAULT 0,
  search          TEXT NOT NULL DEFAULT '',            -- folded title + words for search
  source          TEXT NOT NULL DEFAULT '',            -- '' written here | import | <app slug>
  source_ref      TEXT NOT NULL DEFAULT '',
  created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
  updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE UNIQUE INDEX posts_slug ON posts(blog_id, slug);
CREATE INDEX posts_user ON posts(user_id, blog_id, status, published_at);
CREATE INDEX posts_due ON posts(status, publish_at);
CREATE UNIQUE INDEX posts_source ON posts(user_id, blog_id, source, source_ref) WHERE source_ref <> '';

-- A picture uploaded for a post (a file under DATA_DIR/media/; the file name
-- is random, so its address cannot be guessed).
CREATE TABLE media (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  file       TEXT NOT NULL UNIQUE,
  name       TEXT NOT NULL DEFAULT '',               -- the name it was uploaded with
  mime       TEXT NOT NULL DEFAULT '',
  bytes      INTEGER NOT NULL DEFAULT 0,
  width      INTEGER NOT NULL DEFAULT 0,
  height     INTEGER NOT NULL DEFAULT 0,
  alt        TEXT NOT NULL DEFAULT '',
  source     TEXT NOT NULL DEFAULT '',               -- '' uploaded | import | <app slug>
  source_ref TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX media_user ON media(user_id, created_at);
CREATE UNIQUE INDEX media_source ON media(user_id, source, source_ref) WHERE source_ref <> '';

-- What readers write under a post. Held until the writer approves it when
-- the blog moderates comments.
CREATE TABLE comments (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE, -- the blog's writer
  post_id    INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  parent_id  INTEGER REFERENCES comments(id) ON DELETE CASCADE,
  name       TEXT NOT NULL DEFAULT '',
  email      TEXT NOT NULL DEFAULT '',               -- never shown
  body       TEXT NOT NULL DEFAULT '',               -- plain text
  status     TEXT NOT NULL DEFAULT 'pending',        -- pending | approved | spam
  by_author  INTEGER NOT NULL DEFAULT 0,             -- the writer's own reply
  ip_hash    TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX comments_user ON comments(user_id, status, created_at);
CREATE INDEX comments_post ON comments(post_id, status, created_at);

-- Readers per post per day: a count, nothing about who (no cookies).
CREATE TABLE views (
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  post_id INTEGER NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
  day     TEXT NOT NULL,
  views   INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (post_id, day)
);
CREATE INDEX views_user ON views(user_id, day);
