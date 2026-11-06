package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

// ---- the blogs as readers see them ----
//
// Every blog lives at /b/<handle>/ on this app's address, at the root of its
// own web address when the writer set one (and the reverse proxy sends it
// here), and under PUBLIC_URL when the box has a public hostname for blogs
// only. Pages are rendered here, with no JavaScript at all.

//go:embed site/*
var siteFS embed.FS

var siteTmpl = template.Must(template.New("site").Funcs(template.FuncMap{
	"join": strings.Join,
	"args": func(v ...any) []any { return v },
	"safeCSS": func(s string) template.CSS {
		if reHex.MatchString(s) {
			return template.CSS(s)
		}
		return template.CSS("#3157d5")
	},
	"initial": func(s string) string {
		r, _ := utf8.DecodeRuneInString(strings.TrimSpace(s))
		if r == utf8.RuneError {
			return "·"
		}
		return strings.ToUpper(string(r))
	},
}).ParseFS(siteFS, "site/*.html"))

// sitePost is a post as a page shows it.
type sitePost struct {
	Title, URL, Date, ISODate, Excerpt, Cover, CoverAlt string
	Tags                                                []string
	Minutes                                             int
	HTML                                                template.HTML
	Featured                                            bool
	Comments                                            int
	Kind                                                string
}

type siteComment struct {
	ID       int64
	Name     string
	Date     string
	Body     []string // paragraphs
	ByAuthor bool
	Replies  []siteComment
}

type menuItem struct {
	Label, URL string
	Active     bool
}

type yearGroup struct {
	Year  string
	Posts []sitePost
}

type sitePage struct {
	Blog        *store.Blog
	Base        string // "" (own address) or "/b/<handle>"
	Abs         string // absolute address of the blog
	Lang        string
	W           map[string]string
	Kind        string // home | post | page | tag | archive | search | notfound | private
	Title       string
	Description string
	Canonical   string
	Image       string
	NoIndex     bool
	Menu        []menuItem
	Socials     []menuItem
	AboutHTML   template.HTML
	Avatar      string
	Posts       []sitePost
	Hero        *sitePost
	Post        *sitePost
	Prev, Next  *sitePost
	Comments    []siteComment
	CommentsN   int
	CanComment  bool
	FormStamp   string
	Notice      string
	NoticeBad   bool
	Tag, Query  string
	Years       []yearGroup
	OlderURL    string
	NewerURL    string
	Preview     bool
	FooterHTML  template.HTML
	Year        int
	Version     string
	ShowMade    bool
	Reading     bool
}

var reHex = regexpMust(`^#[0-9a-fA-F]{6}$`)

// ---- host switch ----

func hostOnly(h string) string {
	if host, _, err := net.SplitHostPort(h); err == nil {
		h = host
	}
	return strings.ToLower(strings.TrimSuffix(h, "."))
}

func urlHost(u string) string {
	if p, err := url.Parse(u); err == nil {
		return strings.ToLower(p.Hostname())
	}
	return ""
}

// hostSwitch sends requests for a blog's own address, and for the public
// blog hostname, to the blog pages; nothing else of the app is served there.
func (s *Server) hostSwitch(next http.Handler) http.Handler {
	baseHost, publicHost := urlHost(s.cfg.BaseURL), urlHost(s.cfg.PublicURL)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := hostOnly(r.Host)
		if host == "" || host == baseHost || host == "localhost" || net.ParseIP(host) != nil {
			next.ServeHTTP(w, r)
			return
		}
		if publicHost != "" && host == publicHost {
			p := r.URL.Path
			if p == "/healthz" || strings.HasPrefix(p, "/b/") || strings.HasPrefix(p, "/_blog/") || strings.HasPrefix(p, "/media/") {
				next.ServeHTTP(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}
		// any other name (host.docker.internal for Pocket Core and the apps
		// acting through it, a LAN name) is the app itself
		b, err := s.store.BlogByDomain(r.Context(), host)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		switch p := r.URL.Path; {
		case p == "/healthz", strings.HasPrefix(p, "/_blog/"), strings.HasPrefix(p, "/media/"):
			next.ServeHTTP(w, r)
		default:
			s.serveBlog(w, r, b, strings.TrimPrefix(p, "/"), "")
		}
	})
}

