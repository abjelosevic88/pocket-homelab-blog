package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

// blogCache keeps the blogs a request touches.
type blogCache map[int64]*store.Blog

func (s *Server) blogOf(ctx context.Context, uid int64, id int64, bc blogCache) *store.Blog {
	if b, ok := bc[id]; ok {
		return b
	}
	b, err := s.store.GetBlog(ctx, uid, id)
	if err != nil {
		return nil
	}
	bc[id] = b
	return b
}

// dress fills a post's addresses for the writer.
func (s *Server) dress(ctx context.Context, uid int64, p *store.Post, bc blogCache) {
	if b := s.blogOf(ctx, uid, p.BlogID, bc); b != nil {
		p.URL = s.postAbs(b, p)
		if p.Status != store.StatusPublished {
			p.URL += "?preview=" + p.PreviewKey
		}
		if p.Cover != "" {
			p.CoverURL = s.mediaAbs(b, p.Cover)
		}
	}
}

func (s *Server) handleListPosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := r.URL.Query()
	f := store.PostFilter{BlogID: int64(queryInt(r, "blog")), Kind: q.Get("kind"), Status: q.Get("status"), Tag: q.Get("tag"), Q: q.Get("q"),
		Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset"), Sort: q.Get("sort")}
	if f.Kind != "" && f.Kind != "post" && f.Kind != "page" {
		writeErr(w, http.StatusBadRequest, "kind is post or page")
		return
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 200
	}
	ps, total, err := s.store.ListPosts(ctx, uid, f)
	if err != nil {
		s.fail(w, err, "posts")
		return
	}
	bc := blogCache{}
	for i := range ps {
		s.dress(ctx, uid, &ps[i], bc)
		if ps[i].Excerpt == "" {
			if full, err := s.store.GetPost(ctx, uid, ps[i].ID); err == nil {
				ps[i].Excerpt = clip(plainText(full.Body), 160)
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": ps, "total": total})
}

func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPost(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "post")
		return
	}
	s.dress(ctx, uid, p, blogCache{})
	writeJSON(w, http.StatusOK, p)
}

type postIn struct {
	BlogID         int64  `json:"blog_id"`
	Kind           string `json:"kind"`
	Title          string `json:"title"`
	Slug           string `json:"slug"`
	Body           string `json:"body"`
	Excerpt        string `json:"excerpt"`
	Cover          string `json:"cover"`
	CoverAlt       string `json:"cover_alt"`
	Tags           any    `json:"tags"` // "a, b" or ["a","b"]
	Featured       bool   `json:"featured"`
	InMenu         bool   `json:"in_menu"`
	MenuOrder      int    `json:"menu_order"`
	Comments       *bool  `json:"comments"`
	SEODescription string `json:"seo_description"`
	PublishedAt    string `json:"published_at"` // change the date a published post shows
	Source         string `json:"source"`
	SourceRef      string `json:"source_ref"`
	// create only: publish right away ("now") or at a time (RFC 3339)
	Publish string `json:"publish"`
}

func tagsString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []any:
		var out []string
		for _, x := range t {
			out = append(out, fmt.Sprint(x))
		}
		return strings.Join(out, ", ")
	}
	return ""
}

func (s *Server) applyPost(ctx context.Context, uid int64, p *store.Post, in postIn, keys map[string]bool) string {
	set := func(k string) bool { return keys == nil || keys[k] }
	if set("kind") && in.Kind != "" {
		if in.Kind != "post" && in.Kind != "page" {
			return "A post or a page"
		}
		p.Kind = in.Kind
	}
	if set("title") {
		p.Title = clip(strings.TrimSpace(in.Title), 200)
	}
	if set("slug") {
		p.Slug = store.Slugify(in.Slug, 80)
	}
	if set("body") {
		p.Body = in.Body
	}
	if set("excerpt") {
		p.Excerpt = clip(strings.TrimSpace(in.Excerpt), 400)
	}
	if set("cover") {
		c := strings.TrimSpace(in.Cover)
		c = strings.TrimPrefix(c, "/media/")
		if c != "" && !strings.HasPrefix(c, "https://") {
			if m, err := s.store.MediaByFile(ctx, c); err != nil || m.UserID != uid {
				return "That picture is not in your pictures"
			}
		}
		p.Cover = c
	}
	if set("cover_alt") {
		p.CoverAlt = clip(strings.TrimSpace(in.CoverAlt), 300)
	}
	if set("tags") {
		p.Tags = tagsString(in.Tags)
	}
	if set("featured") {
		p.Featured = in.Featured
	}
	if set("in_menu") {
		p.InMenu = in.InMenu
	}
	if set("menu_order") {
		p.MenuOrder = in.MenuOrder
	}
	if set("comments") && in.Comments != nil {
		p.Comments = *in.Comments
	}
	if set("seo_description") {
		p.SEODescription = clip(strings.TrimSpace(in.SEODescription), 300)
	}
	if set("published_at") && in.PublishedAt != "" && p.Status == store.StatusPublished {
		t, err := parseWhen(in.PublishedAt)
		if err != nil {
			return "The date is like 2026-11-05 or 2026-11-05T09:30"
		}
		p.PublishedAt = t.UTC().Format(time.RFC3339Nano)
	}
	if len(p.Body) > 2<<20 {
		return "The post is too long"
	}
	return ""
}

