package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), Blog as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "blog:read", Name: "See your blog", Description: "Read your blogs and posts (drafts too), the comments readers wrote and how many people read each post — for your journal, dashboards, search and Pocket AI."},
	{Scope: "blog:write", Name: "Write drafts on your blog", Description: "Start a post as a draft for you to finish (\"turn this into a blog post\", a picture from Pocket Photo Lab), with its pictures, and publish it when you ask. It may change only the posts that app started itself."},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(blogs|posts|summary|search|tags|comments|stats|day/\d{4}-\d{2}-\d{2})$`), "blog:read"},
	{"GET", regexp.MustCompile(`^/api/v1/(blogs|posts)/\d+$`), "blog:read"},
	{"POST", regexp.MustCompile(`^/api/v1/posts$`), "blog:write"},
	{"POST", regexp.MustCompile(`^/api/v1/media$`), "blog:write"},
	{"PUT", regexp.MustCompile(`^/api/v1/posts/\d+$`), "blog:write"},
	{"POST", regexp.MustCompile(`^/api/v1/posts/\d+/(publish|unpublish)$`), "blog:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "blog" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
