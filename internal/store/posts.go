package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

const postCols = `id, user_id, blog_id, kind, title, slug, body, excerpt, cover, cover_alt, tags, status, publish_at, published_at,
	featured, in_menu, menu_order, comments, seo_description, preview_key, words, views, source, source_ref, created_at, updated_at`

func scanPost(sc interface{ Scan(...any) error }) (*Post, error) {
	var p Post
	var featured, inMenu, comments int
	if err := sc.Scan(&p.ID, &p.UserID, &p.BlogID, &p.Kind, &p.Title, &p.Slug, &p.Body, &p.Excerpt, &p.Cover, &p.CoverAlt, &p.Tags,
		&p.Status, &p.PublishAt, &p.PublishedAt, &featured, &inMenu, &p.MenuOrder, &comments, &p.SEODescription, &p.PreviewKey,
		&p.Words, &p.Views, &p.Source, &p.SourceRef, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.Featured, p.InMenu, p.Comments = featured == 1, inMenu == 1, comments == 1
	return &p, nil
}

// CountWords counts the words of Markdown text (roughly: what a reader reads).
func CountWords(md string) int {
	n, in := 0, false
	for _, r := range md {
		w := unicode.IsLetter(r) || unicode.IsDigit(r)
		if w && !in {
			n++
		}
		in = w
	}
	return n
}

// ReadingMinutes is "4 min read" for so many words.
func ReadingMinutes(words int) int {
	m := (words + 199) / 200
	if m < 1 {
		m = 1
	}
	return m
}

func (p *Post) prepare() {
	p.Title = strings.TrimSpace(p.Title)
	p.Tags = JoinTags(p.Tags)
	p.Words = CountWords(p.Body)
	if p.Kind != "page" {
		p.Kind = "post"
	}
	if p.Status == "" {
		p.Status = StatusDraft
	}
}

func searchText(p *Post) string {
	return Fold(p.Title + " " + p.Excerpt + " " + p.Tags + " " + p.Body)
}

// uniqueSlug finds a free address part in the blog: "walk", "walk-2", …
func (s *Store) uniqueSlug(ctx context.Context, blogID int64, want string, except int64) string {
	if want == "" {
		want = "post"
	}
	slug := want
	for i := 2; i < 1000; i++ {
		var n int
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM posts WHERE blog_id = ? AND slug = ? AND id <> ?`, blogID, slug, except).Scan(&n)
		if n == 0 && !reservedSlug(slug) {
			return slug
		}
		slug = fmt.Sprintf("%s-%d", want, i)
	}
	return fmt.Sprintf("%s-%d", want, time.Now().Unix())
}

// reservedSlug: addresses the blog pages use themselves.
func reservedSlug(s string) bool {
	switch s {
	case "tag", "tags", "archive", "feed.xml", "rss.xml", "sitemap.xml", "robots.txt", "search", "page", "media", "comments", "assets", "style.css":
		return true
	}
	return false
}

// CreatePost adds a post or page; its address part is made unique in the blog.
func (s *Store) CreatePost(ctx context.Context, uid int64, p *Post) error {
	p.prepare()
	p.Slug = s.uniqueSlug(ctx, p.BlogID, firstSlug(p.Slug, p.Title), 0)
	var owner int64
	if err := s.DB.QueryRowContext(ctx, `SELECT user_id FROM blogs WHERE id = ?`, p.BlogID).Scan(&owner); err != nil || owner != uid {
		return ErrNotFound
	}
	if p.PreviewKey == "" {
		p.PreviewKey = RandomKey(12)
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO posts (user_id, blog_id, kind, title, slug, body, excerpt, cover, cover_alt, tags, status,
		publish_at, published_at, featured, in_menu, menu_order, comments, seo_description, preview_key, words, views, search, source, source_ref)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		uid, p.BlogID, p.Kind, p.Title, p.Slug, p.Body, p.Excerpt, p.Cover, p.CoverAlt, p.Tags, p.Status, p.PublishAt, p.PublishedAt,
		boolInt(p.Featured), boolInt(p.InMenu), p.MenuOrder, boolInt(p.Comments), p.SEODescription, p.PreviewKey, p.Words, p.Views,
		searchText(p), p.Source, p.SourceRef)
	if err != nil {
		return conflict(err)
	}
	p.ID, _ = res.LastInsertId()
	got, err := s.GetPost(ctx, uid, p.ID)
	if err == nil {
		*p = *got
	}
	return err
}

func firstSlug(slug, title string) string {
	if s := Slugify(slug, 80); s != "" {
		return s
	}
	return Slugify(title, 80)
}

// UpdatePost saves a post (its address part stays unique in the blog).
func (s *Store) UpdatePost(ctx context.Context, uid int64, p *Post) error {
	p.prepare()
	p.Slug = s.uniqueSlug(ctx, p.BlogID, firstSlug(p.Slug, p.Title), p.ID)
	res, err := s.DB.ExecContext(ctx, `UPDATE posts SET kind=?, title=?, slug=?, body=?, excerpt=?, cover=?, cover_alt=?, tags=?, status=?,
		publish_at=?, published_at=?, featured=?, in_menu=?, menu_order=?, comments=?, seo_description=?, preview_key=?, words=?, search=?,
		updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=? AND user_id=?`,
		p.Kind, p.Title, p.Slug, p.Body, p.Excerpt, p.Cover, p.CoverAlt, p.Tags, p.Status, p.PublishAt, p.PublishedAt,
		boolInt(p.Featured), boolInt(p.InMenu), p.MenuOrder, boolInt(p.Comments), p.SEODescription, p.PreviewKey, p.Words, searchText(p), p.ID, uid)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPost returns one of a person's posts.
func (s *Store) GetPost(ctx context.Context, uid, id int64) (*Post, error) {
	p, err := scanPost(s.DB.QueryRowContext(ctx, `SELECT `+postCols+` FROM posts WHERE id = ? AND user_id = ?`, id, uid))
	if err != nil {
		return nil, notFound(err)
	}
	s.commentCounts(ctx, p)
	return p, nil
}

// PostBySlug finds a post or page of a blog by its address part.
func (s *Store) PostBySlug(ctx context.Context, blogID int64, slug string) (*Post, error) {
	p, err := scanPost(s.DB.QueryRowContext(ctx, `SELECT `+postCols+` FROM posts WHERE blog_id = ? AND slug = ?`, blogID, slug))
	return p, notFound(err)
}

// PostBySource finds what came from another app (or an import) before.
func (s *Store) PostBySource(ctx context.Context, uid, blogID int64, source, ref string) (*Post, error) {
	p, err := scanPost(s.DB.QueryRowContext(ctx, `SELECT `+postCols+` FROM posts WHERE user_id = ? AND blog_id = ? AND source = ? AND source_ref = ?`, uid, blogID, source, ref))
	return p, notFound(err)
}

// DeletePost removes a post with its comments and readers.
func (s *Store) DeletePost(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM posts WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) commentCounts(ctx context.Context, p *Post) {
	_ = s.DB.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN status='approved' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='pending' THEN 1 ELSE 0 END),0) FROM comments WHERE post_id = ?`, p.ID).Scan(&p.CommentsN, &p.PendingN)
}

// PostFilter narrows a list of posts.
type PostFilter struct {
	BlogID int64
	Kind   string // post | page | "" both
	Status string // draft | scheduled | published | "" all
	Tag    string
	Q      string
	Day    string // YYYY-MM-DD (local): published, or written, that day
	Public bool   // only what readers may see now (published, not in the future)
	Limit  int
	Offset int
	Body   bool // include the text (exports)
	Sort   string
}

// ListPosts lists a person's posts (uid > 0) or a blog's public ones (Public, uid 0).
func (s *Store) ListPosts(ctx context.Context, uid int64, f PostFilter) ([]Post, int, error) {
	where := []string{"1=1"}
	args := []any{}
	if uid > 0 {
		where = append(where, "user_id = ?")
		args = append(args, uid)
	}
	if f.BlogID > 0 {
		where = append(where, "blog_id = ?")
		args = append(args, f.BlogID)
	}
	if f.Kind != "" {
		where = append(where, "kind = ?")
		args = append(args, f.Kind)
	}
	if f.Public {
		where = append(where, "status = 'published' AND published_at <= ?")
		args = append(args, Now())
	} else if f.Status != "" {
		where = append(where, "status = ?")
		args = append(args, f.Status)
	}
	if f.Tag != "" {
		where = append(where, "(', ' || tags || ',') LIKE ?")
		args = append(args, "%, "+strings.ToLower(f.Tag)+",%")
	}
	if q := strings.TrimSpace(Fold(f.Q)); q != "" {
		for _, w := range strings.Fields(q) {
			where = append(where, "search LIKE ?")
			args = append(args, "%"+w+"%")
		}
	}
	if f.Day != "" {
		if d, err := time.ParseInLocation("2006-01-02", f.Day, time.Local); err == nil {
			from, to := d.UTC().Format(time.RFC3339Nano), d.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
			where = append(where, "((published_at >= ? AND published_at < ?) OR (created_at >= ? AND created_at < ?))")
			args = append(args, from, to, from, to)
		}
	}
	w := strings.Join(where, " AND ")
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM posts WHERE `+w, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	order := "CASE status WHEN 'draft' THEN 0 WHEN 'scheduled' THEN 1 ELSE 2 END, COALESCE(NULLIF(published_at,''), updated_at) DESC, id DESC"
	switch {
	case f.Public:
		order = "featured DESC, published_at DESC, id DESC"
	case f.Sort == "updated":
		order = "updated_at DESC, id DESC"
	case f.Sort == "oldest":
		order = "id"
	case f.Sort == "views":
		order = "views DESC, id DESC"
	case f.Sort == "menu":
		order = "menu_order, title COLLATE NOCASE"
	}
	q := `SELECT ` + postCols + ` FROM posts WHERE ` + w + ` ORDER BY ` + order
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", f.Limit, f.Offset)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	out := []Post{}
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			rows.Close()
			return nil, 0, err
		}
		if !f.Body {
			p.Body = ""
		}
		out = append(out, *p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	for i := range out {
		s.commentCounts(ctx, &out[i])
	}
	return out, total, nil
}

// MenuPages are a blog's published pages that are listed in its menu.
func (s *Store) MenuPages(ctx context.Context, blogID int64) []Post {
	ps, _, _ := s.ListPosts(ctx, 0, PostFilter{BlogID: blogID, Kind: "page", Public: true})
	out := []Post{}
	for _, p := range ps {
		if p.InMenu {
			out = append(out, p)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].MenuOrder < out[j].MenuOrder })
	return out
}

// Neighbours are the posts published just before and after p.
func (s *Store) Neighbours(ctx context.Context, p *Post) (prev, next *Post) {
	now := Now()
	if x, err := scanPost(s.DB.QueryRowContext(ctx, `SELECT `+postCols+` FROM posts WHERE blog_id = ? AND kind = 'post' AND status = 'published'
		AND published_at <= ? AND (published_at < ? OR (published_at = ? AND id < ?)) ORDER BY published_at DESC, id DESC LIMIT 1`,
		p.BlogID, now, p.PublishedAt, p.PublishedAt, p.ID)); err == nil {
		prev = x
	}
	if x, err := scanPost(s.DB.QueryRowContext(ctx, `SELECT `+postCols+` FROM posts WHERE blog_id = ? AND kind = 'post' AND status = 'published'
		AND published_at <= ? AND (published_at > ? OR (published_at = ? AND id > ?)) ORDER BY published_at, id LIMIT 1`,
		p.BlogID, now, p.PublishedAt, p.PublishedAt, p.ID)); err == nil {
		next = x
	}
	return prev, next
}

// TagCount is a tag with how many posts have it.
type TagCount struct {
	Tag   string `json:"tag"`
	Count int    `json:"count"`
}

// Tags counts the tags of a blog's posts (public: only what readers see).
func (s *Store) Tags(ctx context.Context, uid, blogID int64, public bool) ([]TagCount, error) {
	q := `SELECT tags FROM posts WHERE blog_id = ? AND kind = 'post' AND tags <> ''`
	args := []any{blogID}
	if uid > 0 {
		q += ` AND user_id = ?`
		args = append(args, uid)
	}
	if public {
		q += ` AND status = 'published' AND published_at <= ?`
		args = append(args, Now())
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := map[string]int{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		for _, x := range SplitTags(t) {
			counts[x]++
		}
	}
	out := []TagCount{}
	for t, n := range counts {
		out = append(out, TagCount{t, n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag < out[j].Tag
	})
	return out, rows.Err()
}

// PublishDue puts every scheduled post whose time has come on its blog and
// returns them.
func (s *Store) PublishDue(ctx context.Context) ([]Post, error) {
	now := Now()
	rows, err := s.DB.QueryContext(ctx, `SELECT `+postCols+` FROM posts WHERE status = 'scheduled' AND publish_at <> '' AND publish_at <= ?`, now)
	if err != nil {
		return nil, err
	}
	var due []Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		due = append(due, *p)
	}
	rows.Close()
	out := []Post{}
	for _, p := range due {
		res, err := s.DB.ExecContext(ctx, `UPDATE posts SET status = 'published', published_at = publish_at, updated_at = ? WHERE id = ? AND status = 'scheduled'`, now, p.ID)
		if err != nil {
			return out, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			p.Status, p.PublishedAt = StatusPublished, p.PublishAt
			out = append(out, p)
		}
	}
	return out, nil
}

// ---- readers ----

// AddView counts one reader of a post today.
func (s *Store) AddView(ctx context.Context, p *Post) error {
	day := Today()
	if _, err := s.DB.ExecContext(ctx, `INSERT INTO views (user_id, post_id, day, views) VALUES (?, ?, ?, 1)
		ON CONFLICT(post_id, day) DO UPDATE SET views = views + 1`, p.UserID, p.ID, day); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, `UPDATE posts SET views = views + 1 WHERE id = ?`, p.ID)
	return err
}

// DayCount is a number for one day.
type DayCount struct {
	Day   string `json:"day"`
	Count int    `json:"count"`
}

// ViewsByDay are a person's readers per day since a day (one blog, or all with 0).
func (s *Store) ViewsByDay(ctx context.Context, uid, blogID int64, from string) ([]DayCount, error) {
	q := `SELECT v.day, SUM(v.views) FROM views v JOIN posts p ON p.id = v.post_id WHERE v.user_id = ? AND v.day >= ?`
	args := []any{uid, from}
	if blogID > 0 {
		q += ` AND p.blog_id = ?`
		args = append(args, blogID)
	}
	rows, err := s.DB.QueryContext(ctx, q+` GROUP BY v.day ORDER BY v.day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DayCount{}
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Day, &d.Count); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// PostViews is a post with its readers in a period.
type PostViews struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
	Views int    `json:"views"`
	URL   string `json:"url,omitempty"`
}