// blogAbs is the address readers open a blog at.
func (s *Server) blogAbs(b *store.Blog) string {
	switch {
	case b.Domain != "":
		return "https://" + b.Domain
	case s.cfg.PublicURL != "":
		return s.cfg.PublicURL + "/b/" + b.Handle
	case s.cfg.BaseURL != "":
		return s.cfg.BaseURL + "/b/" + b.Handle
	}
	return "/b/" + b.Handle
}

// postAbs is the address of one post.
func (s *Server) postAbs(b *store.Blog, p *store.Post) string {
	return s.blogAbs(b) + "/" + p.Slug
}

// mediaAbs is the address of a picture next to a blog ("" stays "").
func (s *Server) mediaAbs(b *store.Blog, file string) string {
	if file == "" {
		return ""
	}
	if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
		return file
	}
	root := s.blogAbs(b)
	if i := strings.Index(root, "/b/"); i >= 0 {
		root = root[:i]
	}
	return root + "/media/" + file
}

// ---- routes ----

func (s *Server) handleBlogRoot(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/b/"+strings.ToLower(chiParam(r, "handle"))+"/", http.StatusMovedPermanently)
}

func (s *Server) handleBlogPath(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.BlogByHandle(r.Context(), chiParam(r, "handle"))
	if err != nil {
		s.siteNotFound(w, r, nil, "")
		return
	}
	if b.Domain != "" && hostOnly(r.Host) == b.Domain {
		http.Redirect(w, r, "/"+chiParam(r, "*"), http.StatusMovedPermanently)
		return
	}
	s.serveBlog(w, r, b, chiParam(r, "*"), "/b/"+b.Handle)
}

// owner is the signed-in writer of the blog, when it is them reading.
func (s *Server) owner(r *http.Request, b *store.Blog) bool {
	c, err := r.Cookie("pbg_session")
	if err != nil || c.Value == "" {
		if s.cfg.AuthMode == "none" {
			return true
		}
		return false
	}
	u, err := s.sessionUser(r.Context(), c.Value)
	return err == nil && u != nil && u.ID == b.UserID
}

func (s *Server) serveBlog(w http.ResponseWriter, r *http.Request, b *store.Blog, rest, base string) {
	ctx := r.Context()
	rest = strings.Trim(rest, "/")
	isOwner := s.owner(r, b)
	// the writer may try another look without saving it (the theme gallery)
	if isOwner {
		q := r.URL.Query()
		if t := q.Get("theme"); oneOf(t, store.Themes) {
			b.Theme = t
		}
		if a := q.Get("accent"); reHex.MatchString(a) {
			b.Accent = a
		}
		if f := q.Get("font"); f == "serif" || f == "sans" {
			b.Font = f
		}
	}
	if b.Visibility == "private" && !isOwner && !(r.URL.Query().Get("preview") != "" && rest != "") {
		s.renderSite(w, http.StatusForbidden, s.basePage(ctx, b, base, "private", word(b.Language, "private")))
		return
	}
	switch {
	case rest == "" && r.Method == http.MethodGet, rest == "" && r.Method == http.MethodHead:
		s.siteHome(w, r, b, base, 1)
	case strings.HasPrefix(rest, "page/"):
		n, _ := strconv.Atoi(strings.TrimPrefix(rest, "page/"))
		if n < 2 {
			http.Redirect(w, r, base+"/", http.StatusMovedPermanently)
			return
		}
		s.siteHome(w, r, b, base, n)
	case strings.HasPrefix(rest, "tag/"):
		s.siteTag(w, r, b, base, strings.TrimPrefix(rest, "tag/"))
	case rest == "archive":
		s.siteArchive(w, r, b, base)
	case rest == "search":
		s.siteSearch(w, r, b, base)
	case rest == "feed.xml" || rest == "rss.xml" || rest == "rss":
		s.siteFeed(w, r, b)
	case rest == "sitemap.xml":
		s.siteSitemap(w, r, b)
	case rest == "robots.txt":
		s.siteRobots(w, r, b)
	case strings.HasSuffix(rest, "/comments") && r.Method == http.MethodPost:
		s.siteComment(w, r, b, base, strings.TrimSuffix(rest, "/comments"))
	case !strings.Contains(rest, "/"):
		s.sitePost(w, r, b, base, rest, isOwner)
	default:
		s.siteNotFound(w, r, b, base)
	}
}

