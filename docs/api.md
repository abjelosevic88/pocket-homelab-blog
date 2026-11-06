# API

JSON over HTTP at `/api/v1`. Authenticate with a session cookie (the web app), a personal token (`Authorization: Bearer pbg_…`, Settings → API & shortcuts), a Pocket Core token, or a Pocket Core app-link token (`pcx_…`, only within its scopes). Everything is per person: another person's ids are 404.

## Blogs

| | |
|---|---|
| `GET /blogs` | Your blogs with counts and their public `url` |
| `POST /blogs` | `{title, handle?, tagline, about, about_page?, theme, accent, font, language}` — `handle` is made from the title when empty |
| `GET/PUT/DELETE /blogs/{id}` | PUT takes any subset: `title, tagline, about, author_name, author_bio, avatar, theme (classic, magazine, minimal, journal), accent (#rrggbb), font (serif, sans), language, visibility (public, unlisted, private), comments (off, moderated, open), per_page, show_reading, footer, nav [{label,url}], socials {instagram,…}, domain, handle` |
| `GET /blogs/check-handle?handle=` | `{ok, reason, suggestion}` |

## Posts and pages

| | |
|---|---|
| `GET /posts` | `?blog= kind=post|page status=draft|scheduled|published tag= q= limit= offset= sort=updated|oldest|views|menu` → `{items, total}` |
| `POST /posts` | `{blog_id?, kind, title, body (Markdown), excerpt, cover, cover_alt, tags, featured, in_menu, comments, seo_description, source, source_ref, publish ("now" or a time)}` — a draft unless `publish` is set; the same `source`+`source_ref` returns the existing post |
| `GET/PUT/DELETE /posts/{id}` | PUT takes any subset (also `published_at` for published posts) |
| `POST /posts/{id}/publish` | `{at?}` — now, or scheduled for `at` (RFC 3339 or `2026-11-05T09:30` local) |
| `POST /posts/{id}/unpublish`, `/duplicate`, `/preview-key` | Back to draft · a copy · a new share-a-draft link |
| `GET /tags?blog=` | Tags with counts |

## Pictures, comments, readers

| | |
|---|---|
| `GET /media`, `POST /media` (multipart `file`, repeatable), `PUT /media/{id}` `{alt}`, `DELETE /media/{id}` | Pictures; each has a public `url` `/media/<random>.jpg` |
| `GET /comments?blog=&post=&status=pending|approved|spam` | Comments under your posts |
| `PUT /comments/{id}` `{status}`, `DELETE /comments/{id}`, `POST /comments/{id}/reply` `{body}` | Moderate and answer |
| `GET /stats?blog=&days=30` | `{views, previous, by_day[], top[], comments}` |

## For the rest of the box

| | Scope |
|---|---|
| `GET /summary` — blogs, drafts, what goes out next, readers this week, comments waiting | `blog:read` |
| `GET /day/YYYY-MM-DD` — posts published and written that day, readers, comments (Pocket Day) | `blog:read` |
| `GET /search?q=` — `{results:[{id,title,snippet,status,kind,date,url,link}]}` (Pocket Search, Pocket AI) | `blog:read` |
| `POST /posts`, `PUT /posts/{id}`, `POST /posts/{id}/publish|unpublish` — only posts that app started | `blog:write` |

## Writing help and other apps

| | |
|---|---|
| `POST /ai/write` | `{action, blog_id, title, body, text, instruction, language}` — `draft, outline, improve, fix, shorter, longer, tone, continue, rewrite, titles, summary, translate, setup` |
| `GET /sources` | The apps a post can start from and whether each is connected |
| `GET /sources/{app}?q=` | Items in that app (`notes, travel, places, bookshelf, kitchen, watch, voice`) |
| `POST /sources/{app}/draft` | `{id, blog_id, ai, note, again}` → `{post, ai, ai_error}` |
| `GET /ideas` | Recent things in your apps that would make a post |

## Data

`GET /export.json`, `GET /export/markdown.zip`, `GET /export/site.zip?blog=` (a static website), `POST /import` (multipart `file`: Pocket Blog JSON or ZIP, WordPress `.xml`, `.md`, `.zip` of Markdown), `GET /backup` (admins).

## Public

`/b/<handle>/` home · `/<slug>` post or page (`?preview=<key>` for a draft) · `/tag/<tag>` · `/archive` · `/search?q=` · `/feed.xml` (RSS) · `/sitemap.xml` · `/robots.txt` · `POST /<slug>/comments` (the comment form) · `/media/<file>` pictures.

Events on the Pocket Core bus: `blog.post_published`, `blog.comment_received`.
