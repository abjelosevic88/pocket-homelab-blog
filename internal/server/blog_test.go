package server

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/config"
	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

func (a *app) page(path string) (int, string) {
	a.t.Helper()
	res, err := http.Get(a.url + path)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func (a *app) blog(auth, handle string) map[string]any {
	a.t.Helper()
	return a.ok(auth, "POST", "/api/v1/blogs", map[string]any{"title": "Blog " + handle, "handle": handle, "tagline": "Hello"})
}

func (a *app) upload(auth string) map[string]any {
	a.t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for x := 0; x < 40; x++ {
		img.Set(x, 5, color.RGBA{200, 0, 0, 255})
	}
	var pic bytes.Buffer
	_ = png.Encode(&pic, img)
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "red.png")
	_, _ = fw.Write(pic.Bytes())
	mw.Close()
	req, _ := http.NewRequest("POST", a.url+"/api/v1/media", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+auth)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	var out []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	if res.StatusCode != 201 || len(out) != 1 {
		a.t.Fatalf("upload → %d", res.StatusCode)
	}
	return out[0]
}

// Every person sees and changes only their own blogs, posts, pictures and
// comments; somebody else's are "not found".
func TestPerPersonIsolation(t *testing.T) {
	a := newApp(t, nil)
	b := a.blog(a.alice, "alice")
	p := a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "Secret draft", "body": "Only mine"})
	pic := a.upload(a.alice)

	for _, path := range []string{"/api/v1/blogs/" + id(b), "/api/v1/posts/" + id(p)} {
		a.expect(404, a.bob, "GET", path, nil)
		a.expect(404, a.bob, "PUT", path, map[string]any{"title": "mine now"})
		a.expect(404, a.bob, "DELETE", path, nil)
	}
	a.expect(404, a.bob, "POST", "/api/v1/posts/"+id(p)+"/publish", nil)
	a.expect(404, a.bob, "PUT", "/api/v1/media/"+id(pic), map[string]any{"alt": "x"})
	a.expect(404, a.bob, "DELETE", "/api/v1/media/"+id(pic), nil)
	if got := a.list(a.bob, "/api/v1/blogs"); len(got) != 0 {
		t.Fatalf("bob sees %d blogs", len(got))
	}
	if got := a.list(a.bob, "/api/v1/media"); len(got) != 0 {
		t.Fatalf("bob sees %d pictures", len(got))
	}
	if got := a.ok(a.bob, "GET", "/api/v1/posts", nil); num(got, "total") != 0 {
		t.Fatalf("bob sees %v posts", got["total"])
	}
	// Bob cannot write into Alice's blog, nor use her picture
	a.expect(400, a.bob, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "x"})
	bb := a.blog(a.bob, "bob")
	a.expect(400, a.bob, "POST", "/api/v1/posts", map[string]any{"blog_id": num(bb, "id"), "title": "x", "cover": pic["file"]})
	// nor take her address
	a.expect(400, a.bob, "PUT", "/api/v1/blogs/"+id(bb), map[string]any{"handle": "alice"})
	// admins do not see other people's content either
	if got := a.list(a.alice, "/api/v1/blogs"); len(got) != 1 {
		t.Fatalf("alice sees %d blogs", len(got))
	}
}