// ---- pages ----

func (s *Server) basePage(ctx context.Context, b *store.Blog, base, kind, title string) *sitePage {
	pg := &sitePage{Blog: b, Base: base, Abs: s.blogAbs(b), Lang: firstNonEmpty(b.Language, "en"), Kind: kind, Year: time.Now().Year(),
		Version: Version, ShowMade: true, Reading: b.ShowReading, NoIndex: b.Visibility != "public"}
	pg.W = map[string]string{}
	for k := range siteWords["en"] {
		pg.W[k] = word(pg.Lang, k)
	}
	pg.Title = b.Title
	if title != "" {
		pg.Title = title + " · " + b.Title
	}
	pg.Description = firstNonEmpty(b.Tagline, clip(plainText(b.About), 160))
	pg.Canonical = pg.Abs + "/"
	pg.Menu = []menuItem{{Label: pg.W["home"], URL: base + "/", Active: kind == "home"}}
	for _, p := range s.store.MenuPages(ctx, b.ID) {
		pg.Menu = append(pg.Menu, menuItem{Label: p.Title, URL: base + "/" + p.Slug})
	}
	for _, l := range b.Nav {
		if l.Label != "" && safeLink(l.URL) {
			pg.Menu = append(pg.Menu, menuItem{Label: l.Label, URL: l.URL})
		}
	}
	for _, k := range socialOrder {
		if u := b.Socials[k]; safeLink(u) && u != "" {
			pg.Socials = append(pg.Socials, menuItem{Label: socialNames[k], URL: u})
		}
	}
	if b.Avatar != "" {
		pg.Avatar = "/media/" + b.Avatar
	}
	if b.Footer != "" {
		pg.FooterHTML = renderMarkdown(b.Footer, "")
	}
	return pg
}

var socialOrder = []string{"instagram", "facebook", "x", "mastodon", "bluesky", "linkedin", "youtube", "tiktok", "github", "website", "email"}
var socialNames = map[string]string{"instagram": "Instagram", "facebook": "Facebook", "x": "X", "mastodon": "Mastodon", "bluesky": "Bluesky",
	"linkedin": "LinkedIn", "youtube": "YouTube", "tiktok": "TikTok", "github": "GitHub", "website": "Website", "email": "Email"}

// safeLink allows web and mail links and links inside the blog.
func safeLink(u string) bool {
	u = strings.TrimSpace(strings.ToLower(u))
	return u == "" || strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "mailto:") || (strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//"))
}

func (s *Server) toSite(b *store.Blog, base string, p *store.Post, full bool) sitePost {
	sp := sitePost{Title: firstNonEmpty(p.Title, "Untitled"), URL: base + "/" + p.Slug, Date: longDate(b.Language, firstNonEmpty(p.PublishedAt, p.UpdatedAt)),
		ISODate: firstNonEmpty(p.PublishedAt, p.UpdatedAt), Excerpt: p.Excerpt, CoverAlt: p.CoverAlt, Tags: store.SplitTags(p.Tags),
		Minutes: store.ReadingMinutes(p.Words), Featured: p.Featured, Comments: p.CommentsN, Kind: p.Kind}
	if p.Cover != "" {
		sp.Cover = "/media/" + p.Cover
		if strings.HasPrefix(p.Cover, "http") {
			sp.Cover = p.Cover
		}
	}
	if full {
		sp.HTML = renderMarkdown(p.Body, "")
	}
	return sp
}

func (s *Server) listing(b *store.Blog, base string, ps []store.Post) []sitePost {
	out := make([]sitePost, 0, len(ps))
	for i := range ps {
		sp := s.toSite(b, base, &ps[i], false)
		if sp.Excerpt == "" {
			sp.Excerpt = ""
		}
		out = append(out, sp)
	}
	return out
}

