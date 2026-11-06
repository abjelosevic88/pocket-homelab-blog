package server

import (
	"bytes"
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
)

// Posts are Markdown. Raw HTML in them is never passed on (a blog is read
// on the same address the writers sign in at), so the only "special"
// things are a YouTube or Vimeo link alone on its line, which becomes the
// video, and pictures, which load lazily.
var md = goldmark.New(
	goldmark.WithExtensions(extension.GFM, extension.Footnote, extension.Typographer),
	goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	goldmark.WithRendererOptions(ghtml.WithXHTML()),
)

var (
	reYouTube = regexp.MustCompile(`^<?https?://(?:www\.|m\.)?(?:youtube\.com/watch\?(?:[^ ]*&)?v=|youtu\.be/|youtube\.com/shorts/)([A-Za-z0-9_-]{6,20})[^ >]*>?$`)
	reVimeo   = regexp.MustCompile(`^<?https?://(?:www\.)?vimeo\.com/(\d{5,12})[^ >]*>?$`)
	reEmbedP  = regexp.MustCompile(`<p>POCKETEMBED(\d+)</p>`)
	reImg     = regexp.MustCompile(`<img src="`)
	reTags    = regexp.MustCompile(`(?s)<[^>]*>`)
	reSpaces  = regexp.MustCompile(`\s+`)
)

// renderMarkdown turns a post into HTML. base, when set, makes picture and
// link addresses that start with "/" absolute (feeds, exports).
func renderMarkdown(src, base string) template.HTML {
	var embeds []string
	lines := strings.Split(src, "\n")
	inFence := false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if m := reYouTube.FindStringSubmatch(t); m != nil {
			embeds = append(embeds, `<div class="embed"><iframe src="https://www.youtube-nocookie.com/embed/`+m[1]+`" title="YouTube video" loading="lazy" allow="accelerometer; encrypted-media; gyroscope; picture-in-picture; fullscreen" allowfullscreen></iframe></div>`)
			lines[i] = fmt.Sprintf("POCKETEMBED%d", len(embeds)-1)
		} else if m := reVimeo.FindStringSubmatch(t); m != nil {
			embeds = append(embeds, `<div class="embed"><iframe src="https://player.vimeo.com/video/`+m[1]+`" title="Vimeo video" loading="lazy" allow="fullscreen; picture-in-picture" allowfullscreen></iframe></div>`)
			lines[i] = fmt.Sprintf("POCKETEMBED%d", len(embeds)-1)
		}
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(strings.Join(lines, "\n")), &buf); err != nil {
		return template.HTML("<p>" + html.EscapeString(src) + "</p>")
	}
	out := buf.String()
	out = reEmbedP.ReplaceAllStringFunc(out, func(m string) string {
		var n int
		fmt.Sscanf(reEmbedP.FindStringSubmatch(m)[1], "%d", &n)
		if n < len(embeds) {
			return embeds[n]
		}
		return ""
	})
	out = reImg.ReplaceAllString(out, `<img loading="lazy" decoding="async" src="`)
	if base != "" {
		out = strings.ReplaceAll(out, `src="/`, `src="`+base+`/`)
		out = strings.ReplaceAll(out, `href="/`, `href="`+base+`/`)
	}
	return template.HTML(out)
}

// plainText is the words of Markdown without its marks.
func plainText(src string) string {
	h := string(renderMarkdown(src, ""))
	h = strings.NewReplacer("</p>", " ", "<br />", " ", "</li>", " ", "</h1>", " ", "</h2>", " ", "</h3>", " ").Replace(h)
	t := html.UnescapeString(reTags.ReplaceAllString(h, ""))
	return strings.TrimSpace(reSpaces.ReplaceAllString(t, " "))
}

// clip shortens text to about n characters at a word boundary.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n]
	cut := string(r)
	if i := strings.LastIndexAny(cut, " \n"); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:-–—") + "…"
}

// autoExcerpt is the excerpt when the writer gave none: the start of the post.
func autoExcerpt(body string) string { return clip(plainText(body), 220) }

// firstImage is the first picture in a post ("" when none).
var reMDImg = regexp.MustCompile(`!\[[^\]]*\]\(([^)\s]+)`)

func firstImage(body string) string {
	if m := reMDImg.FindStringSubmatch(body); m != nil {
		return m[1]
	}
	return ""
}

// ---- words and dates in the blog's language ----

