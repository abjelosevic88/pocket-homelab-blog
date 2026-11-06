# Configuration

Environment variables; every one has a default. Personal settings (writing, notifications, connections, AI) live in the UI per person — the `OPENAI_*`/`NOTIFY_*` values below are copied once into the first admin's settings. Everything about a blog (name, look, address, comments) is set in the app under **Look & details**.

| Variable | Default | What |
|---|---|---|
| `PORT`, `HOST` | `8080`, `0.0.0.0` | Listen address |
| `BASE_URL` | – | Where the app is (links in pushes, the OIDC callback); blogs are at `BASE_URL/b/<address>/` |
| `PUBLIC_URL` | – | A second hostname that serves **only** the blogs, feeds and pictures — no sign-in page, no API. Use it to put the blogs on the internet while the app stays private |
| `DATA_DIR` | `./data` | Database, pictures, Core token, backups |
| `DB_PATH` | `$DATA_DIR/pocket-blog.db` | SQLite file |
| `MEDIA_DIR` | `$DATA_DIR/media` | Uploaded pictures (random file names) |
| `MAX_UPLOAD_BYTES` | `26214400` | Largest picture upload (25 MB; photos are made at most 2400 px and lose their camera data) |
| `MAX_BODY_BYTES` | `67108864` | Largest JSON request or import file (64 MB) |
| `TZ` | – | Time zone for dates, scheduled posts and the morning message |
| `AUTH_MODE` | `local` | `local` (accounts), `proxy` (trust `AUTH_PROXY_HEADER`/`AUTH_PROXY_EMAIL_HEADER`), `none` |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | Seed the first admin while the database is empty |
| `SESSION_TTL`, `SECURE_COOKIES` | `720h`, `false` | Set `SECURE_COOKIES=true` behind HTTPS |
| `OIDC_ISSUER`, `OIDC_CLIENT_ID`, `OIDC_CLIENT_SECRET`, `OIDC_NAME`, `OIDC_AUTO_CREATE` | – | Sign in with an OpenID Connect provider (not needed with Pocket Core) |
| `NOTIFY_URL`, `NOTIFY_FORMAT` | – | Apprise (`…/notify/key`), ntfy topic URL or JSON webhook; format `apprise`/`ntfy`/`json` |
| `NOTIFY_HOUR` | `8` | Default hour of the morning note (each person can change theirs) |
| `OPENAI_API_KEY`, `OPENAI_MODEL`, `OPENAI_BASE_URL` | – | Writing help without Pocket AI (any OpenAI-compatible API: Ollama, LM Studio, OpenRouter…) |
| `CORE_URL`, `CORE_REGISTRATION_KEY`, `CORE_APP_URL` | – | Pocket Core |
| `TRUST_PROXY`, `METRICS_ENABLED`, `LOG_LEVEL`, `LOG_FORMAT` | `true`, `true`, `info`, `text` | |
| `DEMO_DATA` | `false` | Seed a lived-in blog (posts, pages, a draft, a scheduled post, comments, three months of readers) into the owner account |

Commands: `pocket-blog backup [-o FILE]`, `pocket-blog reset-password [-email] [-password]`, `pocket-blog version`.

Connections to Pocket Travel, Places, Bookshelf, Kitchen, Watch, Notes and Voice are made per person under Settings → Connected apps: through Pocket Core (nothing to paste) or directly with the app's URL and an API token. Blog only ever reads from them.

## Putting blogs on the internet

The app and the blogs can live on different hostnames. A reverse proxy sends a public hostname (say `read.example.com`) to the same container and you set `PUBLIC_URL=https://read.example.com`: on that hostname only `/b/…`, `/media/…` and the blog stylesheet answer, everything else is a 404. A blog's own domain (Look & details → Your own web address) works the same way: point it at the server and it shows that blog at its root.

Public pages run no JavaScript (`Content-Security-Policy: script-src 'none'`), raw HTML in posts is never passed through, and videos come only from YouTube (privacy mode) and Vimeo.
