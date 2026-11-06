package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

func (s *Server) handleListComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := r.URL.Query().Get("status")
	if st != "" && !oneOf(st, []string{"pending", "approved", "spam"}) {
		writeErr(w, http.StatusBadRequest, "status is pending, approved or spam")
		return
	}
	cs, err := s.store.ListComments(ctx, uid, store.CommentFilter{BlogID: int64(queryInt(r, "blog")), PostID: int64(queryInt(r, "post")), Status: st, Limit: 500})
	if err != nil {
		s.fail(w, err, "comments")
		return
	}
	bc := blogCache{}
	posts := map[int64]*store.Post{}
	for i := range cs {
		p, ok := posts[cs[i].PostID]
		if !ok {
			p, _ = s.store.GetPost(ctx, uid, cs[i].PostID)
			posts[cs[i].PostID] = p
		}
		if p != nil {
			if b := s.blogOf(ctx, uid, p.BlogID, bc); b != nil {
				cs[i].PostURL = s.postAbs(b, p) + fmt.Sprintf("#c%d", cs[i].ID)
			}
		}
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *Server) handleUpdateComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var in struct {
		Status string `json:"status"`
	}
	if err := decode(r, &in); err != nil || !oneOf(in.Status, []string{"pending", "approved", "spam"}) {
		writeErr(w, http.StatusBadRequest, "status is pending, approved or spam")
		return
	}
	if err := s.store.SetCommentStatus(ctx, actorID(ctx), idParam(r, "id"), in.Status); err != nil {
		s.fail(w, err, "comment")
		return
	}
	c, _ := s.store.GetComment(ctx, actorID(ctx), idParam(r, "id"))
	writeJSON(w, http.StatusOK, c)
}

func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteComment(r.Context(), actorID(r.Context()), idParam(r, "id")); err != nil {
		s.fail(w, err, "comment")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleReplyComment: the writer answers a reader (approving their comment).
func (s *Server) handleReplyComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	c, err := s.store.GetComment(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "comment")
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.Body) == "" {
		writeErr(w, http.StatusBadRequest, "write your answer")
		return
	}
	p, err := s.store.GetPost(ctx, uid, c.PostID)
	if err != nil {
		s.fail(w, err, "post")
		return
	}
	b, _ := s.store.GetBlog(ctx, uid, p.BlogID)
	name := userFrom(ctx).Name
	if b != nil && b.AuthorName != "" {
		name = b.AuthorName
	}
	if c.Status != "approved" {
		_ = s.store.SetCommentStatus(ctx, uid, c.ID, "approved")
	}
	parent := c.ID
	if c.ParentID != nil {
		parent = *c.ParentID
	}
	reply := &store.Comment{UserID: uid, PostID: c.PostID, ParentID: &parent, Name: firstNonEmpty(name, "Author"), Body: strings.TrimSpace(in.Body), Status: "approved", ByAuthor: true}
	if err := s.store.AddComment(ctx, reply); err != nil {
		s.fail(w, err, "reply")
		return
	}
	writeJSON(w, http.StatusCreated, reply)
}

// commentArrived tells the writer, and the box, about a new comment.
func (s *Server) commentArrived(ctx context.Context, b *store.Blog, p *store.Post, c *store.Comment) {
	s.publish(ctx, p.UserID, "blog.comment_received", fmt.Sprintf("comment:%d", c.ID), map[string]any{
		"comment_id": c.ID, "post_id": p.ID, "post": p.Title, "blog": b.Title, "name": c.Name, "excerpt": clip(c.Body, 200),
		"status": c.Status, "url": s.postAbs(b, p) + fmt.Sprintf("#c%d", c.ID),
	})
	st, _ := s.store.GetSettings(ctx, p.UserID)
	if !st.NotifyComment || st.NotifyURL == "" {
		return
	}
	title := fmt.Sprintf("%s commented on “%s”", c.Name, firstNonEmpty(p.Title, "Untitled"))
	body := clip(c.Body, 300)
	if c.Status == "pending" {
		body += "\n\nIt waits for your OK."
	}
	s.push(ctx, p.UserID, st, title, body, strings.TrimRight(s.cfg.BaseURL, "/")+"/comments")
}