// parseWhen reads RFC 3339, "2026-11-05T09:30" (local) or "2026-11-05".
func parseWhen(v string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		return t, nil
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", v, time.Local); err == nil {
		return t, nil
	}
	return time.ParseInLocation("2006-01-02", v, time.Local)
}

// defaultBlog is the blog to write in when none was named: the person's first.
func (s *Server) defaultBlog(ctx context.Context, uid, id int64) (*store.Blog, error) {
	if id > 0 {
		return s.store.GetBlog(ctx, uid, id)
	}
	bs, err := s.store.ListBlogs(ctx, uid)
	if err != nil {
		return nil, err
	}
	if len(bs) == 0 {
		return nil, store.ErrNotFound
	}
	return &bs[0], nil
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in postIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	b, err := s.defaultBlog(ctx, uid, in.BlogID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "make a blog first (or name one of yours in blog_id)")
		return
	}
	p := &store.Post{BlogID: b.ID, Kind: "post", Status: store.StatusDraft, Comments: true, Source: strings.TrimSpace(in.Source), SourceRef: strings.TrimSpace(in.SourceRef)}
	if app := linkApp(ctx); app != "" {
		p.Source = app // another app's drafts are marked as its own
	}
	if why := s.applyPost(ctx, uid, p, in, nil); why != "" {
		writeErr(w, http.StatusBadRequest, why)
		return
	}
	if p.SourceRef != "" {
		if prev, err := s.store.PostBySource(ctx, uid, b.ID, p.Source, p.SourceRef); err == nil {
			s.dress(ctx, uid, prev, blogCache{})
			writeJSON(w, http.StatusOK, prev)
			return
		}
	}
	if err := s.store.CreatePost(ctx, uid, p); err != nil {
		s.fail(w, err, "create post")
		return
	}
	if in.Publish != "" {
		if why := s.publishPost(ctx, uid, b, p, in.Publish); why != "" {
			writeErr(w, http.StatusBadRequest, why)
			return
		}
	}
	s.dress(ctx, uid, p, blogCache{b.ID: b})
	writeJSON(w, http.StatusCreated, p)
}

// mayTouch: an app acting through a link may change only its own posts.
func mayTouch(ctx context.Context, p *store.Post) bool {
	app := linkApp(ctx)
	return app == "" || p.Source == app
}

