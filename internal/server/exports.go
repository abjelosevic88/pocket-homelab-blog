package server

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

// ---- your words, out and in ----

// exportDoc is one person's blogs as JSON (pictures come with the ZIPs).
type exportDoc struct {
	App      string          `json:"app"`
	Version  string          `json:"version"`
	Exported string          `json:"exported_at"`
	Blogs    []exportBlog    `json:"blogs"`
	Media    []store.Media   `json:"media"`
	Comments []store.Comment `json:"comments"`
}

type exportBlog struct {
	store.Blog
	PostsList []store.Post `json:"posts_list"`
}

func (s *Server) export(ctx context.Context, uid int64) exportDoc {
	doc := exportDoc{App: "pocket-blog", Version: Version, Exported: store.Now(), Blogs: []exportBlog{}}
	bs, _ := s.store.ListBlogs(ctx, uid)
	for _, b := range bs {
		ps, _, _ := s.store.ListPosts(ctx, uid, store.PostFilter{BlogID: b.ID, Body: true, Sort: "oldest"})
		doc.Blogs = append(doc.Blogs, exportBlog{Blog: b, PostsList: ps})
	}
	doc.Media, _ = s.store.ListMedia(ctx, uid)
	cs, _ := s.store.ListComments(ctx, uid, store.CommentFilter{})
	spam, _ := s.store.ListComments(ctx, uid, store.CommentFilter{Status: "spam"})
	doc.Comments = append(cs, spam...)
	return doc
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-blog-export.json"`)
	writeJSON(w, http.StatusOK, s.export(r.Context(), actorID(r.Context())))
}

// handleExportMarkdown: every post as a Markdown file with its details on top
// (works with Hugo, Jekyll, Obsidian…), plus the pictures.
func (s *Server) handleExportMarkdown(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	doc := s.export(ctx, uid)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-blog-markdown.zip"`)
	zw := zip.NewWriter(w)
	defer zw.Close()
	if f, err := zw.Create("export.json"); err == nil {
		_ = json.NewEncoder(f).Encode(doc)
	}
	for _, b := range doc.Blogs {
		for _, p := range b.PostsList {
			dir := b.Handle + "/posts/"
			if p.Kind == "page" {
				dir = b.Handle + "/pages/"
			}
			f, err := zw.Create(dir + p.Slug + ".md")
			if err != nil {
				continue
			}
			_, _ = f.Write([]byte(frontMatter(&p) + p.Body + "\n"))
		}
	}
	s.zipMedia(zw, doc.Media, "media/")
}

func yamlStr(v string) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func frontMatter(p *store.Post) string {
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s\n", yamlStr(p.Title))
	if d := firstNonEmpty(p.PublishedAt, p.PublishAt); d != "" {
		fmt.Fprintf(&b, "date: %s\n", d)
	}
	if tags := store.SplitTags(p.Tags); len(tags) > 0 {
		t, _ := json.Marshal(tags)
		fmt.Fprintf(&b, "tags: %s\n", t)
	}
	if p.Excerpt != "" {
		fmt.Fprintf(&b, "description: %s\n", yamlStr(p.Excerpt))
	}
	if p.Cover != "" {
		fmt.Fprintf(&b, "cover: %s\n", yamlStr("/media/"+p.Cover))
	}
	fmt.Fprintf(&b, "slug: %s\n", p.Slug)
	if p.Status != store.StatusPublished {
		b.WriteString("draft: true\n")
	}
	b.WriteString("---\n\n")
	return b.String()
}

func (s *Server) zipMedia(zw *zip.Writer, ms []store.Media, dir string) {
	for _, m := range ms {
		src, err := os.Open(filepath.Join(s.cfg.MediaDir, m.File))
		if err != nil {
			continue
		}
		if f, err := zw.CreateHeader(&zip.FileHeader{Name: dir + m.File, Method: zip.Store}); err == nil {
			_, _ = io.Copy(f, src)
		}
		src.Close()
	}
}