func (s *Server) siteHome(w http.ResponseWriter, r *http.Request, b *store.Blog, base string, page int) {
	ctx := r.Context()
	per := b.PerPage
	if per < 3 || per > 50 {
		per = 10
	}
	ps, total, err := s.store.ListPosts(ctx, 0, store.PostFilter{BlogID: b.ID, Kind: "post", Public: true, Limit: per, Offset: (page - 1) * per})
	if err != nil {
		s.siteError(w, err)
		return
	}
	if page > 1 && len(ps) == 0 {
		s.siteNotFound(w, r, b, base)
		return
	}
	s.fillExcerpts(ctx, ps)
	pg := s.basePage(ctx, b, base, "home", "")
	if page > 1 {
		pg.Title = fmt.Sprintf("%s · %d", b.Title, page)
		pg.Canonical = fmt.Sprintf("%s/page/%d", pg.Abs, page)
		pg.Menu[0].Active = false
	} else if b.About != "" {
		pg.AboutHTML = renderMarkdown(b.About, "")
	}
	pg.Posts = s.listing(b, base, ps)
	if page == 1 && b.Theme == "magazine" && len(pg.Posts) > 0 {
		h := pg.Posts[0]
		pg.Hero = &h
		pg.Posts = pg.Posts[1:]
	}
	if page*per < total {
		pg.OlderURL = fmt.Sprintf("%s/page/%d", base, page+1)
	}
	if page == 2 {
		pg.NewerURL = base + "/"
	} else if page > 2 {
		pg.NewerURL = fmt.Sprintf("%s/page/%d", base, page-1)
	}
	if len(ps) > 0 && ps[0].Cover != "" {
		pg.Image = s.mediaAbs(b, ps[0].Cover)
	}
	if b.Avatar != "" {
		pg.Image = s.mediaAbs(b, b.Avatar)
	}
	s.renderSite(w, http.StatusOK, pg)
}

// fillExcerpts gives every listed post its excerpt (written, or the start of the text).
func (s *Server) fillExcerpts(ctx context.Context, ps []store.Post) {
	for i := range ps {
		if ps[i].Excerpt != "" {
			continue
		}
		if full, err := s.store.GetPost(ctx, ps[i].UserID, ps[i].ID); err == nil {
			ps[i].Excerpt = autoExcerpt(full.Body)
		}
	}
}

func (s *Server) siteTag(w http.ResponseWriter, r *http.Request, b *store.Blog, base, tag string) {
	tag, _ = url.PathUnescape(tag)
	ps, _, err := s.store.ListPosts(r.Context(), 0, store.PostFilter{BlogID: b.ID, Kind: "post", Public: true, Tag: tag, Limit: 200})
	if err != nil {
		s.siteError(w, err)
		return
	}
	if len(ps) == 0 {
		s.siteNotFound(w, r, b, base)
		return
	}
	s.fillExcerpts(r.Context(), ps)
	pg := s.basePage(r.Context(), b, base, "tag", "#"+tag)
	pg.Menu[0].Active = false
	pg.Tag = tag
	pg.Canonical = pg.Abs + "/tag/" + url.PathEscape(tag)
	pg.Posts = s.listing(b, base, ps)
	s.renderSite(w, http.StatusOK, pg)
}

func (s *Server) siteArchive(w http.ResponseWriter, r *http.Request, b *store.Blog, base string) {
	ps, _, err := s.store.ListPosts(r.Context(), 0, store.PostFilter{BlogID: b.ID, Kind: "post", Public: true})
	if err != nil {
		s.siteError(w, err)
		return
	}
	pg := s.basePage(r.Context(), b, base, "archive", word(b.Language, "archive"))
	pg.Menu[0].Active = false
	pg.Canonical = pg.Abs + "/archive"
	for _, sp := range s.listing(b, base, ps) {
		y := ""
		if len(sp.ISODate) >= 4 {
			if t, err := time.Parse(time.RFC3339Nano, sp.ISODate); err == nil {
				y = strconv.Itoa(t.Local().Year())
			}
		}
		if n := len(pg.Years); n == 0 || pg.Years[n-1].Year != y {
			pg.Years = append(pg.Years, yearGroup{Year: y})
		}
		pg.Years[len(pg.Years)-1].Posts = append(pg.Years[len(pg.Years)-1].Posts, sp)
	}
	s.renderSite(w, http.StatusOK, pg)
}