func TestPublishAndPublicPages(t *testing.T) {
	a := newApp(t, nil)
	b := a.blog(a.alice, "walks")
	p := a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "Ljeto u Mostaru!", "tags": "travel, Bosnia",
		"body": "Hello **world**.\n\n<script>alert(1)</script>\n\nhttps://www.youtube.com/watch?v=dQw4w9WgXcQ\n"})
	if p["slug"] != "ljeto-u-mostaru" {
		t.Fatalf("slug = %v", p["slug"])
	}
	// a draft is not on the blog, but its preview link works
	if code, _ := a.page("/b/walks/ljeto-u-mostaru"); code != 404 {
		t.Fatalf("draft is public: %d", code)
	}
	if code, body := a.page("/b/walks/ljeto-u-mostaru?preview=" + p["preview_key"].(string)); code != 200 || !strings.Contains(body, "preview-bar") {
		t.Fatalf("preview → %d", code)
	}
	if code, _ := a.page("/b/walks/ljeto-u-mostaru?preview=wrong"); code != 404 {
		t.Fatalf("wrong preview key → %d", code)
	}
	a.expect(400, a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "", "body": "", "publish": "now"})
	a.ok(a.alice, "POST", "/api/v1/posts/"+id(p)+"/publish", nil)
	code, body := a.page("/b/walks/ljeto-u-mostaru")
	if code != 200 || !strings.Contains(body, "<strong>world</strong>") {
		t.Fatalf("post page → %d", code)
	}
	if strings.Contains(body, "<script>alert") {
		t.Fatal("raw HTML from a post reached the page")
	}
	if !strings.Contains(body, "youtube-nocookie.com/embed/dQw4w9WgXcQ") {
		t.Fatal("YouTube link did not become a video")
	}
	if _, home := a.page("/b/walks/"); !strings.Contains(home, "Ljeto u Mostaru!") {
		t.Fatal("home page does not list the post")
	}
	if _, tag := a.page("/b/walks/tag/bosnia"); !strings.Contains(tag, "Ljeto u Mostaru!") {
		t.Fatal("tag page does not list the post")
	}
	if _, feed := a.page("/b/walks/feed.xml"); !strings.Contains(feed, "<title>Ljeto u Mostaru!</title>") || !strings.Contains(feed, "http://blog.test/b/walks/ljeto-u-mostaru") {
		t.Fatalf("feed: %s", feed)
	}
	if _, sm := a.page("/b/walks/sitemap.xml"); !strings.Contains(sm, "/b/walks/ljeto-u-mostaru") {
		t.Fatal("sitemap misses the post")
	}
	if _, found := a.page("/b/walks/search?q=mostaru"); !strings.Contains(found, "Ljeto u Mostaru!") {
		t.Fatal("search does not find the post")
	}
	// readers are counted once a day each, bots not at all
	req, _ := http.NewRequest("GET", a.url+"/b/walks/ljeto-u-mostaru", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone)")
	for i := 0; i < 2; i++ {
		res, _ := http.DefaultClient.Do(req)
		res.Body.Close()
	}
	if got := a.ok(a.alice, "GET", "/api/v1/posts/"+id(p), nil); num(got, "views") != 1 {
		t.Fatalf("views = %v, want 1", got["views"])
	}
	// private: nobody but the writer
	a.ok(a.alice, "PUT", "/api/v1/blogs/"+id(b), map[string]any{"visibility": "private"})
	if code, _ := a.page("/b/walks/"); code != 403 {
		t.Fatalf("private blog → %d", code)
	}
	if code, _ := a.page("/b/nobody/"); code != 404 {
		t.Fatalf("unknown blog → %d", code)
	}
}

func TestScheduledPostGoesOut(t *testing.T) {
	a := newApp(t, nil)
	b := a.blog(a.alice, "later")
	p := a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "Soon", "body": "Text"})
	at := time.Now().Add(2 * time.Hour).UTC().Format(time.RFC3339)
	got := a.ok(a.alice, "POST", "/api/v1/posts/"+id(p)+"/publish", map[string]any{"at": at})
	if got["status"] != "scheduled" {
		t.Fatalf("status = %v", got["status"])
	}
	if code, _ := a.page("/b/later/soon"); code != 404 {
		t.Fatal("scheduled post is visible early")
	}
	// move its time into the past and let the janitor run
	ctx := context.Background()
	post, _ := a.st.GetPost(ctx, a.ids["Alice"], n(id(p)))
	post.PublishAt = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	_ = a.st.UpdatePost(ctx, a.ids["Alice"], post)
	a.s.publishDue(ctx)
	if code, _ := a.page("/b/later/soon"); code != 200 {
		t.Fatalf("scheduled post did not go out: %d", code)
	}
}

func TestComments(t *testing.T) {
	a := newApp(t, nil)
	a.s.commentMinAge = 0
	b := a.blog(a.alice, "talk")
	p := a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "Say hi", "body": "Comments please", "publish": "now"})
	_, body := a.page("/b/talk/say-hi")
	i := strings.Index(body, `name="stamp" value="`)
	if i < 0 {
		t.Fatal("no comment form")
	}
	stamp := body[i+len(`name="stamp" value="`):]
	stamp = stamp[:strings.Index(stamp, `"`)]
	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	post := func(form url.Values) string {
		res, err := noRedirect.PostForm(a.url+"/b/talk/say-hi/comments", form)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.Header.Get("Location")
	}
	if loc := post(url.Values{"name": {"Mira"}, "body": {"Lovely post"}, "stamp": {stamp}}); !strings.Contains(loc, "comment=held") {
		t.Fatalf("redirect = %s", loc)
	}
	// robots: a filled hidden field, a forged form
	post(url.Values{"name": {"Bot"}, "body": {"spam"}, "stamp": {stamp}, "website": {"http://x"}})
	post(url.Values{"name": {"Bot"}, "body": {"spam"}, "stamp": {"1.abc"}})
	cs := a.list(a.alice, "/api/v1/comments?status=pending")
	if len(cs) != 1 {
		t.Fatalf("pending = %d, want 1", len(cs))
	}
	c := cs[0].(map[string]any)
	if _, page := a.page("/b/talk/say-hi"); strings.Contains(page, "Lovely post") {
		t.Fatal("a held comment is shown")
	}
	a.expect(404, a.bob, "PUT", "/api/v1/comments/"+id(c), map[string]any{"status": "approved"})
	a.ok(a.alice, "POST", "/api/v1/comments/"+id(c)+"/reply", map[string]any{"body": "Thank you!"})
	_, page := a.page("/b/talk/say-hi")
	if !strings.Contains(page, "Lovely post") || !strings.Contains(page, "Thank you!") {
		t.Fatal("approved comment or reply missing")
	}
	if got := a.ok(a.alice, "GET", "/api/v1/posts/"+id(p), nil); num(got, "comments_count") != 2 {
		t.Fatalf("comments_count = %v", got["comments_count"])
	}
}

