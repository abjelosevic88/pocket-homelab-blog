package store

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// RandomKey is a random lowercase key of about n*1.6 characters (n bytes).
func RandomKey(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return strings.ToLower(strings.TrimRight(base32.StdEncoding.EncodeToString(b), "="))
}

const mediaCols = `id, user_id, file, name, mime, bytes, width, height, alt, source, source_ref, created_at`

func scanMedia(sc interface{ Scan(...any) error }) (*Media, error) {
	var m Media
	err := sc.Scan(&m.ID, &m.UserID, &m.File, &m.Name, &m.Mime, &m.Bytes, &m.Width, &m.Height, &m.Alt, &m.Source, &m.SourceRef, &m.CreatedAt)
	return &m, err
}

// AddMedia records an uploaded picture.
func (s *Store) AddMedia(ctx context.Context, uid int64, m *Media) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO media (user_id, file, name, mime, bytes, width, height, alt, source, source_ref)
		VALUES (?,?,?,?,?,?,?,?,?,?)`, uid, m.File, m.Name, m.Mime, m.Bytes, m.Width, m.Height, m.Alt, m.Source, m.SourceRef)
	if err != nil {
		return conflict(err)
	}
	m.ID, _ = res.LastInsertId()
	m.UserID = uid
	m.CreatedAt = Now()
	return nil
}

// ListMedia lists a person's pictures, newest first.
func (s *Store) ListMedia(ctx context.Context, uid int64) ([]Media, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+mediaCols+` FROM media WHERE user_id = ? ORDER BY id DESC`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Media{}
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

// GetMedia returns one of a person's pictures.
func (s *Store) GetMedia(ctx context.Context, uid, id int64) (*Media, error) {
	m, err := scanMedia(s.DB.QueryRowContext(ctx, `SELECT `+mediaCols+` FROM media WHERE id = ? AND user_id = ?`, id, uid))
	return m, notFound(err)
}

// MediaByFile finds a picture by its (random) file name.
func (s *Store) MediaByFile(ctx context.Context, file string) (*Media, error) {
	m, err := scanMedia(s.DB.QueryRowContext(ctx, `SELECT `+mediaCols+` FROM media WHERE file = ?`, file))
	return m, notFound(err)
}

// MediaBySource finds a picture brought in from another app before.
func (s *Store) MediaBySource(ctx context.Context, uid int64, source, ref string) (*Media, error) {
	m, err := scanMedia(s.DB.QueryRowContext(ctx, `SELECT `+mediaCols+` FROM media WHERE user_id = ? AND source = ? AND source_ref = ?`, uid, source, ref))
	return m, notFound(err)
}

// SetMediaAlt saves the words that describe a picture.
func (s *Store) SetMediaAlt(ctx context.Context, uid, id int64, alt string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE media SET alt = ? WHERE id = ? AND user_id = ?`, alt, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteMedia forgets a picture (the caller removes the file).
func (s *Store) DeleteMedia(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM media WHERE id = ? AND user_id = ?`, id, uid)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MediaUse counts, per file, the person's posts and blogs that show it.
func (s *Store) MediaUse(ctx context.Context, uid int64) (map[string]int, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT cover, body FROM posts WHERE user_id = ?
		UNION ALL SELECT avatar, about FROM blogs WHERE user_id = ?`, uid, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var cover, body string
		if err := rows.Scan(&cover, &body); err != nil {
			return nil, err
		}
		texts = append(texts, cover+"\n"+body)
	}
	ms, err := s.ListMedia(ctx, uid)
	if err != nil {
		return nil, err
	}
	out := map[string]int{}
	for _, m := range ms {
		for _, t := range texts {
			if strings.Contains(t, m.File) {
				out[m.File]++
			}
		}
	}
	return out, rows.Err()
}