// handleExportSite builds one blog as plain web pages: put the folder on any
// web host (Netlify, GitHub Pages, a USB stick) and it works without this app.
func (s *Server) handleExportSite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	b, err := s.defaultBlog(ctx, uid, int64(queryInt(r, "blog")))
	if err != nil {
		s.fail(w, err, "blog")
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", `attachment; filename="`+b.Handle+`-website.zip"`)
	zw := zip.NewWriter(w)
	defer zw.Close()
	page := func(name string, pg *sitePage) {
		pg.CanComment, pg.Preview = false, false
		var buf bytes.Buffer
		if err := siteTmpl.ExecuteTemplate(&buf, "page", pg); err != nil {
			return
		}
		html := strings.ReplaceAll(buf.String(), `/_blog/site.css?v=`+Version, `/_blog/site.css`)
		if f, err := zw.Create(name); err == nil {
			_, _ = f.Write([]byte(html))
		}
	}
	if css, err := siteFS.ReadFile("site/site.css"); err == nil {
		if f, err := zw.Create("_blog/site.css"); err == nil {
			_, _ = f.Write(css)
		}
	}
	all, _, _ := s.store.ListPosts(ctx, uid, store.PostFilter{BlogID: b.ID, Kind: "post", Public: true})
	s.fillExcerpts(ctx, all)
	per := b.PerPage
	if per < 3 {
		per = 10
	}
	pages := (len(all) + per - 1) / per
	if pages == 0 {
		pages = 1
	}
	for n := 1; n <= pages; n++ {
		pg := s.basePage(ctx, b, "", "home", "")
		end := n * per
		if end > len(all) {
			end = len(all)
		}
		pg.Posts = s.listing(b, "", all[(n-1)*per:end])
		if n == 1 {
			if b.About != "" {
				pg.AboutHTML = renderMarkdown(b.About, "")
			}
			if b.Theme == "magazine" && len(pg.Posts) > 0 {
				h := pg.Posts[0]
				pg.Hero, pg.Posts = &h, pg.Posts[1:]
			}
		}
		if n < pages {
			pg.OlderURL = fmt.Sprintf("/page/%d/", n+1)
		}
		if n == 2 {
			pg.NewerURL = "/"
		} else if n > 2 {
			pg.NewerURL = fmt.Sprintf("/page/%d/", n-1)
		}
		name := "index.html"
		if n > 1 {
			name = fmt.Sprintf("page/%d/index.html", n)
		}
		page(name, pg)
	}
	everything, _, _ := s.store.ListPosts(ctx, uid, store.PostFilter{BlogID: b.ID, Public: true})
	for i := range everything {
		p, err := s.store.GetPost(ctx, uid, everything[i].ID)
		if err != nil {
			continue
		}
		pg := s.basePage(ctx, b, "", p.Kind, p.Title)
		pg.Menu[0].Active = false
		sp := s.toSite(b, "", p, true)
		pg.Post = &sp
		pg.Description = firstNonEmpty(p.SEODescription, p.Excerpt, clip(plainText(p.Body), 160))
		if p.Kind == "post" {
			cs, _ := s.store.PublicComments(ctx, p.ID)
			pg.Comments, pg.CommentsN = threadComments(b.Language, cs), len(cs)
			if pv, nx := s.store.Neighbours(ctx, p); pv != nil || nx != nil {
				if pv != nil {
					x := s.toSite(b, "", pv, false)
					pg.Prev = &x
				}
				if nx != nil {
					x := s.toSite(b, "", nx, false)
					pg.Next = &x
				}
			}
		}
		page(p.Slug+"/index.html", pg)
	}
	tags, _ := s.store.Tags(ctx, uid, b.ID, true)
	for _, t := range tags {
		ps, _, _ := s.store.ListPosts(ctx, uid, store.PostFilter{BlogID: b.ID, Kind: "post", Public: true, Tag: t.Tag})
		s.fillExcerpts(ctx, ps)
		pg := s.basePage(ctx, b, "", "tag", "#"+t.Tag)
		pg.Menu[0].Active = false
		pg.Tag, pg.Posts = t.Tag, s.listing(b, "", ps)
		page("tag/"+t.Tag+"/index.html", pg)
	}
	arch := s.basePage(ctx, b, "", "archive", word(b.Language, "archive"))
	arch.Menu[0].Active = false
	for _, sp := range s.listing(b, "", all) {
		y := ""
		if t, err := time.Parse(time.RFC3339Nano, sp.ISODate); err == nil {
			y = strconv.Itoa(t.Local().Year())
		}
		if n := len(arch.Years); n == 0 || arch.Years[n-1].Year != y {
			arch.Years = append(arch.Years, yearGroup{Year: y})
		}
		arch.Years[len(arch.Years)-1].Posts = append(arch.Years[len(arch.Years)-1].Posts, sp)
	}
	page("archive/index.html", arch)
	nf := s.basePage(ctx, b, "", "notfound", "404")
	nf.Menu[0].Active = false
	page("404.html", nf)
	ms, _ := s.store.ListMedia(ctx, uid)
	use, _ := s.store.MediaUse(ctx, uid)
	var used []store.Media
	for _, m := range ms {
		if use[m.File] > 0 {
			used = append(used, m)
		}
	}
	s.zipMedia(zw, used, "media/")
	if f, err := zw.Create("README.txt"); err == nil {
		fmt.Fprintf(f, "%s — made with Pocket Blog %s on %s.\n\nUpload everything in this folder to the root of any web host\n(Netlify Drop, GitHub Pages, Cloudflare Pages, your own server).\nSearch and comments need Pocket Blog itself; everything else works as plain pages.\n",
			b.Title, Version, time.Now().Format("2 January 2006"))
	}
}

