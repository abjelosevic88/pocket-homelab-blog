package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

// ---- "Start from your apps" ----
//
// A post can start from something already in another Pocket app: a trip, a
// book, a place, a recipe, a film, a note or a voice memo. Blog reads it
// through the person's own Pocket Core link (or direct token), writes the
// material into a draft — with Pocket AI as a first version in the
// person's voice, or as a tidy template without AI — and brings the
// pictures it may (a note's images, a film poster, a recipe photo).

// sourceItem is one thing a post can start from.
type sourceItem struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Subtitle string  `json:"subtitle,omitempty"`
	Date     string  `json:"date,omitempty"` // YYYY-MM-DD
	Rating   float64 `json:"rating,omitempty"`
	Why      string  `json:"why,omitempty"` // ideas: why now
	App      string  `json:"app,omitempty"`
}

// sourceKind is what a post from each app is.
var sourceKind = map[string]struct{ label, instruction, noun string }{
	"notes":     {"A note", "Turn the note into a blog post: keep everything it says and its pictures, make it read well for strangers.", "note"},
	"travel":    {"A trip", "Write a travel story: where, when, the plan and the highlights, practical tips at the end.", "trip"},
	"places":    {"A place", "Write a short, honest review of this place: what it is, what was good or not, the rating, for whom it is.", "place"},
	"bookshelf": {"A book", "Write a book review: what the book is about (no spoilers), what the reader thought, the rating, favourite quotes as > quotes.", "book"},
	"kitchen":   {"A recipe", "Write a recipe post: a short personal introduction, then the ingredient list and the steps exactly as given (do not change amounts).", "recipe"},
	"watch":     {"A film or show", "Write a short review of what was watched: what it is, what the writer liked, for whom it is. Leave [placeholders] for the writer's opinions.", "film"},
	"voice":     {"A voice memo", "Turn what the person said into a blog post: keep their words, ideas and voice, make it read well.", "memo"},
}