// ---- numbers ----

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	days := queryInt(r, "days")
	if days <= 0 || days > 730 {
		days = 30
	}
	blog := int64(queryInt(r, "blog"))
	from := store.AddDays(store.Today(), -(days - 1))
	byDay, err := s.store.ViewsByDay(ctx, uid, blog, from)
	if err != nil {
		s.fail(w, err, "stats")
		return
	}
	// every day, also the quiet ones
	have := map[string]int{}
	total := 0
	for _, d := range byDay {
		have[d.Day] = d.Count
		total += d.Count
	}
	series := make([]store.DayCount, 0, days)
	for i := 0; i < days; i++ {
		d := store.AddDays(from, i)
		series = append(series, store.DayCount{Day: d, Count: have[d]})
	}
	prevFrom := store.AddDays(from, -days)
	prevDays, _ := s.store.ViewsByDay(ctx, uid, blog, prevFrom)
	prev := 0
	for _, d := range prevDays {
		if d.Day < from {
			prev += d.Count
		}
	}
	top, _ := s.store.TopPosts(ctx, uid, blog, from, 10)
	bc := blogCache{}
	for i := range top {
		if p, err := s.store.GetPost(ctx, uid, top[i].ID); err == nil {
			if b := s.blogOf(ctx, uid, p.BlogID, bc); b != nil {
				top[i].URL = s.postAbs(b, p)
			}
		}
	}
	cs, _ := s.store.ListComments(ctx, uid, store.CommentFilter{BlogID: blog})
	comments := 0
	for _, c := range cs {
		if c.CreatedAt >= from && !c.ByAuthor {
			comments++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"days": days, "from": from, "views": total, "previous": prev, "by_day": series, "top": top, "comments": comments})
}

// handleSummary is the short version for dashboards and Pocket AI.
func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	bs, err := s.store.ListBlogs(ctx, uid)
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	type blogSum struct {
		ID      int64  `json:"id"`
		Title   string `json:"title"`
		URL     string `json:"url"`
		Posts   int    `json:"posts"`
		Drafts  int    `json:"drafts"`
		Pending int    `json:"pending_comments"`
	}
	out := map[string]any{}
	sums := []blogSum{}
	for i := range bs {
		b := &bs[i]
		sums = append(sums, blogSum{b.ID, b.Title, s.blogAbs(b), b.Posts, b.Drafts, b.Pending})
	}
	out["blogs"] = sums
	week := store.AddDays(store.Today(), -6)
	days, _ := s.store.ViewsByDay(ctx, uid, 0, week)
	v := 0
	for _, d := range days {
		v += d.Count
	}
	out["views_7d"] = v
	out["pending_comments"] = s.store.PendingComments(ctx, uid)
	bc := blogCache{}
	brief := func(ps []store.Post) []map[string]any {
		l := []map[string]any{}
		for i := range ps {
			s.dress(ctx, uid, &ps[i], bc)
			l = append(l, map[string]any{"id": ps[i].ID, "title": firstNonEmpty(ps[i].Title, "Untitled"), "status": ps[i].Status, "url": ps[i].URL,
				"published_at": ps[i].PublishedAt, "publish_at": ps[i].PublishAt, "updated_at": ps[i].UpdatedAt, "views": ps[i].Views, "blog_id": ps[i].BlogID})
		}
		return l
	}
	drafts, nd, _ := s.store.ListPosts(ctx, uid, store.PostFilter{Status: store.StatusDraft, Limit: 5, Sort: "updated"})
	sched, _, _ := s.store.ListPosts(ctx, uid, store.PostFilter{Status: store.StatusScheduled, Limit: 5})
	recent, np, _ := s.store.ListPosts(ctx, uid, store.PostFilter{Kind: "post", Status: store.StatusPublished, Limit: 5})
	out["drafts"], out["drafts_total"] = brief(drafts), nd
	out["scheduled"] = brief(sched)
	out["recent"], out["published_total"] = brief(recent), np
	out["url"] = strings.TrimRight(s.cfg.BaseURL, "/")
	writeJSON(w, http.StatusOK, out)
}