func (s *Server) siteSearch(w http.ResponseWriter, r *http.Request, b *store.Blog, base string) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	pg := s.basePage(r.Context(), b, base, "search", word(b.Language, "search"))
	pg.Menu[0].Active = false
	pg.Query = q
	pg.NoIndex = true
	if q != "" {
		ps, _, err := s.store.ListPosts(r.Context(), 0, store.PostFilter{BlogID: b.ID, Public: true, Q: clip(q, 100), Limit: 50})
		if err != nil {
			s.siteError(w, err)
			return
		}
		s.fillExcerpts(r.Context(), ps)
		pg.Posts = s.listing(b, base, ps)
	}
	s.renderSite(w, http.StatusOK, pg)
}

func (s *Server) sitePost(w http.ResponseWriter, r *http.Request, b *store.Blog, base, slug string, isOwner bool) {
	ctx := r.Context()
	p, err := s.store.PostBySlug(ctx, b.ID, slug)
	if err != nil {
		s.siteNotFound(w, r, b, base)
		return
	}
	live := p.Status == store.StatusPublished && p.PublishedAt <= store.Now()
	preview := !live
	if !live {
		key := r.URL.Query().Get("preview")
		if !isOwner && (key == "" || !hmac.Equal([]byte(key), []byte(p.PreviewKey))) {
			s.siteNotFound(w, r, b, base)
			return
		}
	}
	full, _ := s.store.GetPost(ctx, p.UserID, p.ID)
	if full != nil {
		p = full
	}
	pg := s.basePage(ctx, b, base, p.Kind, firstNonEmpty(p.Title, "Untitled"))
	pg.Menu[0].Active = false
	for i := range pg.Menu {
		if pg.Menu[i].URL == base+"/"+p.Slug {
			pg.Menu[i].Active = true
		}
	}
	sp := s.toSite(b, base, p, true)
	pg.Post = &sp
	pg.Preview = preview
	pg.Canonical = s.postAbs(b, p)
	pg.Description = firstNonEmpty(p.SEODescription, p.Excerpt, clip(plainText(p.Body), 160))
	pg.Image = s.mediaAbs(b, firstNonEmpty(p.Cover, firstImage(p.Body)))
	if strings.HasPrefix(firstImage(p.Body), "/media/") && p.Cover == "" {
		pg.Image = s.mediaAbs(b, strings.TrimPrefix(firstImage(p.Body), "/media/"))
	}
	if preview {
		pg.NoIndex = true
	}
	if p.Kind == "post" {
		if live {
			pg.Prev, pg.Next = nil, nil
			if pv, nx := s.store.Neighbours(ctx, p); pv != nil || nx != nil {
				if pv != nil {
					x := s.toSite(b, base, pv, false)
					pg.Prev = &x
				}
				if nx != nil {
					x := s.toSite(b, base, nx, false)
					pg.Next = &x
				}
			}
		}
		cs, _ := s.store.PublicComments(ctx, p.ID)
		pg.Comments, pg.CommentsN = threadComments(b.Language, cs), len(cs)
		pg.CanComment = live && b.Comments != "off" && p.Comments
		pg.FormStamp = s.formStamp(p.ID)
	}
	switch r.URL.Query().Get("comment") {
	case "held":
		pg.Notice = word(b.Language, "held")
	case "thanks":
		pg.Notice = word(b.Language, "thanks")
	case "wait":
		pg.Notice, pg.NoticeBad = word(b.Language, "too_many"), true
	case "empty":
		pg.Notice, pg.NoticeBad = word(b.Language, "empty"), true
	}
	if live && !isOwner && r.Method == http.MethodGet && !isBot(r.UserAgent()) && s.firstView(r, p.ID) {
		_ = s.store.AddView(ctx, p)
	}
	s.renderSite(w, http.StatusOK, pg)
}

func threadComments(lang string, cs []store.Comment) []siteComment {
	byID := map[int64]int{}
	var top []siteComment
	toSite := func(c store.Comment) siteComment {
		var paras []string
		for _, para := range strings.Split(strings.ReplaceAll(c.Body, "\r", ""), "\n\n") {
			if t := strings.TrimSpace(para); t != "" {
				paras = append(paras, t)
			}
		}
		return siteComment{ID: c.ID, Name: firstNonEmpty(c.Name, "—"), Date: longDate(lang, c.CreatedAt), Body: paras, ByAuthor: c.ByAuthor}
	}
	for _, c := range cs {
		if c.ParentID == nil {
			byID[c.ID] = len(top)
			top = append(top, toSite(c))
		}
	}
	for _, c := range cs {
		if c.ParentID != nil {
			if i, ok := byID[*c.ParentID]; ok {
				top[i].Replies = append(top[i].Replies, toSite(c))
			} else {
				top = append(top, toSite(c))
			}
		}
	}
	return top
}

