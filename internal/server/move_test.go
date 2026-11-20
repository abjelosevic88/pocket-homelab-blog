package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/config"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// moveLinks starts Blog behind a fake Core whose pcx_<who>_<export|import|read>
// tokens are Pocket Move links acting for alice@ or bob@.
func moveLinks(t *testing.T) *app {
	t.Helper()
	coreSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/v1/auth/introspect" {
			io.WriteString(w, `{}`)
			return
		}
		var in struct {
			Token string `json:"token"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		parts := strings.SplitN(strings.TrimPrefix(in.Token, "pcx_"), "_", 2)
		if len(parts) != 2 {
			io.WriteString(w, `{"active":false}`)
			return
		}
		scope := map[string]string{"export": core.ScopeMoveExport, "import": core.ScopeMoveImport, "read": "blog:read"}[parts[1]]
		email := parts[0] + "@example.com"
		fmt.Fprintf(w, `{"active":true,"kind":"delegated","scope":%q,"audience":"blog","user":{"id":1,"sub":%q,"email":%q,"name":%q,"role":"member"},"app":{"id":7,"slug":"move","name":"Pocket Move"}}`, scope, email, email, parts[0])
	}))
	t.Cleanup(coreSrv.Close)
	return newApp(t, func(c *config.Config) { c.CoreURL = coreSrv.URL })
}

func (a *app) raw(tok, method, path, ctype string, body []byte) (int, []byte) {
	a.t.Helper()
	req, _ := http.NewRequest(method, a.url+path, bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, b
}

func (a *app) moveImport(tok, name string, data []byte) map[string]any {
	a.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	code, out := a.raw(tok, "POST", "/api/v1/import", mw.FormDataContentType(), buf.Bytes())
	if code != 200 {
		a.t.Fatalf("import %s → %d %s", name, code, out)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	return m
}

func TestMoveLinkScopes(t *testing.T) {
	a := moveLinks(t)
	a.blog(a.bob, "bobs")
	exp, imp, read := "pcx_bob_export", "pcx_bob_import", "pcx_bob_read"
	for _, c := range []struct {
		tok, method, path string
		want              int
	}{
		{exp, "GET", core.PortablePath, 200},
		{exp, "GET", "/api/v1/export.json", 200},
		{exp, "GET", "/api/v1/export/markdown.zip", 200},
		{exp, "GET", "/api/v1/export/site.zip", 200},
		{exp, "POST", "/api/v1/import", 403},
		{exp, "GET", "/api/v1/settings", 403},
		{exp, "GET", "/api/v1/posts", 403},
		{exp, "GET", "/api/v1/backup", 403},
		{imp, "GET", core.PortablePath, 200},
		{imp, "GET", "/api/v1/export.json", 403},
		{imp, "POST", "/api/v1/posts", 403},
		{imp, "POST", "/api/v1/media", 403},
		{imp, "POST", "/api/v1/blogs", 403},
		{imp, "PUT", "/api/v1/settings", 403},
		{read, "GET", core.PortablePath, 403},
		{read, "GET", "/api/v1/export.json", 403},
	} {
		if code, body := a.raw(c.tok, c.method, c.path, "application/json", []byte("{}")); code != c.want {
			t.Errorf("%s %s %s → %d, want %d (%s)", c.tok, c.method, c.path, code, c.want, body)
		}
	}
	_, body := a.raw(exp, "GET", core.PortablePath, "", nil)
	var p core.Portable
	if err := json.Unmarshal(body, &p); err != nil || len(p.Exports) != 3 || len(p.Imports) != 4 || p.Exports[0].Kind != core.KindPocket {
		t.Fatalf("portable: %s", body)
	}
}

// Move brings Alice's blog to Bob, who has no blog yet: the whole ZIP comes in
// once (with its picture), a second run adds nothing, and nothing of one
// person shows up for the other.
func TestMoveImportsAddOnce(t *testing.T) {
	a := moveLinks(t)
	b := a.blog(a.alice, "journal")
	pic := a.upload(a.alice)
	a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "One", "body": "First ![x](/media/" + pic["file"].(string) + ")", "publish": "now"})
	a.ok(a.alice, "POST", "/api/v1/posts", map[string]any{"blog_id": num(b, "id"), "title": "Two", "body": "Second"})

	code, zip := a.raw("pcx_alice_export", "GET", "/api/v1/export/markdown.zip", "", nil)
	if code != 200 || !bytes.HasPrefix(zip, []byte("PK")) {
		t.Fatalf("alice's zip: %d", code)
	}
	if _, js := a.raw("pcx_bob_export", "GET", "/api/v1/export.json", "", nil); strings.Contains(string(js), "journal") || strings.Contains(string(js), "First") {
		t.Fatalf("bob's export holds alice's blog: %s", js)
	}

	first := a.moveImport("pcx_bob_import", "blog.zip", zip)
	if num(first, "blogs") != 1 || num(first, "posts") != 2 {
		t.Fatalf("first import: %v", first)
	}
	second := a.moveImport("pcx_bob_import", "blog.zip", zip)
	if num(second, "blogs")+num(second, "posts")+num(second, "comments")+num(second, "pictures") != 0 {
		t.Fatalf("second import added: %v", second)
	}
	_, js := a.raw("pcx_alice_export", "GET", "/api/v1/export.json", "", nil)
	third := a.moveImport("pcx_bob_import", "blog.json", js)
	if num(third, "blogs")+num(third, "posts") != 0 {
		t.Fatalf("json import after zip added: %v", third)
	}

	md := []byte("---\ntitle: Hello\n---\n\nFrom elsewhere\n")
	if m := a.moveImport("pcx_bob_import", "hello.md", md); num(m, "posts") != 1 {
		t.Fatalf("markdown import: %v", m)
	}
	if m := a.moveImport("pcx_bob_import", "hello.md", md); num(m, "posts") != 0 {
		t.Fatalf("markdown import twice: %v", m)
	}

	if m := a.ok(a.bob, "GET", "/api/v1/posts", nil); num(m, "total") != 3 {
		t.Fatalf("bob's posts: %v", m["total"])
	}
	if m := a.ok(a.alice, "GET", "/api/v1/posts", nil); num(m, "total") != 2 {
		t.Fatalf("alice's posts changed: %v", m["total"])
	}
	if l := a.list(a.alice, "/api/v1/blogs"); len(l) != 1 {
		t.Fatalf("alice's blogs changed: %v", l)
	}
}