// handleSources lists the apps a post can start from and whether each works.
func (s *Server) handleSources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := userFrom(ctx)
	st, _ := s.store.GetSettings(ctx, u.ID)
	out := []map[string]any{}
	for _, a := range pocketApps {
		_, via, detail := s.connStatus(ctx, u, st, a)
		out = append(out, map[string]any{"slug": a.slug, "name": a.name, "label": sourceKind[a.slug].label, "reason": a.reason, "connected": via != "", "detail": detail})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSourceItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, ok := appBySlug(chiParam(r, "app"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown app")
		return
	}
	c := s.clients(ctx, actorID(ctx))
	sib, err := c.conn(ctx, a, a.scopes...)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := s.listSource(ctx, sib, a.slug, strings.TrimSpace(r.URL.Query().Get("q")), c.st)
	if err != nil {
		writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

// ---- reading the other apps ----

func getJSONFrom(ctx context.Context, sib *siblingConn, path string, q url.Values) (any, error) {
	var raw json.RawMessage
	if err := sib.client.Do(ctx, http.MethodGet, path, q, nil, &raw); err != nil {
		return nil, err
	}
	var v any
	err := json.Unmarshal(raw, &v)
	return v, err
}

// rows finds the list in an answer: bare, or under one of keys.
func rows(v any, keys ...string) []map[string]any {
	var arr []any
	switch t := v.(type) {
	case []any:
		arr = t
	case map[string]any:
		for _, k := range append(keys, "items", "results") {
			if a, ok := t[k].([]any); ok {
				arr = a
				break
			}
		}
	}
	out := []map[string]any{}
	for _, x := range arr {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func str(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case []any:
			var parts []string
			for _, x := range v {
				if s, ok := x.(string); ok && s != "" {
					parts = append(parts, s)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, ", ")
			}
		}
	}
	return ""
}

func num(m map[string]any, k string) float64 {
	switch v := m[k].(type) {
	case float64:
		return v
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	}
	return 0
}

func obj(m map[string]any, k string) map[string]any {
	if o, ok := m[k].(map[string]any); ok {
		return o
	}
	return map[string]any{}
}

// day10 is the YYYY-MM-DD part of a date, a timestamp or unix seconds.
func day10(v any) string {
	switch t := v.(type) {
	case string:
		if len(t) >= 10 {
			return t[:10]
		}
	case float64:
		if t > 1e9 {
			return time.Unix(int64(t), 0).Format("2006-01-02")
		}
	}
	return ""
}

func idOf(m map[string]any) string {
	switch v := m["id"].(type) {
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case string:
		return v
	}
	return ""
}

func joinNonEmpty(sep string, parts ...string) string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return strings.Join(out, sep)
}

func (s *Server) listSource(ctx context.Context, sib *siblingConn, app, q string, st store.Settings) ([]sourceItem, error) {
	vals := url.Values{}
	items := []sourceItem{}
	switch app {
	case "notes":
		path := "/api/v1/notes"
		if q != "" {
			path = "/api/v1/search"
			vals.Set("q", q)
			vals.Set("limit", "30")
		} else {
			vals.Set("sort", "modified")
			vals.Set("limit", "40")
			if st.NotesFolder != "" {
				vals.Set("category", st.NotesFolder)
				vals.Set("recursive", "1")
			}
		}
		v, err := getJSONFrom(ctx, sib, path, vals)
		if err != nil {
			return nil, err
		}
		for _, m := range rows(v, "notes") {
			items = append(items, sourceItem{ID: idOf(m), Title: firstNonEmpty(str(m, "title"), "Untitled note"), Subtitle: joinNonEmpty(" · ", str(m, "category"), clip(str(m, "snippet", "excerpt"), 90)), Date: day10(m["modified"])})
		}
	case "travel":
		if q != "" {
			vals.Set("q", q)
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/trips", vals)
		if err != nil {
			return nil, err
		}
		for _, m := range rows(v, "trips") {
			items = append(items, sourceItem{ID: idOf(m), Title: str(m, "name"), Subtitle: joinNonEmpty(" · ", str(m, "destination"), dateRange(str(m, "start_date"), str(m, "end_date"))), Date: str(m, "start_date")})
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].Date > items[j].Date })
	case "places":
		vals.Set("status", "been")
		vals.Set("limit", "60")
		if q != "" {
			vals.Set("q", q)
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/places", vals)
		if err != nil {
			return nil, err
		}
		for _, m := range rows(v, "places") {
			items = append(items, sourceItem{ID: idOf(m), Title: str(m, "name"), Subtitle: joinNonEmpty(" · ", str(m, "kind"), str(m, "city")), Date: day10(m["last_visit"]), Rating: num(m, "rating")})
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].Date > items[j].Date })
	case "bookshelf":
		vals.Set("status", "read")
		vals.Set("sort", "finished")
		vals.Set("limit", "60")
		if q != "" {
			vals.Set("q", q)
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/books", vals)
		if err != nil {
			return nil, err
		}
		for _, m := range rows(v, "books") {
			items = append(items, sourceItem{ID: idOf(m), Title: str(m, "title"), Subtitle: str(m, "authors"), Date: day10(m["finished_on"]), Rating: num(m, "rating")})
		}
	case "kitchen":
		vals.Set("limit", "60")
		if q != "" {
			vals.Set("q", q)
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/recipes", vals)
		if err != nil {
			return nil, err
		}
		for _, m := range rows(v, "recipes") {
			items = append(items, sourceItem{ID: idOf(m), Title: str(m, "title"), Subtitle: joinNonEmpty(" · ", str(m, "course"), str(m, "cuisine")), Date: day10(m["last_cooked"]), Rating: num(m, "rating")})
		}
	case "watch":
		vals.Set("limit", "60")
		if q != "" {
			vals.Set("q", q)
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/history", vals)
		if err != nil {
			return nil, err
		}
		seenTitle := map[string]bool{}
		for _, m := range rows(v, "items") {
			t := watchTitle(m)
			if seenTitle[t] {
				continue
			}
			seenTitle[t] = true
			items = append(items, sourceItem{ID: firstNonEmpty(str(m, "item_id"), idOf(m)), Title: t, Subtitle: joinNonEmpty(" · ", str(m, "year"), str(m, "genres")), Date: day10(m["watched_at"])})
		}
	case "voice":
		vals.Set("limit", "40")
		if q != "" {
			vals.Set("q", q)
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/memos", vals)
		if err != nil {
			return nil, err
		}
		for _, m := range rows(v, "items") {
			items = append(items, sourceItem{ID: idOf(m), Title: firstNonEmpty(str(m, "title"), "Voice memo"), Subtitle: clip(str(m, "summary"), 100), Date: day10(m["recorded_at"])})
		}
	default:
		return nil, errors.New("unknown app")
	}
	return items, nil
}

func watchTitle(m map[string]any) string {
	if str(m, "kind") == "episode" && str(m, "series") != "" {
		return str(m, "series")
	}
	return str(m, "title")
}

func dateRange(a, b string) string {
	if a == "" {
		return ""
	}
	if b == "" || b == a {
		return a
	}
	return a + " – " + b
}

func stars(r float64) string {
	if r <= 0 {
		return ""
	}
	n := int(math.Round(r))
	if n > 5 {
		n = 5
	}
	return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
}

// material is what an item is, as Markdown for the post (or for the AI).
type material struct {
	title    string
	body     string            // Markdown
	tags     []string          // suggested
	coverRef string            // path in the app to fetch the cover from (with the link token)
	coverURL string            // or a public https address
	images   map[string]string // note pictures: Markdown src → path in the app
	link     string            // the item in its app
}

func (s *Server) fetchMaterial(ctx context.Context, sib *siblingConn, app, id string) (*material, error) {
	mat := &material{images: map[string]string{}}
	switch app {
	case "notes":
		v, err := getJSONFrom(ctx, sib, "/api/v1/notes/"+url.PathEscape(id), nil)
		if err != nil {
			return nil, err
		}
		m, _ := v.(map[string]any)
		mat.title = str(m, "title")
		mat.body = stripLeadingH1(str(m, "content"), mat.title)
		for _, t := range strings.Split(str(m, "tags"), ", ") {
			if t != "" && t != "blog" {
				mat.tags = append(mat.tags, t)
			}
		}
		for _, src := range reMDImgAll.FindAllStringSubmatch(mat.body, -1) {
			p := src[1]
			if strings.HasPrefix(p, "http://") || strings.HasPrefix(p, "https://") || strings.HasPrefix(p, "/media/") || strings.HasPrefix(p, "data:") {
				continue
			}
			rel, _ := url.PathUnescape(p)
			mat.images[p] = "/api/v1/notes/" + url.PathEscape(id) + "/attachment?path=" + url.QueryEscape(rel)
		}
		mat.link = "/note/" + id
	case "travel":
		v, err := getJSONFrom(ctx, sib, "/api/v1/trips/"+url.PathEscape(id), nil)
		if err != nil {
			return nil, err
		}
		m, _ := v.(map[string]any)
		mat.title = str(m, "name")
		c := obj(m, "counts")
		var b strings.Builder
		fmt.Fprintf(&b, "- Destination: %s\n- When: %s\n", firstNonEmpty(str(m, "destination"), "?"), dateRange(str(m, "start_date"), str(m, "end_date")))
		if n := num(m, "days"); n > 0 {
			fmt.Fprintf(&b, "- Days: %d\n", int(n))
		}
		if n := num(c, "places"); n > 0 {
			fmt.Fprintf(&b, "- Places on the list: %d (visited %d)\n", int(n), int(num(c, "visited")))
		}
		if t := str(m, "description"); t != "" {
			b.WriteString("\n" + t + "\n")
		}
		if t := str(m, "notes"); t != "" {
			b.WriteString("\n## Notes\n\n" + t + "\n")
		}
		mat.body = b.String()
		mat.tags = []string{"travel"}
		if d := str(m, "destination"); d != "" {
			mat.tags = append(mat.tags, strings.ToLower(strings.Split(d, ",")[0]))
		}
		mat.link = "/trips/" + id
	case "places":
		v, err := getJSONFrom(ctx, sib, "/api/v1/places/"+url.PathEscape(id), nil)
		if err != nil {
			return nil, err
		}
		root, _ := v.(map[string]any)
		m := obj(root, "place")
		if len(m) == 0 {
			m = root
		}
		mat.title = str(m, "name")
		var b strings.Builder
		fmt.Fprintf(&b, "- What: %s\n- Where: %s\n", joinNonEmpty(", ", str(m, "kind"), str(m, "cuisine")), joinNonEmpty(", ", str(m, "address"), str(m, "city"), str(m, "country")))
		if r := num(m, "rating"); r > 0 {
			fmt.Fprintf(&b, "- My rating: %s (%.1f/5)\n", stars(r), r)
		}
		if p := int(num(m, "price")); p > 0 {
			fmt.Fprintf(&b, "- Price: %s\n", strings.Repeat("€", p))
		}
		if wsite := str(m, "website"); wsite != "" {
			fmt.Fprintf(&b, "- Website: %s\n", wsite)
		}
		if t := str(m, "notes"); t != "" {
			b.WriteString("\n" + t + "\n")
		}
		visits := rows(root["visits"])
		if len(visits) > 0 {
			b.WriteString("\n## My visits\n\n")
			for _, vi := range visits {
				if vi["planned"] == true {
					continue
				}
				line := "- " + str(vi, "date")
				if r := num(vi, "rating"); r > 0 {
					line += " " + stars(r)
				}
				if t := str(vi, "notes"); t != "" {
					line += ": " + t
				}
				if t := str(vi, "with_whom"); t != "" {
					line += " (with " + t + ")"
				}
				b.WriteString(line + "\n")
			}
		}
		mat.body = b.String()
		mat.tags = []string{strings.ToLower(firstNonEmpty(str(m, "kind"), "places"))}
		if c := str(m, "city"); c != "" {
			mat.tags = append(mat.tags, strings.ToLower(c))
		}
		mat.link = "/places/" + id
	case "bookshelf":
		v, err := getJSONFrom(ctx, sib, "/api/v1/books/"+url.PathEscape(id), nil)
		if err != nil {
			return nil, err
		}
		root, _ := v.(map[string]any)
		m := obj(root, "book")
		if len(m) == 0 {
			m = root
		}
		mat.title = str(m, "title")
		var b strings.Builder
		fmt.Fprintf(&b, "- Book: *%s*%s by %s\n", str(m, "title"), prefixed(": ", str(m, "subtitle")), firstNonEmpty(str(m, "authors"), "?"))
		if p := num(m, "pages"); p > 0 {
			fmt.Fprintf(&b, "- Pages: %d\n", int(p))
		}
		if r := num(m, "rating"); r > 0 {
			fmt.Fprintf(&b, "- My rating: %s\n", stars(r))
		}
		if f := str(m, "finished_on"); f != "" {
			fmt.Fprintf(&b, "- Finished: %s\n", f)
		}
		if t := str(m, "description"); t != "" {
			b.WriteString("\n## What it is about\n\n" + t + "\n")
		}
		if t := str(m, "review"); t != "" {
			b.WriteString("\n## What I thought\n\n" + t + "\n")
		}
		if qs := rows(root["quotes"]); len(qs) > 0 {
			b.WriteString("\n## Quotes I kept\n\n")
			for _, q := range qs {
				b.WriteString("> " + strings.ReplaceAll(str(q, "text"), "\n", "\n> ") + "\n\n")
			}
		}
		mat.body = b.String()
		mat.tags = []string{"books"}
		mat.link = "/books/" + id
	case "kitchen":
		v, err := getJSONFrom(ctx, sib, "/api/v1/recipes/"+url.PathEscape(id), nil)
		if err != nil {
			return nil, err
		}
		root, _ := v.(map[string]any)
		m := obj(root, "recipe")
		if len(m) == 0 {
			m = root
		}
		mat.title = str(m, "title")
		var b strings.Builder
		if t := str(m, "description"); t != "" {
			b.WriteString(t + "\n\n")
		}
		var facts []string
		if n := num(m, "servings"); n > 0 {
			facts = append(facts, fmt.Sprintf("Serves %d", int(n)))
		}
		if n := num(m, "total_minutes"); n > 0 {
			facts = append(facts, fmt.Sprintf("%d minutes", int(n)))
		}
		if len(facts) > 0 {
			b.WriteString("*" + strings.Join(facts, " · ") + "*\n\n")
		}
		b.WriteString("## Ingredients\n\n")
		group := ""
		for _, ing := range rows(m["ingredients"]) {
			if g := str(ing, "group"); g != "" && g != group {
				group = g
				b.WriteString("\n**" + g + "**\n\n")
			}
			line := joinNonEmpty(" ", str(ing, "qty"), str(ing, "unit"), str(ing, "name"))
			if n := str(ing, "note"); n != "" {
				line += ", " + n
			}
			if ing["optional"] == true {
				line += " (optional)"
			}
			b.WriteString("- " + line + "\n")
		}
		b.WriteString("\n## Steps\n\n")
		stepsAny, _ := m["steps"].([]any)
		for i, st := range stepsAny {
			text := ""
			switch x := st.(type) {
			case string:
				text = x
			case map[string]any:
				text = str(x, "text", "body")
			}
			if text != "" {
				fmt.Fprintf(&b, "%d. %s\n", i+1, text)
			}
		}
		if t := str(m, "notes"); t != "" {
			b.WriteString("\n## Tips\n\n" + t + "\n")
		}
		if src := str(m, "source_url"); src != "" {
			fmt.Fprintf(&b, "\n*Based on a recipe from [%s](%s).*\n", firstNonEmpty(str(m, "source_name"), "the original"), src)
		}
		mat.body = b.String()
		mat.tags = []string{"recipes"}
		if c := str(m, "cuisine"); c != "" {
			mat.tags = append(mat.tags, strings.ToLower(c))
		}
		if u := str(m, "image_url"); strings.HasPrefix(u, "https://") {
			mat.coverURL = u
		}
		mat.link = "/recipes/" + id
	case "watch":
		vals := url.Values{"limit": {"300"}}
		v, err := getJSONFrom(ctx, sib, "/api/v1/history", vals)
		if err != nil {
			return nil, err
		}
		var m map[string]any
		var times int
		for _, x := range rows(v, "items") {
			if str(x, "item_id") == id || idOf(x) == id || (m != nil && watchTitle(x) == watchTitle(m)) {
				if m == nil {
					m = x
				}
				times++
			}
		}
		if m == nil {
			return nil, store.ErrNotFound
		}
		mat.title = watchTitle(m)
		var b strings.Builder
		fmt.Fprintf(&b, "- Title: %s", mat.title)
		if y := str(m, "year"); y != "" {
			fmt.Fprintf(&b, " (%s)", y)
		}
		b.WriteString("\n")
		if g := str(m, "genres"); g != "" {
			fmt.Fprintf(&b, "- Genres: %s\n", g)
		}
		if str(m, "kind") == "episode" {
			fmt.Fprintf(&b, "- A series; last episode watched: S%sE%s %s\n", str(m, "season"), str(m, "episode"), str(m, "title"))
		}
		fmt.Fprintf(&b, "- Watched: %s\n", day10(m["watched_at"]))
		b.WriteString("\n[What I thought of it]\n")
		mat.body = b.String()
		mat.tags = []string{"films"}
		if p := str(m, "poster"); strings.HasPrefix(p, "/api/v1/img/") {
			mat.coverRef = p
		}
		mat.link = "/title/" + id
	case "voice":
		v, err := getJSONFrom(ctx, sib, "/api/v1/memos/"+url.PathEscape(id), nil)
		if err != nil {
			return nil, err
		}
		m, _ := v.(map[string]any)
		mat.title = str(m, "title")
		mat.body = firstNonEmpty(str(m, "tidy"), str(m, "transcript"))
		for _, t := range strings.Split(str(m, "tags"), ", ") {
			if t != "" {
				mat.tags = append(mat.tags, t)
			}
		}
		mat.link = "/memos/" + id
	default:
		return nil, errors.New("unknown app")
	}
	return mat, nil
}

var reMDImgAll = regexp.MustCompile(`!\[[^\]]*\]\(<?([^)\s>]+)>?`)

func prefixed(p, v string) string {
	if v == "" {
		return ""
	}
	return p + v
}

// stripLeadingH1 drops a "# Title" first line that repeats the title.
func stripLeadingH1(body, title string) string {
	t := strings.TrimLeft(body, "\n")
	if strings.HasPrefix(t, "# ") {
		line, rest, _ := strings.Cut(t, "\n")
		if strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(line, "# ")), strings.TrimSpace(title)) || title == "" {
			return strings.TrimLeft(rest, "\n")
		}
	}
	return body
}

// handleSourceDraft makes a draft from one item of another app.
func (s *Server) handleSourceDraft(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	a, ok := appBySlug(chiParam(r, "app"))
	if !ok {
		writeErr(w, http.StatusNotFound, "unknown app")
		return
	}
	var in struct {
		ID     string `json:"id"`
		BlogID int64  `json:"blog_id"`
		AI     bool   `json:"ai"`
		Again  bool   `json:"again"` // a new draft even if there is one already
		Note   string `json:"note"`  // what the writer wants to add or stress
	}
	if err := decode(r, &in); err != nil || strings.TrimSpace(in.ID) == "" {
		writeErr(w, http.StatusBadRequest, "pick something first")
		return
	}
	b, err := s.defaultBlog(ctx, uid, in.BlogID)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "make a blog first")
		return
	}
	ref := a.slug + ":" + in.ID
	if !in.Again {
		if prev, err := s.store.PostBySource(ctx, uid, b.ID, a.slug, ref); err == nil {
			s.dress(ctx, uid, prev, blogCache{b.ID: b})
			writeJSON(w, http.StatusOK, map[string]any{"post": prev, "existing": true})
			return
		}
	}
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, a, a.scopes...)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	mat, err := s.fetchMaterial(ctx, sib, a.slug, in.ID)
	if err != nil {
		writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
		return
	}
	// pictures first, so the text can point at them
	for src, path := range mat.images {
		data, _, err := sib.client.Bytes(ctx, path, s.cfg.MaxUploadBytes)
		if err != nil {
			continue
		}
		if m, err := s.savePicture(ctx, uid, data, src, a.slug, ref+":"+src, ""); err == nil {
			mat.body = strings.ReplaceAll(mat.body, "("+src+")", "(/media/"+m.File+")")
			mat.body = strings.ReplaceAll(mat.body, "(<"+src+">)", "(/media/"+m.File+")")
		}
	}
	cover := ""
	if mat.coverRef != "" {
		if data, _, err := sib.client.Bytes(ctx, mat.coverRef, s.cfg.MaxUploadBytes); err == nil {
			if m, err := s.savePicture(ctx, uid, data, mat.title, a.slug, ref+":cover", mat.title); err == nil {
				cover = m.File
			}
		}
	} else if mat.coverURL != "" {
		if data, err := s.fetchPublicImage(ctx, mat.coverURL); err == nil {
			if m, err := s.savePicture(ctx, uid, data, mat.title, a.slug, ref+":cover", mat.title); err == nil {
				cover = m.File
			}
		}
	}
	p := &store.Post{BlogID: b.ID, Kind: "post", Status: store.StatusDraft, Comments: true, Title: mat.title, Body: mat.body,
		Tags: strings.Join(mat.tags, ", "), Cover: cover, CoverAlt: mat.title, Source: a.slug, SourceRef: ref}
	if in.Again {
		p.SourceRef = ref + ":" + store.RandomKey(3)
	}
	usedAI := ""
	aiErr := ""
	if in.AI {
		st, _ := s.store.GetSettings(ctx, uid)
		if cl, provider, why := s.aiFor(ctx, uid, st); cl != nil && st.AIWriting {
			kind := sourceKind[a.slug]
			instr := kind.instruction
			if strings.TrimSpace(in.Note) != "" {
				instr += " The writer adds: " + clip(in.Note, 1000)
			}
			select {
			case s.aiSem <- struct{}{}:
				out, err := s.aiWrite(ctx, cl, st, b.Language, aiWriteIn{Action: "draft", Text: "A blog post about this " + kind.noun + ": " + mat.title, Body: mat.body, Instruction: instr})
				<-s.aiSem
				if err == nil {
					p.Title, p.Body, p.Excerpt = firstNonEmpty(out.Title, p.Title), out.Body, out.Excerpt
					if len(out.Tags) > 0 {
						p.Tags = strings.Join(append(mat.tags, out.Tags...), ", ")
					}
					usedAI = provider
				} else {
					aiErr = err.Error()
				}
			case <-ctx.Done():
				return
			}
		} else {
			aiErr = firstNonEmpty(why, "writing help is off")
		}
	}
	if usedAI == "" && strings.TrimSpace(in.Note) != "" {
		p.Body = strings.TrimSpace(in.Note) + "\n\n" + p.Body
	}
	if err := s.store.CreatePost(ctx, uid, p); err != nil {
		s.fail(w, err, "create post")
		return
	}
	s.dress(ctx, uid, p, blogCache{b.ID: b})
	writeJSON(w, http.StatusCreated, map[string]any{"post": p, "ai": usedAI, "ai_error": aiErr, "source_url": s.siblingLink(ctx, uid, a, mat.link)})
}