// ---- import ----

// handleImport takes a Pocket Blog export, a WordPress export (.xml), a
// Markdown file or a ZIP of Markdown files. Importing the same thing twice
// adds nothing the second time.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var data []byte
	name := ""
	blogID := int64(queryInt(r, "blog"))
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes*4)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		f, fh, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "no file")
			return
		}
		defer f.Close()
		name = fh.Filename
		data, err = io.ReadAll(f)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "could not read the file")
			return
		}
		if v, err := strconv.ParseInt(r.FormValue("blog"), 10, 64); err == nil {
			blogID = v
		}
	} else {
		b, err := s.readBody(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		data = b
	}
	lower := strings.ToLower(name)
	trim := bytes.TrimSpace(data)
	switch {
	case bytes.HasPrefix(trim, []byte("{")):
		var doc exportDoc
		if err := json.Unmarshal(data, &doc); err != nil || doc.App != "pocket-blog" {
			writeErr(w, http.StatusBadRequest, "that is not a Pocket Blog export")
			return
		}
		writeJSON(w, http.StatusOK, s.importPocket(ctx, uid, doc))
	case bytes.HasPrefix(trim, []byte("<")) || strings.HasSuffix(lower, ".xml"):
		b, err := s.importTarget(ctx, uid, blogID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "make a blog first, then bring the posts into it")
			return
		}
		out, err := s.importWordPress(ctx, uid, b, data)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	case strings.HasSuffix(lower, ".zip") || bytes.HasPrefix(data, []byte("PK\x03\x04")):
		b, err := s.importTarget(ctx, uid, blogID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "make a blog first, then bring the posts into it")
			return
		}
		out, err := s.importMarkdownZip(ctx, uid, b, data)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, out)
	default:
		b, err := s.importTarget(ctx, uid, blogID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "make a blog first, then bring the posts into it")
			return
		}
		counts := map[string]int{"posts": 0, "pictures": 0, "comments": 0}
		if s.importMarkdownFile(ctx, uid, b, firstNonEmpty(name, "post.md"), string(data), nil) {
			counts["posts"]++
		}
		writeJSON(w, http.StatusOK, counts)
	}
}

func (s *Server) importTarget(ctx context.Context, uid, id int64) (*store.Blog, error) {
	return s.defaultBlog(ctx, uid, id)
}