// TopPosts are the most read posts since a day.
func (s *Store) TopPosts(ctx context.Context, uid, blogID int64, from string, limit int) ([]PostViews, error) {
	q := `SELECT p.id, p.title, p.slug, SUM(v.views) n FROM views v JOIN posts p ON p.id = v.post_id WHERE v.user_id = ? AND v.day >= ?`
	args := []any{uid, from}
	if blogID > 0 {
		q += ` AND p.blog_id = ?`
		args = append(args, blogID)
	}
	rows, err := s.DB.QueryContext(ctx, q+fmt.Sprintf(` GROUP BY p.id ORDER BY n DESC LIMIT %d`, limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PostViews{}
	for rows.Next() {
		var p PostViews
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.Views); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// SetViews puts a day's count for a post (demo data and imports).
func (s *Store) SetViews(ctx context.Context, uid, postID int64, day string, n int) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO views (user_id, post_id, day, views) VALUES (?, ?, ?, ?)
		ON CONFLICT(post_id, day) DO UPDATE SET views = excluded.views`, uid, postID, day, n)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE posts SET views = (SELECT COALESCE(SUM(views),0) FROM views WHERE post_id = ?) WHERE id = ?`, postID, postID)
	return err
}

// ErrSlugTaken: never returned to readers, kept for callers that test it.
var ErrSlugTaken = errors.New("address taken")

var _ = sql.ErrNoRows
