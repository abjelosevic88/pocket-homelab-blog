package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

// pushes runs every minute; once a day, at each person's own hour, it says
// how the blog did yesterday and which comments wait (when they want that).
// New comments and scheduled posts going out are told as they happen.
func (s *Server) pushes(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || !st.NotifyDigest || now.Hour() != st.NotifyHour || now.Minute() > 10 {
			continue
		}
		if !s.once(ctx, fmt.Sprintf("push:%d:%s", uid, store.Today())) {
			continue
		}
		s.dailyCheck(ctx, uid, st, now)
	}
}

// once is true the first time it is asked for a key.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

// dailyCheck builds and sends one person's morning message: readers
// yesterday, the most read post and the comments waiting for an OK.
func (s *Server) dailyCheck(ctx context.Context, uid int64, st store.Settings, now time.Time) (string, bool) {
	yesterday := store.AddDays(now.Format("2006-01-02"), -1)
	days, _ := s.store.ViewsByDay(ctx, uid, 0, yesterday)
	views := 0
	for _, d := range days {
		if d.Day == yesterday {
			views = d.Count
		}
	}
	pending := s.store.PendingComments(ctx, uid)
	if views == 0 && pending == 0 {
		return "", false
	}
	var lines []string
	if views > 0 {
		lines = append(lines, fmt.Sprintf("%d %s yesterday.", views, plural(views, "reader", "readers")))
		if top, _ := s.store.TopPosts(ctx, uid, 0, yesterday, 1); len(top) > 0 && top[0].Views > 0 {
			lines = append(lines, fmt.Sprintf("Most read: “%s” (%d).", firstNonEmpty(top[0].Title, "Untitled"), top[0].Views))
		}
	}
	if pending > 0 {
		lines = append(lines, fmt.Sprintf("%d %s %s for your OK.", pending, plural(pending, "comment", "comments"), plural(pending, "waits", "wait")))
	}
	body := strings.Join(lines, "\n")
	link := strings.TrimRight(s.cfg.BaseURL, "/") + "/"
	if pending > 0 {
		link += "comments"
	}
	if !s.push(ctx, uid, st, "Your blog yesterday", body, link) {
		return body, false
	}
	return body, true
}

// push sends one message to a person's own notification address.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, link string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "info", Tags: []string{"memo"}, Link: link}); err != nil {
		s.log.Warn("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket Blog", Body: notifyTestBody, Priority: "info", Tags: []string{"memo"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const notifyTestBody = "Notifications work. New comments, posts going out at the time you set, and (if you want it) a morning note about your readers will arrive here."