func (s *Server) importPocket(ctx context.Context, uid int64, doc exportDoc) map[string]int {
	counts := map[string]int{"blogs": 0, "posts": 0, "comments": 0}
	postIDs := map[int64]int64{}
	for _, eb := range doc.Blogs {
		b := eb.Blog
		// a blog that had to take another address is remembered under its old one
		mapKey := fmt.Sprintf("import-blog:%d:%s", uid, b.Handle)
		target, err := s.store.BlogByHandle(ctx, b.Handle)
		if v, _, ok := s.store.CacheGet(ctx, mapKey); ok && (err != nil || target.UserID != uid) {
			if id, perr := strconv.ParseInt(v, 10, 64); perr == nil {
				if mine, gerr := s.store.GetBlog(ctx, uid, id); gerr == nil {
					target, err = mine, nil
				}
			}
		}
		if err != nil || target.UserID != uid {
			nb := b
			nb.ID = 0
			if err == nil { // somebody else's address: take a free one
				nb.Handle = s.suggestHandle(ctx, b.Title, 0)
			}
			if nb.Domain != "" {
				if _, err := s.store.BlogByDomain(ctx, nb.Domain); err == nil {
					nb.Domain = ""
				}
			}
			if nb.Avatar != "" {
				if m, err := s.store.MediaByFile(ctx, nb.Avatar); err != nil || m.UserID != uid {
					nb.Avatar = ""
				}
			}
			if err := s.store.CreateBlog(ctx, uid, &nb); err != nil {
				continue
			}
			target = &nb
			counts["blogs"]++
			_ = s.store.CachePut(ctx, mapKey, itoa(nb.ID))
		}
		for _, p := range eb.PostsList {
			old := p.ID
			ref := fmt.Sprintf("pocket:%s:%s", b.Handle, p.Slug)
			if prev, err := s.store.PostBySource(ctx, uid, target.ID, "import", ref); err == nil {
				postIDs[old] = prev.ID
				continue
			}
			if prev, err := s.store.PostBySlug(ctx, target.ID, p.Slug); err == nil && prev.UserID == uid && prev.Body == p.Body {
				postIDs[old] = prev.ID
				continue
			}
			p.ID, p.BlogID, p.Source, p.SourceRef, p.Views = 0, target.ID, "import", ref, 0
			if p.Cover != "" {
				if m, err := s.store.MediaByFile(ctx, p.Cover); err != nil || m.UserID != uid {
					p.Cover = ""
				}
			}
			if err := s.store.CreatePost(ctx, uid, &p); err == nil {
				postIDs[old] = p.ID
				counts["posts"]++
			}
		}
	}
	commentIDs := map[int64]int64{}
	for _, c := range doc.Comments {
		pid, ok := postIDs[c.PostID]
		if !ok {
			continue
		}
		have, _ := s.store.ListComments(ctx, uid, store.CommentFilter{PostID: pid, Status: c.Status})
		dup := false
		for _, h := range have {
			if h.Name == c.Name && h.Body == c.Body {
				dup = true
				commentIDs[c.ID] = h.ID
			}
		}
		if dup {
			continue
		}
		old := c.ID
		c.ID, c.UserID, c.PostID = 0, uid, pid
		if c.ParentID != nil {
			if np, ok := commentIDs[*c.ParentID]; ok {
				c.ParentID = &np
			} else {
				c.ParentID = nil
			}
		}
		if err := s.store.AddComment(ctx, &c); err == nil {
			commentIDs[old] = c.ID
			counts["comments"]++
		}
	}
	return counts
}

// WordPress export (Tools → Export → All content).
type wxr struct {
	Channel struct {
		Items []wxrItem `xml:"item"`
	} `xml:"channel"`
}