func (s *Server) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPost(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "post")
		return
	}
	if !mayTouch(ctx, p) {
		writeErr(w, http.StatusForbidden, "this app may change only the posts it started")
		return
	}
	var in postIn
	keys, err := decodeKeys(r, &in)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if why := s.applyPost(ctx, uid, p, in, keys); why != "" {
		writeErr(w, http.StatusBadRequest, why)
		return
	}
	if err := s.store.UpdatePost(ctx, uid, p); err != nil {
		s.fail(w, err, "save post")
		return
	}
	p, _ = s.store.GetPost(ctx, uid, p.ID)
	s.dress(ctx, uid, p, blogCache{})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeletePost(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete post")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// publishPost puts a post on its blog now ("now" / "") or at a later time.
func (s *Server) publishPost(ctx context.Context, uid int64, b *store.Blog, p *store.Post, when string) string {
	if strings.TrimSpace(p.Title) == "" && p.Kind == "post" {
		return "Give the post a title first"
	}
	if strings.TrimSpace(p.Body) == "" {
		return "Write something first"
	}
	now := time.Now()
	at := now
	if when != "" && when != "now" {
		t, err := parseWhen(when)
		if err != nil {
			return "The time is like 2026-11-05T09:30"
		}
		at = t
	}
	if at.After(now.Add(time.Minute)) {
		p.Status, p.PublishAt = store.StatusScheduled, at.UTC().Format(time.RFC3339Nano)
	} else {
		was := p.Status
		p.Status, p.PublishAt = store.StatusPublished, ""
		if p.PublishedAt == "" || was != store.StatusPublished {
			p.PublishedAt = now.UTC().Format(time.RFC3339Nano)
		}
	}
	if p.Excerpt == "" {
		p.Excerpt = ""
	}
	if err := s.store.UpdatePost(ctx, uid, p); err != nil {
		return err.Error()
	}
	if p.Status == store.StatusPublished {
		s.postPublished(ctx, b, p)
	}
	return ""
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPost(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "post")
		return
	}
	if !mayTouch(ctx, p) {
		writeErr(w, http.StatusForbidden, "this app may publish only the posts it started")
		return
	}
	var in struct {
		At string `json:"at"`
	}
	_ = decode(r, &in)
	b, err := s.store.GetBlog(ctx, uid, p.BlogID)
	if err != nil {
		s.fail(w, err, "blog")
		return
	}
	if why := s.publishPost(ctx, uid, b, p, in.At); why != "" {
		writeErr(w, http.StatusBadRequest, why)
		return
	}
	p, _ = s.store.GetPost(ctx, uid, p.ID)
	s.dress(ctx, uid, p, blogCache{b.ID: b})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleUnpublish(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPost(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "post")
		return
	}
	if !mayTouch(ctx, p) {
		writeErr(w, http.StatusForbidden, "this app may change only the posts it started")
		return
	}
	p.Status, p.PublishAt = store.StatusDraft, ""
	if err := s.store.UpdatePost(ctx, uid, p); err != nil {
		s.fail(w, err, "save post")
		return
	}
	p, _ = s.store.GetPost(ctx, uid, p.ID)
	s.dress(ctx, uid, p, blogCache{})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDuplicatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPost(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "post")
		return
	}
	cp := *p
	cp.ID, cp.Status, cp.PublishAt, cp.PublishedAt, cp.PreviewKey, cp.Views, cp.Source, cp.SourceRef = 0, store.StatusDraft, "", "", "", 0, "", ""
	cp.Title = strings.TrimSpace(cp.Title + " (copy)")
	cp.Slug = ""
	if err := s.store.CreatePost(ctx, uid, &cp); err != nil {
		s.fail(w, err, "copy post")
		return
	}
	s.dress(ctx, uid, &cp, blogCache{})
	writeJSON(w, http.StatusCreated, cp)
}

// handleNewPreviewKey stops the old share-a-draft link working.
func (s *Server) handleNewPreviewKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	p, err := s.store.GetPost(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "post")
		return
	}
	p.PreviewKey = store.RandomKey(12)
	if err := s.store.UpdatePost(ctx, uid, p); err != nil {
		s.fail(w, err, "save post")
		return
	}
	s.dress(ctx, uid, p, blogCache{})
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.defaultBlog(ctx, uid, int64(queryInt(r, "blog")))
	if err != nil {
		writeJSON(w, http.StatusOK, []store.TagCount{})
		return
	}
	ts, err := s.store.Tags(ctx, uid, b.ID, false)
	if err != nil {
		s.fail(w, err, "tags")
		return
	}
	writeJSON(w, http.StatusOK, ts)
}

// publishDue sends scheduled posts out when their time has come.
func (s *Server) publishDue(ctx context.Context) {
	ps, err := s.store.PublishDue(ctx)
	if err != nil {
		s.log.Warn("publish scheduled posts", "err", err)
	}
	for i := range ps {
		p := &ps[i]
		b, err := s.store.GetBlog(ctx, p.UserID, p.BlogID)
		if err != nil {
			continue
		}
		s.log.Info("scheduled post published", "post", p.ID, "blog", b.Handle)
		s.postPublished(ctx, b, p)
		st, _ := s.store.GetSettings(ctx, p.UserID)
		if st.NotifyPublished {
			s.push(ctx, p.UserID, st, "Your post is live", fmt.Sprintf("“%s” is on %s now.", firstNonEmpty(p.Title, "Untitled"), b.Title), s.postAbs(b, p))
		}
	}
}

// postPublished tells the rest of the box (Pocket Flows, Day, Alerts…).
func (s *Server) postPublished(ctx context.Context, b *store.Blog, p *store.Post) {
	s.publish(ctx, p.UserID, "blog.post_published", fmt.Sprintf("post:%d", p.ID), map[string]any{
		"post_id": p.ID, "blog_id": b.ID, "blog": b.Title, "title": p.Title, "url": s.postAbs(b, p), "tags": store.SplitTags(p.Tags),
		"excerpt": firstNonEmpty(p.Excerpt, autoExcerpt(p.Body)), "words": p.Words, "published_at": p.PublishedAt, "visibility": b.Visibility,
	})
}