// fetchPublicImage downloads a picture from a public https address (a recipe photo).
func (s *Server) fetchPublicImage(ctx context.Context, u string) ([]byte, error) {
	pu, err := url.Parse(u)
	if err != nil || pu.Scheme != "https" || pu.Hostname() == "" {
		return nil, errors.New("not a public picture address")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", s.cfg.UserAgent)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, s.cfg.MaxUploadBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > s.cfg.MaxUploadBytes {
		return nil, errors.New("too big")
	}
	return data, nil
}

// ---- ideas: what in the other apps would make a good post now ----

func (s *Server) handleIdeas(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	key := fmt.Sprintf("ideas:%d", uid)
	if !queryBool(r, "fresh") {
		if v, at, ok := s.store.CacheGet(ctx, key); ok {
			if t, err := time.Parse(time.RFC3339Nano, at); err == nil && time.Since(t) < 15*time.Minute {
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				_, _ = w.Write([]byte(v))
				return
			}
		}
	}
	ideas := s.collectIdeas(ctx, uid)
	b, _ := json.Marshal(ideas)
	_ = s.store.CachePut(ctx, key, string(b))
	writeJSON(w, http.StatusOK, ideas)
}

func (s *Server) collectIdeas(ctx context.Context, uid int64) []sourceItem {
	c := s.clients(ctx, uid)
	today := store.Today()
	within := func(d string, days int) bool { return d != "" && d <= today && d >= store.AddDays(today, -days) }
	ideas := []sourceItem{}
	written := map[string]bool{}
	if ps, _, err := s.store.ListPosts(ctx, uid, store.PostFilter{}); err == nil {
		for _, p := range ps {
			if parts := strings.SplitN(p.SourceRef, ":", 3); p.Source != "" && len(parts) >= 2 {
				written[parts[0]+":"+parts[1]] = true
			}
		}
	}
	add := func(app string, it sourceItem, why string) {
		if written[app+":"+it.ID] {
			return
		}
		it.App, it.Why = app, why
		ideas = append(ideas, it)
	}
	tctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	for _, a := range pocketApps {
		sib, err := optional(c.conn(tctx, a, a.scopes...))
		if err != nil || sib == nil {
			continue
		}
		switch a.slug {
		case "notes":
			v, err := getJSONFrom(tctx, sib, "/api/v1/notes", url.Values{"tag": {"blog"}, "limit": {"10"}, "sort": {"modified"}})
			if err != nil {
				continue
			}
			for _, m := range rows(v, "notes") {
				add("notes", sourceItem{ID: idOf(m), Title: firstNonEmpty(str(m, "title"), "Untitled note"), Date: day10(m["modified"])}, "A note you tagged #blog")
			}
		case "travel":
			items, err := s.listSource(tctx, sib, "travel", "", c.st)
			if err != nil {
				continue
			}
			v, _ := getJSONFrom(tctx, sib, "/api/v1/trips", nil)
			ends := map[string]string{}
			for _, m := range rows(v, "trips") {
				ends[idOf(m)] = str(m, "end_date")
			}
			for _, it := range items {
				if within(ends[it.ID], 45) {
					add("travel", it, "You came back from this trip")
				}
			}
		case "bookshelf":
			items, err := s.listSource(tctx, sib, "bookshelf", "", c.st)
			if err != nil {
				continue
			}
			for _, it := range items {
				if within(it.Date, 60) {
					add("bookshelf", it, "You finished this book")
				}
			}
		case "places":
			items, err := s.listSource(tctx, sib, "places", "", c.st)
			if err != nil {
				continue
			}
			for _, it := range items {
				if within(it.Date, 30) && it.Rating >= 4 {
					add("places", it, "You loved this place")
				}
			}
		case "kitchen":
			v, err := getJSONFrom(tctx, sib, "/api/v1/cooklog", url.Values{"limit": {"30"}})
			if err != nil {
				continue
			}
			got := map[string]bool{}
			for _, m := range rows(v) {
				rid := str(m, "recipe_id")
				if got[rid] || !within(day10(m["date"]), 21) || num(m, "rating") < 4 {
					continue
				}
				got[rid] = true
				add("kitchen", sourceItem{ID: rid, Title: str(m, "recipe_title"), Date: day10(m["date"]), Rating: num(m, "rating")}, "You cooked this and liked it")
			}
		case "watch":
			items, err := s.listSource(tctx, sib, "watch", "", c.st)
			if err != nil {
				continue
			}
			n := 0
			for _, it := range items {
				if within(it.Date, 10) && n < 3 {
					add("watch", it, "You watched this")
					n++
				}
			}
		case "voice":
			v, err := getJSONFrom(tctx, sib, "/api/v1/memos", url.Values{"limit": {"30"}})
			if err != nil {
				continue
			}
			for _, m := range rows(v, "items") {
				if k := str(m, "kind"); (k == "idea" || k == "journal") && within(day10(m["recorded_at"]), 14) {
					add("voice", sourceItem{ID: idOf(m), Title: firstNonEmpty(str(m, "title"), "Voice memo"), Subtitle: clip(str(m, "summary"), 100), Date: day10(m["recorded_at"])}, "An "+k+" you recorded")
				}
			}
		}
	}
	sort.SliceStable(ideas, func(i, j int) bool { return ideas[i].Date > ideas[j].Date })
	if len(ideas) > 12 {
		ideas = ideas[:12]
	}
	return ideas
}