type wxrItem struct {
	Title      string `xml:"title"`
	Link       string `xml:"link"`
	Content    string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Excerpt    string `xml:"http://wordpress.org/export/1.2/excerpt/ encoded"`
	PostID     string `xml:"http://wordpress.org/export/1.2/ post_id"`
	PostDate   string `xml:"http://wordpress.org/export/1.2/ post_date_gmt"`
	PostDateL  string `xml:"http://wordpress.org/export/1.2/ post_date"`
	PostName   string `xml:"http://wordpress.org/export/1.2/ post_name"`
	Status     string `xml:"http://wordpress.org/export/1.2/ status"`
	PostType   string `xml:"http://wordpress.org/export/1.2/ post_type"`
	Categories []struct {
		Domain string `xml:"domain,attr"`
		Name   string `xml:",chardata"`
	} `xml:"category"`
	Comments []struct {
		ID       string `xml:"http://wordpress.org/export/1.2/ comment_id"`
		Author   string `xml:"http://wordpress.org/export/1.2/ comment_author"`
		Email    string `xml:"http://wordpress.org/export/1.2/ comment_author_email"`
		DateGMT  string `xml:"http://wordpress.org/export/1.2/ comment_date_gmt"`
		Content  string `xml:"http://wordpress.org/export/1.2/ comment_content"`
		Approved string `xml:"http://wordpress.org/export/1.2/ comment_approved"`
		Parent   string `xml:"http://wordpress.org/export/1.2/ comment_parent"`
		Type     string `xml:"http://wordpress.org/export/1.2/ comment_type"`
	} `xml:"http://wordpress.org/export/1.2/ comment"`
}

func (s *Server) importWordPress(ctx context.Context, uid int64, b *store.Blog, data []byte) (map[string]int, error) {
	var doc wxr
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("that is not a WordPress export file (Tools → Export in WordPress): %v", err)
	}
	counts := map[string]int{"posts": 0, "pages": 0, "comments": 0, "skipped": 0}
	for _, it := range doc.Channel.Items {
		if it.PostType != "post" && it.PostType != "page" {
			continue
		}
		if it.Status == "trash" || it.Status == "auto-draft" || it.Status == "inherit" {
			continue
		}
		ref := "wp:" + it.PostID
		if _, err := s.store.PostBySource(ctx, uid, b.ID, "import", ref); err == nil {
			counts["skipped"]++
			continue
		}
		body, err := wpToMarkdown(it.Content)
		if err != nil {
			body = it.Content
		}
		p := &store.Post{BlogID: b.ID, Kind: it.PostType, Title: strings.TrimSpace(it.Title), Slug: it.PostName, Body: body,
			Excerpt: strings.TrimSpace(plainHTML(it.Excerpt)), Status: store.StatusDraft, Comments: true, Source: "import", SourceRef: ref}
		var tags []string
		for _, c := range it.Categories {
			if (c.Domain == "post_tag" || c.Domain == "category") && strings.TrimSpace(c.Name) != "" && !strings.EqualFold(c.Name, "Uncategorized") {
				tags = append(tags, c.Name)
			}
		}
		p.Tags = strings.Join(tags, ", ")
		if it.Status == "publish" {
			p.Status = store.StatusPublished
			p.PublishedAt = wpTime(it.PostDate, it.PostDateL)
		}
		if p.Kind == "page" {
			p.Comments = false
			p.InMenu = p.Status == store.StatusPublished
		}
		if err := s.store.CreatePost(ctx, uid, p); err != nil {
			continue
		}
		if p.Kind == "page" {
			counts["pages"]++
		} else {
			counts["posts"]++
		}
		ids := map[string]int64{}
		for _, c := range it.Comments {
			if c.Type != "" && c.Type != "comment" {
				continue
			}
			st := "pending"
			switch c.Approved {
			case "1":
				st = "approved"
			case "spam", "trash":
				continue
			}
			cm := &store.Comment{UserID: uid, PostID: p.ID, Name: c.Author, Email: c.Email, Body: plainHTML(c.Content), Status: st, CreatedAt: wpTime(c.DateGMT, "")}
			if pid, ok := ids[c.Parent]; ok && c.Parent != "0" {
				cm.ParentID = &pid
			}
			if err := s.store.AddComment(ctx, cm); err == nil {
				ids[c.ID] = cm.ID
				counts["comments"]++
			}
		}
	}
	if counts["posts"]+counts["pages"]+counts["skipped"] == 0 {
		return nil, fmt.Errorf("no posts found in that file")
	}
	return counts, nil
}

