package store

import (
	"encoding/json"
	"strings"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Post statuses.
const (
	StatusDraft     = "draft"     // only the writer sees it
	StatusScheduled = "scheduled" // goes out by itself at publish_at
	StatusPublished = "published" // on the blog
)

// Themes are the looks a blog can have.
var Themes = []string{"classic", "magazine", "minimal", "journal"}

// Blog is one blog: name, address, look and who may read it.
type Blog struct {
	ID          int64             `json:"id"`
	UserID      int64             `json:"-"`
	Handle      string            `json:"handle"`
	Title       string            `json:"title"`
	Tagline     string            `json:"tagline"`
	About       string            `json:"about"`
	AuthorName  string            `json:"author_name"`
	AuthorBio   string            `json:"author_bio"`
	Avatar      string            `json:"avatar"`
	Theme       string            `json:"theme"`
	Accent      string            `json:"accent"`
	Font        string            `json:"font"`
	Language    string            `json:"language"`
	Visibility  string            `json:"visibility"` // public | unlisted | private
	Comments    string            `json:"comments"`   // off | moderated | open
	PerPage     int               `json:"per_page"`
	ShowReading bool              `json:"show_reading"`
	Footer      string            `json:"footer"`
	Nav         []NavLink         `json:"nav"`
	Socials     map[string]string `json:"socials"`
	Domain      string            `json:"domain"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`

	// Filled for the person's own lists.
	URL        string `json:"url,omitempty"`
	Posts      int    `json:"posts"`
	Drafts     int    `json:"drafts"`
	Pending    int    `json:"pending_comments"`
	LastPostAt string `json:"last_post_at,omitempty"`
}

// NavLink is an extra link in a blog's menu.
type NavLink struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Post is a post or a page.
type Post struct {
	ID             int64  `json:"id"`
	UserID         int64  `json:"-"`
	BlogID         int64  `json:"blog_id"`
	Kind           string `json:"kind"` // post | page
	Title          string `json:"title"`
	Slug           string `json:"slug"`
	Body           string `json:"body"`
	Excerpt        string `json:"excerpt"`
	Cover          string `json:"cover"`
	CoverAlt       string `json:"cover_alt"`
	Tags           string `json:"tags"`
	Status         string `json:"status"`
	PublishAt      string `json:"publish_at"`
	PublishedAt    string `json:"published_at"`
	Featured       bool   `json:"featured"`
	InMenu         bool   `json:"in_menu"`
	MenuOrder      int    `json:"menu_order"`
	Comments       bool   `json:"comments"`
	SEODescription string `json:"seo_description"`
	PreviewKey     string `json:"preview_key"`
	Words          int    `json:"words"`
	Views          int    `json:"views"`
	Source         string `json:"source"`
	SourceRef      string `json:"source_ref"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`

	// Filled for the writer.
	URL       string `json:"url,omitempty"`
	CoverURL  string `json:"cover_url,omitempty"`
	CommentsN int    `json:"comments_count"`
	PendingN  int    `json:"pending_count"`
}

// Media is an uploaded picture.
type Media struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	File      string `json:"file"`
	Name      string `json:"name"`
	Mime      string `json:"mime"`
	Bytes     int64  `json:"bytes"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Alt       string `json:"alt"`
	Source    string `json:"source"`
	SourceRef string `json:"source_ref"`
	CreatedAt string `json:"created_at"`
	URL       string `json:"url"`
	Used      int    `json:"used"` // posts it is in
}

// Comment is what a reader wrote under a post.
type Comment struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	PostID    int64  `json:"post_id"`
	ParentID  *int64 `json:"parent_id"`
	Name      string `json:"name"`
	Email     string `json:"email,omitempty"`
	Body      string `json:"body"`
	Status    string `json:"status"` // pending | approved | spam
	ByAuthor  bool   `json:"by_author"`
	IPHash    string `json:"-"`
	CreatedAt string `json:"created_at"`

	PostTitle string `json:"post_title,omitempty"`
	PostURL   string `json:"post_url,omitempty"`
	BlogID    int64  `json:"blog_id,omitempty"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Writing.
	Autosave    bool   `json:"autosave"`
	AIWriting   bool   `json:"ai_writing"`   // the "Help me write" buttons
	AIVoice     string `json:"ai_voice"`     // how the AI should sound: "friendly and personal"
	NotesFolder string `json:"notes_folder"` // Pocket Notes folder offered as post ideas ("" = all notes)

	// Notifications.
	NotifyComment   bool   `json:"notify_comment"`   // a reader wrote a comment
	NotifyPublished bool   `json:"notify_published"` // a scheduled post went out
	NotifyDigest    bool   `json:"notify_digest"`    // morning: comments waiting, readers yesterday
	NotifyHour      int    `json:"notify_hour"`
	NotifyURL       string `json:"notify_url"`
	NotifyFormat    string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

// SplitTags turns "a, b ,c" into clean lowercase tags.
func SplitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, t := range strings.Split(s, ",") {
		t = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#")))
		if t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

// JoinTags is the stored form of tags.
func JoinTags(s string) string { return strings.Join(SplitTags(s), ", ") }

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
