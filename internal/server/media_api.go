package server

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

// savePicture stores a picture for a person and records it.
func (s *Server) savePicture(ctx context.Context, uid int64, data []byte, name, source, ref, alt string) (*store.Media, error) {
	if ref != "" {
		if m, err := s.store.MediaBySource(ctx, uid, source, ref); err == nil {
			s.dressMedia(m)
			return m, nil
		}
	}
	out, mime, ext, w, h, err := preparePicture(data)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.cfg.MediaDir, 0o755); err != nil {
		return nil, err
	}
	file := store.RandomKey(10) + ext
	if err := os.WriteFile(filepath.Join(s.cfg.MediaDir, file), out, 0o644); err != nil {
		return nil, err
	}
	m := &store.Media{File: file, Name: clip(strings.TrimSpace(name), 200), Mime: mime, Bytes: int64(len(out)), Width: w, Height: h,
		Alt: clip(strings.TrimSpace(alt), 300), Source: source, SourceRef: ref}
	if err := s.store.AddMedia(ctx, uid, m); err != nil {
		_ = os.Remove(filepath.Join(s.cfg.MediaDir, file))
		return nil, err
	}
	s.dressMedia(m)
	return m, nil
}

func (s *Server) dressMedia(m *store.Media) { m.URL = "/media/" + m.File }

func (s *Server) handleListMedia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	ms, err := s.store.ListMedia(ctx, uid)
	if err != nil {
		s.fail(w, err, "pictures")
		return
	}
	use, _ := s.store.MediaUse(ctx, uid)
	for i := range ms {
		s.dressMedia(&ms[i])
		ms[i].Used = use[ms[i].File]
	}
	writeJSON(w, http.StatusOK, ms)
}

// handleUpload takes one or more pictures (form field "file").
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxUploadBytes*10+1<<20)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeErr(w, http.StatusBadRequest, "the pictures are too big (at most "+itoa(s.cfg.MaxUploadBytes>>20)+" MB each)")
		return
	}
	files := r.MultipartForm.File["file"]
	if len(files) == 0 {
		writeErr(w, http.StatusBadRequest, "no picture")
		return
	}
	out := []store.Media{}
	for _, fh := range files {
		if fh.Size > s.cfg.MaxUploadBytes {
			writeErr(w, http.StatusRequestEntityTooLarge, fh.Filename+" is too big (at most "+itoa(s.cfg.MaxUploadBytes>>20)+" MB)")
			return
		}
		f, err := fh.Open()
		if err != nil {
			s.fail(w, err, "upload")
			return
		}
		data, err := io.ReadAll(io.LimitReader(f, s.cfg.MaxUploadBytes+1))
		f.Close()
		if err != nil {
			s.fail(w, err, "upload")
			return
		}
		m, err := s.savePicture(ctx, uid, data, fh.Filename, "", "", r.FormValue("alt"))
		if err != nil {
			writeErr(w, http.StatusBadRequest, fh.Filename+": "+err.Error())
			return
		}
		out = append(out, *m)
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) handleUpdateMedia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		Alt string `json:"alt"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := s.store.SetMediaAlt(ctx, uid, idParam(r, "id"), clip(strings.TrimSpace(in.Alt), 300)); err != nil {
		s.fail(w, err, "picture")
		return
	}
	m, _ := s.store.GetMedia(ctx, uid, idParam(r, "id"))
	s.dressMedia(m)
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) handleDeleteMedia(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	m, err := s.store.GetMedia(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "picture")
		return
	}
	if err := s.store.DeleteMedia(ctx, uid, m.ID); err != nil {
		s.fail(w, err, "picture")
		return
	}
	_ = os.Remove(filepath.Join(s.cfg.MediaDir, m.File))
	w.WriteHeader(http.StatusNoContent)
}
