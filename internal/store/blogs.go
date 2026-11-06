package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"unicode"
)

const blogCols = `id, user_id, handle, title, tagline, about, author_name, author_bio, avatar, theme, accent, font, language,
	visibility, comments, per_page, show_reading, footer, nav, socials, domain, created_at, updated_at`

func scanBlog(sc interface{ Scan(...any) error }) (*Blog, error) {
	var b Blog
	var nav, socials string
	var reading int
	if err := sc.Scan(&b.ID, &b.UserID, &b.Handle, &b.Title, &b.Tagline, &b.About, &b.AuthorName, &b.AuthorBio, &b.Avatar,
		&b.Theme, &b.Accent, &b.Font, &b.Language, &b.Visibility, &b.Comments, &b.PerPage, &reading, &b.Footer, &nav, &socials,
		&b.Domain, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	b.ShowReading = reading == 1
	_ = json.Unmarshal([]byte(nav), &b.Nav)
	_ = json.Unmarshal([]byte(socials), &b.Socials)
	if b.Nav == nil {
		b.Nav = []NavLink{}
	}
	if b.Socials == nil {
		b.Socials = map[string]string{}
	}
	return &b, nil
}

// Slugify makes a web address part: "Ljeto u Mostaru!" → "ljeto-u-mostaru".
func Slugify(s string, max int) string {
	var b strings.Builder
	dash := false
	for _, r := range Fold(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			// letters without a Latin form (Greek, Chinese…) stay as they are
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
		if max > 0 && b.Len() >= max {
			break
		}
	}
	return strings.Trim(b.String(), "-")
}

var foldMap = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'ą': "a", 'æ': "ae",
	'ç': "c", 'ć': "c", 'č': "c", 'ď': "d", 'đ': "d", 'ð': "d",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ę': "e", 'ě': "e",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ī': "i", 'ł': "l", 'ľ': "l",
	'ñ': "n", 'ń': "n", 'ň': "n", 'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'ō': "o", 'ő': "o", 'œ': "oe",
	'ř': "r", 'ś': "s", 'š': "s", 'ß': "ss", 'ť': "t", 'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ū': "u", 'ů': "u", 'ű': "u",
	'ý': "y", 'ÿ': "y", 'ź': "z", 'ż': "z", 'ž': "z",
	// Serbian Cyrillic
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'ђ': "dj", 'е': "e", 'ж': "z", 'з': "z", 'и': "i", 'ј': "j", 'к': "k",
	'л': "l", 'љ': "lj", 'м': "m", 'н': "n", 'њ': "nj", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'ћ': "c", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "c", 'џ': "dz", 'ш': "s",
}

// Fold lowercases and strips accents (and writes Cyrillic in Latin letters),
// for search and addresses.
func Fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if f, ok := foldMap[r]; ok {
			b.WriteString(f)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ValidHandle: 2–40 of a-z, 0-9 and dashes, not starting or ending with one.
func ValidHandle(h string) bool {
	if len(h) < 2 || len(h) > 40 || h[0] == '-' || h[len(h)-1] == '-' {
		return false
	}
	for _, r := range h {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

func (s *Store) blogArgs(b *Blog) []any {
	nav, _ := json.Marshal(b.Nav)
	if b.Nav == nil {
		nav = []byte("[]")
	}
	socials, _ := json.Marshal(b.Socials)
	if b.Socials == nil {
		socials = []byte("{}")
	}
	return []any{b.Handle, b.Title, b.Tagline, b.About, b.AuthorName, b.AuthorBio, b.Avatar, b.Theme, b.Accent, b.Font, b.Language,
		b.Visibility, b.Comments, b.PerPage, boolInt(b.ShowReading), b.Footer, string(nav), string(socials), strings.ToLower(b.Domain)}
}

// CreateBlog adds a blog for a person (the handle is box-wide unique).
func (s *Store) CreateBlog(ctx context.Context, uid int64, b *Blog) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO blogs (handle, title, tagline, about, author_name, author_bio, avatar, theme, accent, font,
		language, visibility, comments, per_page, show_reading, footer, nav, socials, domain, user_id) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		append(s.blogArgs(b), uid)...)
	if err != nil {
		return conflict(err)
	}
	b.ID, _ = res.LastInsertId()
	b.UserID = uid
	got, err := s.GetBlog(ctx, uid, b.ID)
	if err == nil {
		*b = *got
	}
	return err
}

// UpdateBlog saves a person's blog.
func (s *Store) UpdateBlog(ctx context.Context, uid int64, b *Blog) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE blogs SET handle=?, title=?, tagline=?, about=?, author_name=?, author_bio=?, avatar=?, theme=?,
		accent=?, font=?, language=?, visibility=?, comments=?, per_page=?, show_reading=?, footer=?, nav=?, socials=?, domain=?,
		updated_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id=? AND user_id=?`, append(s.blogArgs(b), b.ID, uid)...)
	if err != nil {
		return conflict(err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetBlog returns one of a person's blogs.
func (s *Store) GetBlog(ctx context.Context, uid, id int64) (*Blog, error) {
	b, err := scanBlog(s.DB.QueryRowContext(ctx, `SELECT `+blogCols+` FROM blogs WHERE id = ? AND user_id = ?`, id, uid))
	return b, notFound(err)
}

// BlogByHandle finds a blog by its address (for readers: anybody's).
func (s *Store) BlogByHandle(ctx context.Context, handle string) (*Blog, error) {
	b, err := scanBlog(s.DB.QueryRowContext(ctx, `SELECT `+blogCols+` FROM blogs WHERE handle = ?`, strings.ToLower(handle)))
	return b, notFound(err)
}

// BlogByDomain finds the blog that has this host name as its own address.
func (s *Store) BlogByDomain(ctx context.Context, host string) (*Blog, error) {
	host = strings.ToLower(strings.TrimPrefix(host, "www."))
	b, err := scanBlog(s.DB.QueryRowContext(ctx, `SELECT `+blogCols+` FROM blogs WHERE domain <> '' AND (domain = ? OR domain = ?)`, host, "www."+host))
	return b, notFound(err)
}

// HandleTaken says whether another blog already has this address.
func (s *Store) HandleTaken(ctx context.Context, handle string, except int64) bool {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM blogs WHERE handle = ? AND id <> ?`, handle, except).Scan(&n)
	return n > 0
}

// ListBlogs lists a person's blogs with their counts.
func (s *Store) ListBlogs(ctx context.Context, uid int64) ([]Blog, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+blogCols+` FROM blogs WHERE user_id = ? ORDER BY id`, uid)
	if err != nil {
		return nil, err
	}
	out := []Blog{}
	for rows.Next() {
		b, err := scanBlog(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		b := &out[i]
		var last sql.NullString
		_ = s.DB.QueryRowContext(ctx, `SELECT
			COALESCE(SUM(CASE WHEN kind='post' AND status='published' THEN 1 ELSE 0 END),0),
			COALESCE(SUM(CASE WHEN status='draft' THEN 1 ELSE 0 END),0),
			MAX(CASE WHEN kind='post' AND status='published' THEN published_at END)
			FROM posts WHERE user_id = ? AND blog_id = ?`, uid, b.ID).Scan(&b.Posts, &b.Drafts, &last)
		b.LastPostAt = last.String
		_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM comments c JOIN posts p ON p.id = c.post_id
			WHERE c.user_id = ? AND p.blog_id = ? AND c.status = 'pending'`, uid, b.ID).Scan(&b.Pending)
	}
	return out, nil
}

// DeleteBlog removes a blog with all its posts and their comments.
func (s *Store) DeleteBlog(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM blogs WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