func wpTime(gmt, local string) string {
	if t, err := time.Parse("2006-01-02 15:04:05", gmt); err == nil && !strings.HasPrefix(gmt, "0000") {
		return t.UTC().Format(time.RFC3339Nano)
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", local, time.Local); err == nil {
		return t.UTC().Format(time.RFC3339Nano)
	}
	return store.Now()
}

// wpToMarkdown: WordPress keeps paragraphs as blank lines, not <p>.
func wpToMarkdown(content string) (string, error) {
	h := content
	if !strings.Contains(h, "<p") {
		var parts []string
		for _, para := range strings.Split(strings.ReplaceAll(h, "\r", ""), "\n\n") {
			if t := strings.TrimSpace(para); t != "" {
				if strings.HasPrefix(t, "<") && !strings.HasPrefix(t, "<a") && !strings.HasPrefix(t, "<strong") && !strings.HasPrefix(t, "<em") && !strings.HasPrefix(t, "<img") {
					parts = append(parts, t)
				} else {
					parts = append(parts, "<p>"+strings.ReplaceAll(t, "\n", "<br>")+"</p>")
				}
			}
		}
		h = strings.Join(parts, "\n")
	}
	h = reWPComment.ReplaceAllString(h, "")
	md, err := htmltomarkdown.ConvertString(h)
	return strings.TrimSpace(md), err
}

var reWPComment = regexpMust(`<!--.*?-->`)

func plainHTML(h string) string {
	t := reTags.ReplaceAllString(strings.NewReplacer("<br>", "\n", "<br />", "\n", "</p>", "\n\n").Replace(h), "")
	return strings.TrimSpace(htmlUnescape(t))
}

func (s *Server) importMarkdownZip(ctx context.Context, uid int64, b *store.Blog, data []byte) (map[string]int, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("that ZIP file cannot be opened")
	}
	// a Pocket Blog Markdown export carries export.json: that is the whole thing
	for _, f := range zr.File {
		if f.Name == "export.json" {
			rc, err := f.Open()
			if err == nil {
				var doc exportDoc
				err = json.NewDecoder(rc).Decode(&doc)
				rc.Close()
				if err == nil && doc.App == "pocket-blog" {
					s.restoreMedia(ctx, uid, zr, doc.Media)
					return s.importPocket(ctx, uid, doc), nil
				}
			}
		}
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[path.Clean(f.Name)] = f
	}
	counts := map[string]int{"posts": 0, "pictures": 0}
	for _, f := range zr.File {
		lower := strings.ToLower(f.Name)
		if !(strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")) || strings.Contains(f.Name, "__MACOSX") {
			continue
		}
		if f.UncompressedSize64 > 4<<20 {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			continue
		}
		b2, _ := io.ReadAll(rc)
		rc.Close()
		pics := func(src string) string {
			if strings.HasPrefix(src, "http") || strings.HasPrefix(src, "/media/") {
				return src
			}
			clean, _ := pathUnescape(src)
			zf := files[path.Clean(path.Join(path.Dir(f.Name), clean))]
			if zf == nil {
				zf = files[path.Clean(strings.TrimPrefix(clean, "/"))]
			}
			if zf == nil || zf.UncompressedSize64 > uint64(s.cfg.MaxUploadBytes) {
				return src
			}
			r, err := zf.Open()
			if err != nil {
				return src
			}
			img, _ := io.ReadAll(r)
			r.Close()
			m, err := s.savePicture(ctx, uid, img, path.Base(zf.Name), "import", "zip:"+zf.Name, "")
			if err != nil {
				return src
			}
			counts["pictures"]++
			return "/media/" + m.File
		}
		if s.importMarkdownFile(ctx, uid, b, f.Name, string(b2), pics) {
			counts["posts"]++
		}
	}
	return counts, nil
}

