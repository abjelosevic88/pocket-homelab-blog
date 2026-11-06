package server

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

func regexpMust(s string) *regexp.Regexp { return regexp.MustCompile(s) }

func chiParam(r *http.Request, name string) string { return chi.URLParam(r, name) }

var reDomain = regexpMust(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,24}$`)
var reLang = regexpMust(`^[a-z]{2,3}(-[a-z0-9]{2,8})?$`)

var reservedHandles = map[string]bool{"api": true, "admin": true, "static": true, "assets": true, "media": true, "settings": true,
	"login": true, "setup": true, "new": true, "www": true, "help": true}

// withURL fills what only the writer's lists show.
func (s *Server) withURL(b *store.Blog) *store.Blog {
	b.URL = s.blogAbs(b)
	return b
}

func (s *Server) handleListBlogs(w http.ResponseWriter, r *http.Request) {
	bs, err := s.store.ListBlogs(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "blogs")
		return
	}
	for i := range bs {
		s.withURL(&bs[i])
	}
	writeJSON(w, http.StatusOK, bs)
}

func (s *Server) handleGetBlog(w http.ResponseWriter, r *http.Request) {
	b, err := s.store.GetBlog(r.Context(), actorID(r.Context()), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "blog")
		return
	}
	writeJSON(w, http.StatusOK, s.withURL(b))
}

// suggestHandle turns a name into a free address: "Ana's Kitchen" → "anas-kitchen".
func (s *Server) suggestHandle(ctx context.Context, name string, except int64) string {
	base := store.Slugify(strings.ReplaceAll(name, "'", ""), 30)
	clean := strings.Builder{}
	for _, r := range base {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			clean.WriteRune(r)
		}
	}
	base = strings.Trim(clean.String(), "-")
	if len(base) < 2 {
		base = "my-blog"
	}
	h := base
	for i := 2; i < 500; i++ {
		if !reservedHandles[h] && !s.store.HandleTaken(ctx, h, except) {
			return h
		}
		h = base + "-" + itoa(int64(i))
	}
	return base + "-" + store.RandomKey(3)
}

// handleCheckHandle says whether an address is free (as the writer types it).
func (s *Server) handleCheckHandle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	h := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("handle")))
	except := int64(queryInt(r, "except"))
	if except > 0 {
		if _, err := s.store.GetBlog(ctx, actorID(ctx), except); err != nil {
			except = 0
		}
	}
	out := map[string]any{"handle": h, "ok": true, "reason": ""}
	switch {
	case !store.ValidHandle(h):
		out["ok"], out["reason"] = false, "Use 2 to 40 small letters, numbers and dashes"
	case reservedHandles[h]:
		out["ok"], out["reason"] = false, "That address is reserved"
	case s.store.HandleTaken(ctx, h, except):
		out["ok"], out["reason"] = false, "Somebody already has that address"
	}
	if ok, _ := out["ok"].(bool); !ok {
		out["suggestion"] = s.suggestHandle(ctx, firstNonEmpty(h, r.URL.Query().Get("name")), except)
	}
	writeJSON(w, http.StatusOK, out)
}

type blogIn struct {
	Handle      string            `json:"handle"`
	Title       string            `json:"title"`
	Tagline     string            `json:"tagline"`
	About       string            `json:"about"`
	AuthorName  string            `json:"author_name"`
	AuthorBio   string            `json:"author_bio"`
	Avatar      string            `json:"avatar"`
	Theme       string            `json:"theme"`
	Accent      string            `json:"accent"`
	Font        string            `json:"font"`
	Language    string            `json:"language"`
	Visibility  string            `json:"visibility"`
	Comments    string            `json:"comments"`
	PerPage     int               `json:"per_page"`
	ShowReading bool              `json:"show_reading"`
	Footer      string            `json:"footer"`
	Nav         []store.NavLink   `json:"nav"`
	Socials     map[string]string `json:"socials"`
	Domain      string            `json:"domain"`
	// on create: also make an "About" page from the about text
	AboutPage bool `json:"about_page"`
}

// applyBlog copies the sent fields onto b, checking each; it returns a
// plain-language problem, or "".
func (s *Server) applyBlog(ctx context.Context, b *store.Blog, in blogIn, keys map[string]bool) string {
	set := func(k string) bool { return keys == nil || keys[k] }
	if set("title") {
		b.Title = clip(strings.TrimSpace(in.Title), 120)
		if b.Title == "" {
			return "Give your blog a name"
		}
	}
	if set("handle") && (in.Handle != "" || keys != nil) {
		h := strings.ToLower(strings.TrimSpace(in.Handle))
		if !store.ValidHandle(h) {
			return "The address uses 2 to 40 small letters, numbers and dashes"
		}
		if reservedHandles[h] {
			return "That address is reserved; pick another"
		}
		if s.store.HandleTaken(ctx, h, b.ID) {
			return "Somebody already has that address; pick another"
		}
		b.Handle = h
	}
	if set("tagline") {
		b.Tagline = clip(strings.TrimSpace(in.Tagline), 200)
	}
	if set("about") {
		b.About = strings.TrimSpace(in.About)
	}
	if set("author_name") {
		b.AuthorName = clip(strings.TrimSpace(in.AuthorName), 80)
	}
	if set("author_bio") {
		b.AuthorBio = clip(strings.TrimSpace(in.AuthorBio), 400)
	}
	if set("avatar") {
		b.Avatar = strings.TrimSpace(in.Avatar)
		if b.Avatar != "" {
			if m, err := s.store.MediaByFile(ctx, b.Avatar); err != nil || (b.UserID != 0 && m.UserID != b.UserID) {
				return "That picture is not in your pictures"
			}
		}
	}
	if set("theme") && in.Theme != "" {
		if !oneOf(in.Theme, store.Themes) {
			return "Unknown look"
		}
		b.Theme = in.Theme
	}
	if set("accent") && in.Accent != "" {
		if !reHex.MatchString(in.Accent) {
			return "The colour is a code like #3157d5"
		}
		b.Accent = strings.ToLower(in.Accent)
	}
	if set("font") && in.Font != "" {
		if in.Font != "serif" && in.Font != "sans" {
			return "The letters are classic (serif) or modern (sans)"
		}
		b.Font = in.Font
	}
	if set("language") && in.Language != "" {
		l := strings.ToLower(strings.TrimSpace(in.Language))
		if !reLang.MatchString(l) {
			return "The language is a code like en or bs"
		}
		b.Language = l
	}
	if set("visibility") && in.Visibility != "" {
		if !oneOf(in.Visibility, []string{"public", "unlisted", "private"}) {
			return "Who can read it: public, unlisted or private"
		}
		b.Visibility = in.Visibility
	}
	if set("comments") && in.Comments != "" {
		if !oneOf(in.Comments, []string{"off", "moderated", "open"}) {
			return "Comments are off, moderated or open"
		}
		b.Comments = in.Comments
	}
	if set("per_page") && in.PerPage != 0 {
		if in.PerPage < 3 || in.PerPage > 50 {
			return "Show 3 to 50 posts per page"
		}
		b.PerPage = in.PerPage
	}
	if set("show_reading") {
		b.ShowReading = in.ShowReading
	}
	if set("footer") {
		b.Footer = clip(strings.TrimSpace(in.Footer), 1000)
	}
	if set("nav") {
		b.Nav = []store.NavLink{}
		for _, l := range in.Nav {
			l.Label, l.URL = clip(strings.TrimSpace(l.Label), 40), strings.TrimSpace(l.URL)
			if l.Label == "" && l.URL == "" {
				continue
			}
			if l.Label == "" || l.URL == "" || !safeLink(l.URL) {
				return "Each menu link needs a name and a web address (https://…)"
			}
			b.Nav = append(b.Nav, l)
		}
		if len(b.Nav) > 12 {
			return "At most 12 menu links"
		}
	}
	if set("socials") {
		b.Socials = map[string]string{}
		for k, v := range in.Socials {
			v = strings.TrimSpace(v)
			if v == "" || socialNames[k] == "" {
				continue
			}
			if k == "email" && !strings.HasPrefix(v, "mailto:") {
				v = "mailto:" + v
			}
			if !strings.Contains(v, ":") {
				v = "https://" + v
			}
			if !safeLink(v) {
				return "A social link must be a web address"
			}
			b.Socials[k] = v
		}
	}
	if set("domain") {
		d := strings.ToLower(strings.TrimSpace(in.Domain))
		d = strings.TrimPrefix(strings.TrimPrefix(d, "https://"), "http://")
		d = strings.TrimRight(d, "/")
		if d != "" && (!reDomain.MatchString(d) || d == urlHost(s.cfg.BaseURL) || d == urlHost(s.cfg.PublicURL)) {
			return "The web address is a name like myblog.com"
		}
		b.Domain = d
	}
	return ""
}

func (s *Server) handleCreateBlog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in blogIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	u := userFrom(ctx)
	b := &store.Blog{UserID: uid, Theme: "classic", Accent: "#3157d5", Font: "serif", Language: "en", Visibility: "public", Comments: "moderated",
		PerPage: 10, ShowReading: true, AuthorName: u.Name, Nav: []store.NavLink{}, Socials: map[string]string{}}
	in.ShowReading = true
	keys := map[string]bool{"title": true, "tagline": true, "about": true, "theme": true, "accent": true, "font": true, "language": true,
		"visibility": true, "comments": true, "avatar": true, "footer": true}
	if in.AuthorName != "" {
		keys["author_name"] = true
	}
	if in.AuthorBio != "" {
		keys["author_bio"] = true
	}
	if in.Handle != "" {
		keys["handle"] = true
	}
	if why := s.applyBlog(ctx, b, in, keys); why != "" {
		writeErr(w, http.StatusBadRequest, why)
		return
	}
	if b.Handle == "" {
		b.Handle = s.suggestHandle(ctx, b.Title, 0)
	}
	if err := s.store.CreateBlog(ctx, uid, b); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "Somebody already has that address; pick another")
			return
		}
		s.fail(w, err, "create blog")
		return
	}
	if in.AboutPage && strings.TrimSpace(in.About) != "" {
		p := &store.Post{BlogID: b.ID, Kind: "page", Title: aboutTitle(b.Language), Slug: "about", Body: strings.TrimSpace(in.About),
			Status: store.StatusPublished, PublishedAt: store.Now(), InMenu: true, Comments: false}
		_ = s.store.CreatePost(ctx, uid, p)
	}
	writeJSON(w, http.StatusCreated, s.withURL(b))
}

func aboutTitle(lang string) string {
	switch langKey(lang) {
	case "bs":
		return "O meni"
	case "de":
		return "Über mich"
	}
	return "About"
}

func (s *Server) handleUpdateBlog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.store.GetBlog(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "blog")
		return
	}
	var in blogIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if why := s.applyBlog(ctx, b, in, keys); why != "" {
		writeErr(w, http.StatusBadRequest, why)
		return
	}
	if err := s.store.UpdateBlog(ctx, uid, b); err != nil {
		if err == store.ErrConflict {
			writeErr(w, http.StatusConflict, "That address or web address is taken")
			return
		}
		s.fail(w, err, "save blog")
		return
	}
	b, _ = s.store.GetBlog(ctx, uid, b.ID)
	writeJSON(w, http.StatusOK, s.withURL(b))
}

func (s *Server) handleDeleteBlog(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteBlog(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete blog")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