var siteWords = map[string]map[string]string{
	"en": {"read_more": "Read more", "min_read": "min read", "older": "Older posts", "newer": "Newer posts", "home": "Home", "archive": "Archive",
		"tagged": "Posts tagged", "comments": "Comments", "no_comments": "No comments yet. Be the first to write one.", "write_comment": "Write a comment",
		"name": "Your name", "email": "Email (optional, never shown)", "comment": "Your comment", "send": "Send", "held": "Thank you! Your comment will appear once it is approved.",
		"thanks": "Thank you for your comment!", "search": "Search", "search_ph": "Search this blog", "results": "Results for", "nothing": "Nothing here yet.",
		"not_found": "This page does not exist (any more).", "back_home": "Back to the home page", "previous": "Previous", "next": "Next", "rss": "RSS feed",
		"preview": "Preview — this is not published yet. Only people with this link can see it.", "private": "This blog is private.", "written_by": "Written by",
		"reply_author": "Author", "closed": "Comments are closed.", "too_many": "Please wait a little before writing another comment.", "empty": "Please write something.",
		"no_results": "Nothing found.", "all_posts": "All posts", "made_with": "Made with Pocket Blog"},
	"bs": {"read_more": "Pročitaj više", "min_read": "min čitanja", "older": "Stariji tekstovi", "newer": "Noviji tekstovi", "home": "Početna", "archive": "Arhiva",
		"tagged": "Tekstovi s oznakom", "comments": "Komentari", "no_comments": "Još nema komentara. Budite prvi.", "write_comment": "Napišite komentar",
		"name": "Vaše ime", "email": "Email (nije obavezan, nikad se ne prikazuje)", "comment": "Vaš komentar", "send": "Pošalji", "held": "Hvala! Komentar će se pojaviti kad ga autor odobri.",
		"thanks": "Hvala na komentaru!", "search": "Traži", "search_ph": "Pretraži blog", "results": "Rezultati za", "nothing": "Ovdje još nema ničega.",
		"not_found": "Ova stranica ne postoji (više).", "back_home": "Nazad na početnu", "previous": "Prethodni", "next": "Sljedeći", "rss": "RSS",
		"preview": "Pregled — ovo još nije objavljeno. Vide ga samo oni koji imaju ovaj link.", "private": "Ovaj blog je privatan.", "written_by": "Napisao/la",
		"reply_author": "Autor", "closed": "Komentari su zatvoreni.", "too_many": "Sačekajte malo prije sljedećeg komentara.", "empty": "Napišite nešto.",
		"no_results": "Ništa nije pronađeno.", "all_posts": "Svi tekstovi", "made_with": "Napravljeno s Pocket Blogom"},
	"de": {"read_more": "Weiterlesen", "min_read": "Min. Lesezeit", "older": "Ältere Beiträge", "newer": "Neuere Beiträge", "home": "Start", "archive": "Archiv",
		"tagged": "Beiträge mit", "comments": "Kommentare", "no_comments": "Noch keine Kommentare.", "write_comment": "Kommentar schreiben",
		"name": "Dein Name", "email": "E-Mail (optional, wird nie angezeigt)", "comment": "Dein Kommentar", "send": "Senden", "held": "Danke! Dein Kommentar erscheint nach der Freigabe.",
		"thanks": "Danke für deinen Kommentar!", "search": "Suchen", "search_ph": "Blog durchsuchen", "results": "Ergebnisse für", "nothing": "Hier ist noch nichts.",
		"not_found": "Diese Seite gibt es nicht (mehr).", "back_home": "Zur Startseite", "previous": "Vorheriger", "next": "Nächster", "rss": "RSS-Feed",
		"preview": "Vorschau — noch nicht veröffentlicht. Nur Leute mit diesem Link sehen es.", "private": "Dieser Blog ist privat.", "written_by": "Geschrieben von",
		"reply_author": "Autor", "closed": "Kommentare sind geschlossen.", "too_many": "Bitte warte kurz vor dem nächsten Kommentar.", "empty": "Bitte schreib etwas.",
		"no_results": "Nichts gefunden.", "all_posts": "Alle Beiträge", "made_with": "Erstellt mit Pocket Blog"},
}

var siteMonths = map[string][]string{
	"en": {"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"},
	"bs": {"januar", "februar", "mart", "april", "maj", "juni", "juli", "august", "septembar", "oktobar", "novembar", "decembar"},
	"hr": {"siječnja", "veljače", "ožujka", "travnja", "svibnja", "lipnja", "srpnja", "kolovoza", "rujna", "listopada", "studenoga", "prosinca"},
	"de": {"Januar", "Februar", "März", "April", "Mai", "Juni", "Juli", "August", "September", "Oktober", "November", "Dezember"},
}

func langKey(lang string) string {
	switch lang {
	case "bs", "hr", "sr", "cnr":
		return "bs"
	case "de":
		return "de"
	}
	return "en"
}

func word(lang, key string) string {
	if w, ok := siteWords[langKey(lang)][key]; ok {
		return w
	}
	return siteWords["en"][key]
}

// longDate is "5 November 2026" (or "5. novembar 2026.") for a stored time.
func longDate(lang, stamp string) string {
	t, err := time.Parse(time.RFC3339Nano, stamp)
	if err != nil {
		return ""
	}
	t = t.Local()
	switch lang {
	case "bs", "sr", "cnr":
		return fmt.Sprintf("%d. %s %d.", t.Day(), siteMonths["bs"][t.Month()-1], t.Year())
	case "hr":
		return fmt.Sprintf("%d. %s %d.", t.Day(), siteMonths["hr"][t.Month()-1], t.Year())
	case "de":
		return fmt.Sprintf("%d. %s %d", t.Day(), siteMonths["de"][t.Month()-1], t.Year())
	}
	return fmt.Sprintf("%d %s %d", t.Day(), siteMonths["en"][t.Month()-1], t.Year())
}