// restoreMedia brings back the pictures of a Pocket Blog export with their names.
func (s *Server) restoreMedia(ctx context.Context, uid int64, zr *zip.Reader, ms []store.Media) {
	byName := map[string]*zip.File{}
	for _, f := range zr.File {
		byName[f.Name] = f
	}
	for _, m := range ms {
		if _, err := s.store.MediaByFile(ctx, m.File); err == nil {
			continue
		}
		zf := byName["media/"+m.File]
		if zf == nil || strings.ContainsAny(m.File, "/\\") {
			continue
		}
		r, err := zf.Open()
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(io.LimitReader(r, s.cfg.MaxUploadBytes+1))
		r.Close()
		if _, _, _, _, _, err := preparePicture(data); err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(s.cfg.MediaDir, m.File), data, 0o644); err != nil {
			continue
		}
		m.ID = 0
		_ = s.store.AddMedia(ctx, uid, &m)
	}
}

// importMarkdownFile adds one Markdown file (with or without front matter) as a post.
func (s *Server) importMarkdownFile(ctx context.Context, uid int64, b *store.Blog, name, text string, pics func(string) string) bool {
	fm, body := splitFrontMatter(text)
	ref := "md:" + name
	if _, err := s.store.PostBySource(ctx, uid, b.ID, "import", ref); err == nil {
		return false
	}
	title := fm["title"]
	if title == "" {
		if strings.HasPrefix(strings.TrimSpace(body), "# ") {
			line, _, _ := strings.Cut(strings.TrimSpace(body), "\n")
			title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			body = stripLeadingH1(body, title)
		} else {
			title = strings.TrimSuffix(path.Base(name), path.Ext(name))
		}
	}
	if pics != nil {
		body = reMDImgAll.ReplaceAllStringFunc(body, func(m string) string {
			sm := reMDImgAll.FindStringSubmatch(m)
			return strings.Replace(m, sm[1], pics(sm[1]), 1)
		})
	}
	p := &store.Post{BlogID: b.ID, Kind: "post", Title: title, Slug: fm["slug"], Body: strings.TrimSpace(body), Excerpt: firstNonEmpty(fm["description"], fm["excerpt"], fm["summary"]),
		Tags: fm["tags"], Status: store.StatusDraft, Comments: true, Source: "import", SourceRef: ref}
	if strings.Contains(strings.ToLower(name), "/pages/") || fm["layout"] == "page" || fm["type"] == "page" {
		p.Kind = "page"
	}
	if fm["draft"] != "true" && fm["published"] != "false" && fm["date"] != "" {
		if t, err := parseWhen(fm["date"]); err == nil {
			p.Status, p.PublishedAt = store.StatusPublished, t.UTC().Format(time.RFC3339Nano)
		} else if t, err := time.Parse("2006-01-02 15:04:05 -0700", fm["date"]); err == nil {
			p.Status, p.PublishedAt = store.StatusPublished, t.UTC().Format(time.RFC3339Nano)
		}
	}
	return s.store.CreatePost(ctx, uid, p) == nil
}

// splitFrontMatter reads simple "key: value" front matter (Hugo, Jekyll, Obsidian).
func splitFrontMatter(text string) (map[string]string, string) {
	fm := map[string]string{}
	t := strings.TrimLeft(strings.ReplaceAll(text, "\r\n", "\n"), "\ufeff")
	if !strings.HasPrefix(t, "---\n") {
		return fm, t
	}
	head, body, ok := strings.Cut(t[4:], "\n---")
	if !ok {
		return fm, t
	}
	body = strings.TrimPrefix(body, "\n")
	lastKey := ""
	for _, line := range strings.Split(head, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "- ") && lastKey != "" {
			v := strings.Trim(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "- ")), `"'`)
			fm[lastKey] = strings.TrimPrefix(fm[lastKey]+", "+v, ", ")
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
			var parts []string
			for _, x := range strings.Split(strings.Trim(v, "[]"), ",") {
				parts = append(parts, strings.Trim(strings.TrimSpace(x), `"'`))
			}
			v = strings.Join(parts, ", ")
		} else if uq, err := strconv.Unquote(v); err == nil {
			v = uq
		} else {
			v = strings.Trim(v, `'`)
		}
		if k == "categories" && fm["tags"] != "" {
			fm["tags"] += ", " + v
		} else if k == "categories" {
			fm["tags"] = v
		} else {
			fm[k] = v
		}
		lastKey = k
		if k == "categories" {
			lastKey = "tags"
		}
	}
	return fm, body
}