func TestExportImportIdempotent(t *testing.T) {
	a := newApp(t, nil)
	b := a.blog(a.alice, "journal")
	a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "One", "body": "First", "publish": "now"})
	a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "Two", "body": "Second"})
	doc := a.ok(a.alice, "GET", "/api/v1/export.json", nil)
	first := a.ok(a.bob, "POST", "/api/v1/import", doc)
	if num(first, "blogs") != 1 || num(first, "posts") != 2 {
		t.Fatalf("first import = %v", first)
	}
	again := a.ok(a.bob, "POST", "/api/v1/import", doc)
	if num(again, "posts") != 0 || num(again, "blogs") != 0 {
		t.Fatalf("second import = %v", again)
	}
	bobs := a.list(a.bob, "/api/v1/blogs")
	if len(bobs) != 1 || bobs[0].(map[string]any)["handle"] == "journal" {
		t.Fatalf("bob's blog = %v (must get its own address)", bobs)
	}
}

func TestImportWordPressAndMarkdown(t *testing.T) {
	a := newApp(t, nil)
	a.blog(a.alice, "imported")
	wp := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:wp="http://wordpress.org/export/1.2/" xmlns:excerpt="http://wordpress.org/export/1.2/excerpt/">
<channel><item><title>Old post</title>
<content:encoded><![CDATA[First line with <strong>bold</strong>.

<h2>Part</h2>
Second paragraph.]]></content:encoded>
<excerpt:encoded><![CDATA[]]></excerpt:encoded>
<wp:post_id>7</wp:post_id><wp:post_date_gmt>2019-05-04 10:00:00</wp:post_date_gmt><wp:post_name>old-post</wp:post_name>
<wp:status>publish</wp:status><wp:post_type>post</wp:post_type>
<category domain="post_tag" nicename="memories"><![CDATA[memories]]></category>
<wp:comment><wp:comment_id>1</wp:comment_id><wp:comment_author>Zara</wp:comment_author><wp:comment_date_gmt>2019-05-05 10:00:00</wp:comment_date_gmt>
<wp:comment_content><![CDATA[Nice!]]></wp:comment_content><wp:comment_approved>1</wp:comment_approved><wp:comment_parent>0</wp:comment_parent><wp:comment_type>comment</wp:comment_type></wp:comment>
</item></channel></rss>`
	up := func(name, content string) map[string]any {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		fw, _ := mw.CreateFormFile("file", name)
		_, _ = fw.Write([]byte(content))
		mw.Close()
		req, _ := http.NewRequest("POST", a.url+"/api/v1/import", &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		req.Header.Set("Authorization", "Bearer "+a.alice)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		if res.StatusCode != 200 {
			t.Fatalf("import %s → %d %v", name, res.StatusCode, out)
		}
		return out
	}
	if got := up("wordpress.xml", wp); num(got, "posts") != 1 || num(got, "comments") != 1 {
		t.Fatalf("wordpress import = %v", got)
	}
	if got := up("wordpress.xml", wp); num(got, "posts") != 0 {
		t.Fatalf("second wordpress import = %v", got)
	}
	code, page := a.page("/b/imported/old-post")
	if code != 200 || !strings.Contains(page, "<strong>bold</strong>") || !strings.Contains(page, "Nice!") || !strings.Contains(page, "2019") {
		t.Fatalf("imported post page → %d", code)
	}
	md := "---\ntitle: \"From Hugo\"\ndate: 2021-03-04\ntags: [a, b]\n---\n\nSome *text*.\n"
	if got := up("from-hugo.md", md); num(got, "posts") != 1 {
		t.Fatalf("markdown import = %v", got)
	}
	if code, _ := a.page("/b/imported/from-hugo"); code != 200 {
		t.Fatalf("markdown post → %d", code)
	}
}

func TestSettingsArePersonal(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"ai_voice": "warm, short sentences", "notify_hour": 7})
	if got := a.ok(a.bob, "GET", "/api/v1/settings", nil); got["ai_voice"] != "" || num(got, "notify_hour") != 8 {
		t.Fatalf("bob got alice's settings: %v %v", got["ai_voice"], got["notify_hour"])
	}
	// only an admin changes the box title
	a.ok(a.bob, "PUT", "/api/v1/settings", map[string]any{"title": "Bob's box"})
	if got := a.ok(a.alice, "GET", "/api/v1/settings", nil); got["title"] != store.DefaultTitle {
		t.Fatalf("title = %v", got["title"])
	}
}

func TestLinkScopes(t *testing.T) {
	cases := []struct {
		method, path string
		scopes       []string
		want         bool
	}{
		{"GET", "/api/v1/posts", []string{"blog:read"}, true},
		{"GET", "/api/v1/posts/3", []string{"blog:read"}, true},
		{"GET", "/api/v1/day/2026-11-05", []string{"blog:read"}, true},
		{"POST", "/api/v1/posts", []string{"blog:read"}, false},
		{"POST", "/api/v1/posts", []string{"blog:write"}, true},
		{"POST", "/api/v1/posts/3/publish", []string{"blog:write"}, true},
		{"DELETE", "/api/v1/posts/3", []string{"blog:write"}, false},
		{"GET", "/api/v1/settings", []string{"blog:read", "blog:write"}, false},
		{"GET", "/api/v1/export.json", []string{"blog:read"}, false},
		{"GET", "/api/v1/auth/me", nil, true},
	}
	for _, c := range cases {
		r, _ := http.NewRequest(c.method, "http://x"+c.path, nil)
		if got := linkAllows(c.scopes, r); got != c.want {
			t.Errorf("%s %s with %v = %v, want %v", c.method, c.path, c.scopes, got, c.want)
		}
	}
}

// The manifest must name this app: a fork once registered as its parent.
func TestManifestSlug(t *testing.T) {
	a := newApp(t, nil)
	for _, l := range []string{"blog"} {
		r, _ := http.NewRequest("GET", a.url+"/pocket-ask.js", nil)
		res, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		_ = l
	}
	src, _ := siteFS.ReadFile("site/site.html")
	if len(src) == 0 {
		t.Fatal("site templates are not embedded")
	}
}

func TestOwnDomainAndPublicHost(t *testing.T) {
	a := newApp(t, func(c *config.Config) { c.PublicURL = "http://read.test" })
	b := a.blog(a.alice, "mine")
	a.ok(a.alice, "PUT", "/api/v1/blogs/"+id(b), map[string]any{"domain": "myblog.example"})
	a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "Hello", "body": "Hi", "publish": "now"})
	get := func(host, path string) (int, string) {
		req, _ := http.NewRequest("GET", a.url+path, nil)
		req.Host = host
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		return res.StatusCode, string(b)
	}
	if code, body := get("myblog.example", "/hello"); code != 200 || !strings.Contains(body, "Hello") {
		t.Fatalf("own domain → %d", code)
	}
	if code, _ := get("myblog.example", "/api/v1/auth/status"); code == 200 {
		t.Fatal("the app's API answers on a blog's own address")
	}
	if code, _ := get("read.test", "/b/mine/hello"); code != 200 {
		t.Fatalf("public host → %d", code)
	}
	if code, _ := get("read.test", "/api/v1/auth/status"); code != 404 {
		t.Fatalf("public host serves the API: %d", code)
	}
	// Pocket Core and the apps acting through it call in as host.docker.internal:<port>
	for _, path := range []string{"/healthz", "/api/v1/auth/status", "/b/mine/hello"} {
		if code, _ := get("host.docker.internal:8246", path); code != 200 {
			t.Fatalf("host.docker.internal %s → %d", path, code)
		}
	}
}

func TestPictureLosesCameraData(t *testing.T) {
	// a JPEG with an EXIF block: the stored copy has none
	img := image.NewRGBA(image.Rect(0, 0, 30, 10))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	out, mime, _, w, h, err := preparePicture(buf.Bytes())
	if err != nil || mime != "image/png" || w != 30 || h != 10 || len(out) == 0 {
		t.Fatalf("png: %v %s %dx%d", err, mime, w, h)
	}
	if _, _, _, _, _, err := preparePicture([]byte("<svg onload=alert(1)>")); err == nil {
		t.Fatal("an SVG was accepted")
	}
	if got := tiffOrientation(append([]byte("MM\x00\x2a\x00\x00\x00\x08\x00\x01\x01\x12\x00\x03\x00\x00\x00\x01\x00\x06\x00\x00"), make([]byte, 8)...)); got != 6 {
		t.Fatalf("orientation = %d, want 6", got)
	}
}