func (s *Server) siteNotFound(w http.ResponseWriter, r *http.Request, b *store.Blog, base string) {
	if b == nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "There is no blog at this address.")
		return
	}
	pg := s.basePage(r.Context(), b, base, "notfound", "404")
	pg.Menu[0].Active = false
	pg.NoIndex = true
	s.renderSite(w, http.StatusNotFound, pg)
}

func (s *Server) siteError(w http.ResponseWriter, err error) {
	s.log.Error("blog page", "err", err)
	http.Error(w, "Something went wrong. Please try again in a moment.", http.StatusInternalServerError)
}

// sitePolicy: no scripts at all; pictures from anywhere; videos from YouTube and Vimeo.
const sitePolicy = "default-src 'self'; script-src 'none'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; " +
	"frame-src https://www.youtube-nocookie.com https://player.vimeo.com; form-action 'self'; base-uri 'none'; object-src 'none'"

func (s *Server) renderSite(w http.ResponseWriter, status int, pg *sitePage) {
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Content-Security-Policy", sitePolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	if pg.Preview || pg.Kind == "private" {
		h.Set("Cache-Control", "private, no-store")
	} else {
		h.Set("Cache-Control", "public, max-age=60")
	}
	if pg.NoIndex {
		h.Set("X-Robots-Tag", "noindex")
	}
	w.WriteHeader(status)
	if err := siteTmpl.ExecuteTemplate(w, "page", pg); err != nil {
		s.log.Error("blog page render", "err", err)
	}
}

func (s *Server) handleSiteCSS(w http.ResponseWriter, r *http.Request) {
	b, err := siteFS.ReadFile("site/site.css")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = w.Write(b)
}

// ---- readers: counted once a day each, never tracked ----

var botWords = []string{"bot", "crawl", "spider", "slurp", "preview", "fetch", "monitor", "curl", "wget", "python", "go-http", "feed", "rss", "headless", "lighthouse"}

func isBot(ua string) bool {
	ua = strings.ToLower(ua)
	if ua == "" {
		return true
	}
	for _, b := range botWords {
		if strings.Contains(ua, b) {
			return true
		}
	}
	return false
}

var seen = struct {
	sync.Mutex
	day  string
	keys map[string]bool
}{keys: map[string]bool{}}

func (s *Server) firstView(r *http.Request, postID int64) bool {
	day := store.Today()
	key := fmt.Sprintf("%s|%d", s.readerHash(r), postID)
	seen.Lock()
	defer seen.Unlock()
	if seen.day != day || len(seen.keys) > 100000 {
		seen.day, seen.keys = day, map[string]bool{}
	}
	if seen.keys[key] {
		return false
	}
	seen.keys[key] = true
	return true
}

// siteSecret is this box's key for comment forms and reader hashes.
func (s *Server) siteSecret() []byte {
	s.secretOnce.Do(func() {
		ctx := context.Background()
		v, _, ok := s.store.CacheGet(ctx, "site-secret")
		if !ok || len(v) < 32 {
			v = store.RandomKey(32)
			_ = s.store.CachePut(ctx, "site-secret", v)
		}
		s.secret = []byte(v)
	})
	return s.secret
}

// readerHash stands for a reader for one day: their address, salted, hashed.
func (s *Server) readerHash(r *http.Request) string {
	ip := r.RemoteAddr
	if h, _, err := net.SplitHostPort(ip); err == nil {
		ip = h
	}
	m := hmac.New(sha256.New, s.siteSecret())
	m.Write([]byte(ip + "|" + store.Today()))
	return hex.EncodeToString(m.Sum(nil))[:24]
}

func (s *Server) formStamp(postID int64) string {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	m := hmac.New(sha256.New, s.siteSecret())
	fmt.Fprintf(m, "%d|%s", postID, ts)
	return ts + "." + hex.EncodeToString(m.Sum(nil))[:20]
}