// handleDay is what happened on the blog on a day (Pocket Day's journal).
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chiParam(r, "date")
	if _, err := time.Parse("2006-01-02", date); err != nil {
		writeErr(w, http.StatusBadRequest, "date is YYYY-MM-DD")
		return
	}
	ps, _, err := s.store.ListPosts(ctx, uid, store.PostFilter{Day: date})
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	bc := blogCache{}
	published, written := []map[string]any{}, []map[string]any{}
	d, _ := time.ParseInLocation("2006-01-02", date, time.Local)
	from, to := d.UTC().Format(time.RFC3339Nano), d.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
	for i := range ps {
		p := &ps[i]
		s.dress(ctx, uid, p, bc)
		row := map[string]any{"id": p.ID, "title": firstNonEmpty(p.Title, "Untitled"), "kind": p.Kind, "status": p.Status, "url": p.URL, "words": p.Words,
			"excerpt": p.Excerpt, "tags": store.SplitTags(p.Tags)}
		if b := bc[p.BlogID]; b != nil {
			row["blog"] = b.Title
		}
		if p.PublishedAt >= from && p.PublishedAt < to && p.Status == store.StatusPublished {
			row["time"] = localClock(p.PublishedAt)
			published = append(published, row)
		} else {
			row["time"] = localClock(p.CreatedAt)
			written = append(written, row)
		}
	}
	views, _ := s.store.ViewsByDay(ctx, uid, 0, date)
	n := 0
	for _, v := range views {
		if v.Day == date {
			n = v.Count
		}
	}
	cs, _ := s.store.ListComments(ctx, uid, store.CommentFilter{})
	comments := 0
	for _, c := range cs {
		if c.CreatedAt >= from && c.CreatedAt < to && !c.ByAuthor {
			comments++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": date, "published": published, "written": written, "views": n, "comments": comments})
}

func localClock(stamp string) string {
	t, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return ""
	}
	return t.Local().Format("15:04")
}

// handleSearch finds the person's posts (Pocket Search, Pocket AI).
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	out := []map[string]any{}
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"query": q, "results": out})
		return
	}
	ps, _, err := s.store.ListPosts(ctx, uid, store.PostFilter{Q: q, Limit: limit, Body: true})
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	bc := blogCache{}
	for i := range ps {
		p := &ps[i]
		s.dress(ctx, uid, p, bc)
		out = append(out, map[string]any{"id": p.ID, "title": firstNonEmpty(p.Title, "Untitled"), "snippet": snippet(plainText(p.Body), q),
			"status": p.Status, "kind": p.Kind, "date": firstNonEmpty(p.PublishedAt, p.UpdatedAt), "url": p.URL, "tags": store.SplitTags(p.Tags),
			"link": strings.TrimRight(s.cfg.BaseURL, "/") + fmt.Sprintf("/posts/%d", p.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "results": out})
}

// snippet is the part of text around the first word of q.
func snippet(text, q string) string {
	words := strings.Fields(store.Fold(q))
	if len(words) == 0 {
		return clip(text, 180)
	}
	folded := []rune(store.Fold(text))
	runes := []rune(text)
	idx := strings.Index(string(folded), words[0])
	if idx < 0 || len(folded) != len(runes) {
		return clip(text, 180)
	}
	at := len([]rune(string(folded)[:idx]))
	start := at - 60
	if start < 0 {
		start = 0
	}
	out := clip(string(runes[start:]), 180)
	if start > 0 {
		out = "…" + out
	}
	return out
}
