package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const commentCols = `c.id, c.user_id, c.post_id, c.parent_id, c.name, c.email, c.body, c.status, c.by_author, c.ip_hash, c.created_at`

func scanComment(sc interface{ Scan(...any) error }, extra ...any) (*Comment, error) {
	var c Comment
	var parent sql.NullInt64
	var by int
	dst := append([]any{&c.ID, &c.UserID, &c.PostID, &parent, &c.Name, &c.Email, &c.Body, &c.Status, &by, &c.IPHash, &c.CreatedAt}, extra...)
	if err := sc.Scan(dst...); err != nil {
		return nil, err
	}
	if parent.Valid {
		c.ParentID = &parent.Int64
	}
	c.ByAuthor = by == 1
	return &c, nil
}

// AddComment stores a comment under one of the writer's posts.
func (s *Store) AddComment(ctx context.Context, c *Comment) error {
	if c.Status == "" {
		c.Status = "pending"
	}
	if c.CreatedAt == "" {
		c.CreatedAt = Now()
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO comments (user_id, post_id, parent_id, name, email, body, status, by_author, ip_hash, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, c.UserID, c.PostID, nullI(c.ParentID), c.Name, c.Email, c.Body, c.Status, boolInt(c.ByAuthor), c.IPHash, c.CreatedAt)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	return nil
}

// CommentFilter narrows the writer's list of comments.
type CommentFilter struct {
	BlogID int64
	PostID int64
	Status string // pending | approved | spam | "" all but spam
	Limit  int
}

// ListComments lists the comments under a person's posts, newest first.
func (s *Store) ListComments(ctx context.Context, uid int64, f CommentFilter) ([]Comment, error) {
	where := []string{"c.user_id = ?"}
	args := []any{uid}
	if f.BlogID > 0 {
		where = append(where, "p.blog_id = ?")
		args = append(args, f.BlogID)
	}
	if f.PostID > 0 {
		where = append(where, "c.post_id = ?")
		args = append(args, f.PostID)
	}
	if f.Status != "" {
		where = append(where, "c.status = ?")
		args = append(args, f.Status)
	} else {
		where = append(where, "c.status <> 'spam'")
	}
	q := `SELECT ` + commentCols + `, p.title, p.blog_id FROM comments c JOIN posts p ON p.id = c.post_id WHERE ` + strings.Join(where, " AND ") + ` ORDER BY c.id DESC`
	if f.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", f.Limit)
	}
	rows, err := s.DB.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		var title string
		var blog int64
		c, err := scanComment(rows, &title, &blog)
		if err != nil {
			return nil, err
		}
		c.PostTitle, c.BlogID = title, blog
		out = append(out, *c)
	}
	return out, rows.Err()
}

// PublicComments are the approved comments of a post, oldest first.
func (s *Store) PublicComments(ctx context.Context, postID int64) ([]Comment, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+commentCols+` FROM comments c WHERE c.post_id = ? AND c.status = 'approved' ORDER BY c.id`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Comment{}
	for rows.Next() {
		c, err := scanComment(rows)
		if err != nil {
			return nil, err
		}
		c.Email = ""
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetComment returns one comment under a person's posts.
func (s *Store) GetComment(ctx context.Context, uid, id int64) (*Comment, error) {
	c, err := scanComment(s.DB.QueryRowContext(ctx, `SELECT `+commentCols+` FROM comments c WHERE c.id = ? AND c.user_id = ?`, id, uid))
	return c, notFound(err)
}

// SetCommentStatus approves a comment or marks it as spam.
func (s *Store) SetCommentStatus(ctx context.Context, uid, id int64, status string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE comments SET status = ? WHERE id = ? AND user_id = ?`, status, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteComment removes a comment (and the replies to it).
func (s *Store) DeleteComment(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM comments WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// RecentCommentsFrom counts the comments one reader (by address hash) wrote lately.
func (s *Store) RecentCommentsFrom(ctx context.Context, ipHash string, since time.Duration) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM comments WHERE ip_hash = ? AND created_at >= ?`,
		ipHash, time.Now().Add(-since).UTC().Format(time.RFC3339Nano)).Scan(&n)
	return n
}

// PendingComments counts a person's comments waiting for a yes.
func (s *Store) PendingComments(ctx context.Context, uid int64) int {
	var n int
	_ = s.DB.QueryRowContext(ctx, `SELECT COUNT(1) FROM comments WHERE user_id = ? AND status = 'pending'`, uid).Scan(&n)
	return n
}