// stampOK: the form came from this page, at least 3 seconds and at most a day ago.
func (s *Server) stampOK(postID int64, stamp string) bool {
	ts, sig, ok := strings.Cut(stamp, ".")
	if !ok {
		return false
	}
	m := hmac.New(sha256.New, s.siteSecret())
	fmt.Fprintf(m, "%d|%s", postID, ts)
	if !hmac.Equal([]byte(sig), []byte(hex.EncodeToString(m.Sum(nil))[:20])) {
		return false
	}
	n, err := strconv.ParseInt(ts, 10, 64)
	age := time.Since(time.Unix(n, 0))
	return err == nil && age >= s.commentMinAge && age <= 24*time.Hour
}

// siteComment takes a reader's comment from the form under a post.
func (s *Server) siteComment(w http.ResponseWriter, r *http.Request, b *store.Blog, base, slug string) {
	ctx := r.Context()
	p, err := s.store.PostBySlug(ctx, b.ID, slug)
	if err != nil || p.Status != store.StatusPublished || p.Kind != "post" {
		s.siteNotFound(w, r, b, base)
		return
	}
	back := base + "/" + p.Slug
	if b.Comments == "off" || !p.Comments {
		http.Redirect(w, r, back+"#comments", http.StatusSeeOther)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	// a hidden field people never fill in, and a form that was really shown
	if r.PostForm.Get("website") != "" || !s.stampOK(p.ID, r.PostForm.Get("stamp")) {
		http.Redirect(w, r, back+"?comment=held#comments", http.StatusSeeOther)
		return
	}
	name := clip(strings.TrimSpace(r.PostForm.Get("name")), 80)
	body := strings.TrimSpace(r.PostForm.Get("body"))
	if name == "" || body == "" {
		http.Redirect(w, r, back+"?comment=empty#comment-form", http.StatusSeeOther)
		return
	}
	if utf8.RuneCountInString(body) > 5000 {
		body = string([]rune(body)[:5000])
	}
	hash := s.readerHash(r)
	s.commentMu.Lock()
	defer s.commentMu.Unlock()
	if s.store.RecentCommentsFrom(ctx, hash, 10*time.Minute) >= 5 {
		http.Redirect(w, r, back+"?comment=wait#comment-form", http.StatusSeeOther)
		return
	}
	c := &store.Comment{UserID: p.UserID, PostID: p.ID, Name: name, Email: clip(strings.TrimSpace(r.PostForm.Get("email")), 200), Body: body, IPHash: hash, Status: "pending"}
	if b.Comments == "open" && !looksLikeSpam(body) {
		c.Status = "approved"
	}
	if id, err := strconv.ParseInt(r.PostForm.Get("parent"), 10, 64); err == nil && id > 0 {
		if pc, err := s.store.GetComment(ctx, p.UserID, id); err == nil && pc.PostID == p.ID {
			c.ParentID = &id
		}
	}
	if err := s.store.AddComment(ctx, c); err != nil {
		s.siteError(w, err)
		return
	}
	s.commentArrived(context.WithoutCancel(ctx), b, p, c)
	if c.Status == "approved" {
		http.Redirect(w, r, fmt.Sprintf("%s?comment=thanks#c%d", back, c.ID), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, back+"?comment=held#comments", http.StatusSeeOther)
}

// looksLikeSpam: many links, or link markup, is held even on open blogs.
func looksLikeSpam(body string) bool {
	l := strings.ToLower(body)
	return strings.Count(l, "http://")+strings.Count(l, "https://") > 2 || strings.Contains(l, "[url=") || strings.Contains(l, "<a href")
}

// ---- feed, sitemap, robots ----

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Content string     `xml:"xmlns:content,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Self        rssSelf   `xml:"atom:link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language"`
	Generator   string    `xml:"generator"`
	Updated     string    `xml:"lastBuildDate,omitempty"`
	Items       []rssItem `xml:"item"`
}

type rssSelf struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	PubDate     string   `xml:"pubDate"`
	Description string   `xml:"description"`
	Content     cdata    `xml:"content:encoded"`
	Categories  []string `xml:"category"`
}

type cdata struct {
	Text string `xml:",cdata"`
}

func (s *Server) siteFeed(w http.ResponseWriter, r *http.Request, b *store.Blog) {
	ctx := r.Context()
	ps, _, err := s.store.ListPosts(ctx, 0, store.PostFilter{BlogID: b.ID, Kind: "post", Public: true, Limit: 20, Body: true, Sort: "published"})
	if err != nil {
		s.siteError(w, err)
		return
	}
	abs := s.blogAbs(b)
	root := abs
	if i := strings.Index(root, "/b/"); i >= 0 {
		root = root[:i]
	}
	f := rssFeed{Version: "2.0", Content: "http://purl.org/rss/1.0/modules/content/", Atom: "http://www.w3.org/2005/Atom",
		Channel: rssChannel{Title: b.Title, Link: abs + "/", Self: rssSelf{Href: abs + "/feed.xml", Rel: "self", Type: "application/rss+xml"},
			Description: firstNonEmpty(b.Tagline, b.Title), Language: b.Language, Generator: "Pocket Blog " + Version}}
	for i := range ps {
		p := &ps[i]
		t, _ := time.Parse(time.RFC3339Nano, p.PublishedAt)
		if i == 0 {
			f.Channel.Updated = t.Format(time.RFC1123Z)
		}
		body := string(renderMarkdown(p.Body, root))
		if p.Cover != "" {
			body = `<p><img src="` + template.HTMLEscapeString(s.mediaAbs(b, p.Cover)) + `" alt="` + template.HTMLEscapeString(p.CoverAlt) + `" /></p>` + body
		}
		f.Channel.Items = append(f.Channel.Items, rssItem{Title: p.Title, Link: s.postAbs(b, p), GUID: s.postAbs(b, p), PubDate: t.Format(time.RFC1123Z),
			Description: firstNonEmpty(p.Excerpt, autoExcerpt(p.Body)), Content: cdata{body}, Categories: store.SplitTags(p.Tags)})
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if b.Visibility == "private" {
		w.WriteHeader(http.StatusForbidden)
		return
	}
	_, _ = w.Write([]byte(xml.Header))
	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	_ = enc.Encode(f)
}

func (s *Server) siteSitemap(w http.ResponseWriter, r *http.Request, b *store.Blog) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	if b.Visibility != "public" {
		http.NotFound(w, r)
		return
	}
	ps, _, err := s.store.ListPosts(r.Context(), 0, store.PostFilter{BlogID: b.ID, Public: true})
	if err != nil {
		s.siteError(w, err)
		return
	}
	var sb strings.Builder
	sb.WriteString(xml.Header + `<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	sb.WriteString("  <url><loc>" + template.HTMLEscapeString(s.blogAbs(b)+"/") + "</loc></url>\n")
	for i := range ps {
		mod := ps[i].UpdatedAt
		if len(mod) >= 10 {
			mod = mod[:10]
		}
		sb.WriteString("  <url><loc>" + template.HTMLEscapeString(s.postAbs(b, &ps[i])) + "</loc><lastmod>" + mod + "</lastmod></url>\n")
	}
	sb.WriteString("</urlset>\n")
	_, _ = w.Write([]byte(sb.String()))
}

func (s *Server) siteRobots(w http.ResponseWriter, r *http.Request, b *store.Blog) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if b.Visibility != "public" {
		fmt.Fprint(w, "User-agent: *\nDisallow: /\n")
		return
	}
	fmt.Fprintf(w, "User-agent: *\nAllow: /\n\nSitemap: %s/sitemap.xml\n", s.blogAbs(b))
}

// ---- pictures ----

// handleMediaFile serves an uploaded picture. The names are random, so only
// someone who was shown the picture knows its address.
func (s *Server) handleMediaFile(w http.ResponseWriter, r *http.Request) {
	file := chiParam(r, "file")
	if file == "" || strings.ContainsAny(file, "/\\") || strings.HasPrefix(file, ".") {
		http.NotFound(w, r)
		return
	}
	m, err := s.store.MediaByFile(r.Context(), file)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.cfg.MediaDir, m.File))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, _ := f.Stat()
	w.Header().Set("Content-Type", m.Mime)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	http.ServeContent(w, r, m.File, fi.ModTime(), f)
}
