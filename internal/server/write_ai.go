package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/ai"
	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

// ---- "Help me write": Pocket AI (or the person's own key) as an editor ----
//
// Every answer is JSON and every text is Markdown in the blog's language.
// Nothing is changed by itself: the editor shows the result and the writer
// keeps it or not.

var langNames = map[string]string{"en": "English", "bs": "Bosnian", "hr": "Croatian", "sr": "Serbian (Latin script)", "de": "German", "fr": "French",
	"es": "Spanish", "it": "Italian", "pt": "Portuguese", "nl": "Dutch", "sl": "Slovenian", "mk": "Macedonian", "pl": "Polish", "cs": "Czech",
	"sv": "Swedish", "da": "Danish", "no": "Norwegian", "fi": "Finnish", "tr": "Turkish", "el": "Greek", "hu": "Hungarian", "ro": "Romanian", "sq": "Albanian"}

func langName(code string) string {
	if n, ok := langNames[code]; ok {
		return n
	}
	if code == "" {
		return "the language the text is written in"
	}
	return code
}

type aiWriteIn struct {
	Action      string `json:"action"`
	BlogID      int64  `json:"blog_id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Text        string `json:"text"`        // a selection, or the idea
	Instruction string `json:"instruction"` // "more friendly", "for beginners"…
	Language    string `json:"language"`    // translate: into this
}

type aiWriteOut struct {
	Title          string   `json:"title,omitempty"`
	Body           string   `json:"body,omitempty"`
	Text           string   `json:"text,omitempty"`
	Titles         []string `json:"titles,omitempty"`
	Excerpt        string   `json:"excerpt,omitempty"`
	SEODescription string   `json:"seo_description,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Tagline        string   `json:"tagline,omitempty"`
	About          string   `json:"about,omitempty"`
	Handle         string   `json:"handle,omitempty"`
	Provider       string   `json:"provider,omitempty"`
}

var aiActions = []string{"draft", "outline", "improve", "fix", "shorter", "longer", "tone", "continue", "rewrite", "titles", "summary", "translate", "setup"}

const aiRules = `You help a person who is not a professional writer with their blog. Rules:
- Write in %s unless told otherwise. Keep the person's own voice, facts and opinions; never invent facts about them (places, people, dates, numbers) beyond what they gave you. Where a fact is missing, leave a short placeholder in square brackets like [add the name of the hotel].
- Write Markdown only: ## and ### headings, paragraphs, - lists, **bold**, *italic*, > quotes, links. No # (level 1) heading — the title is separate. No HTML.
- Keep any Markdown pictures ![…](…) and links exactly as they are.
- Answer with JSON only, exactly the fields asked for.%s`

func (s *Server) aiSystem(lang, voice string) string {
	extra := ""
	if v := strings.TrimSpace(voice); v != "" {
		extra = "\n- How the writer wants to sound: " + clip(v, 300) + "."
	}
	return fmt.Sprintf(aiRules, langName(lang), extra)
}

func (s *Server) handleAIWrite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in aiWriteIn
	if err := decode(r, &in); err != nil || !oneOf(in.Action, aiActions) {
		writeErr(w, http.StatusBadRequest, "unknown action")
		return
	}
	st, _ := s.store.GetSettings(ctx, uid)
	if !st.AIWriting {
		writeErr(w, http.StatusBadRequest, "Writing help is switched off in Settings → AI")
		return
	}
	cl, provider, why := s.aiFor(ctx, uid, st)
	if cl == nil {
		writeErr(w, http.StatusServiceUnavailable, firstNonEmpty(why, "No AI is set up: connect Pocket AI or add a key in Settings → AI"))
		return
	}
	lang := ""
	if b, err := s.defaultBlog(ctx, uid, in.BlogID); err == nil {
		lang = b.Language
	}
	select {
	case s.aiSem <- struct{}{}:
		defer func() { <-s.aiSem }()
	case <-ctx.Done():
		return
	}
	out, err := s.aiWrite(ctx, cl, st, lang, in)
	if err != nil {
		writeErr(w, http.StatusBadGateway, "The AI could not do it: "+err.Error())
		return
	}
	out.Provider = provider
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) aiWrite(ctx context.Context, cl *ai.Client, st store.Settings, lang string, in aiWriteIn) (*aiWriteOut, error) {
	sys := s.aiSystem(lang, st.AIVoice)
	post := func() string {
		return fmt.Sprintf("Title: %s\n\nPost:\n%s", firstNonEmpty(strings.TrimSpace(in.Title), "(none yet)"), clip(in.Body, 60000))
	}
	need := func(v, msg string) error {
		if strings.TrimSpace(v) == "" {
			return errors.New(msg)
		}
		return nil
	}
	var prompt string
	switch in.Action {
	case "draft":
		if err := need(in.Text+in.Body, "say what the post is about"); err != nil {
			return nil, err
		}
		prompt = "Write a complete, warm and readable blog post (400–900 words, with a few ## headings) from what the writer gave you. " +
			"Return {\"title\": \"…\", \"body\": \"…markdown…\", \"excerpt\": \"one or two sentences for lists\", \"tags\": [\"2–5 short lowercase tags\"]}.\n\n" +
			"What the post should be about:\n" + clip(in.Text, 8000)
		if strings.TrimSpace(in.Body) != "" {
			prompt += "\n\nNotes and material to use:\n" + clip(in.Body, 40000)
		}
		if in.Instruction != "" {
			prompt += "\n\nAlso: " + clip(in.Instruction, 500)
		}
	case "outline":
		if err := need(in.Text+in.Title, "say what the post is about"); err != nil {
			return nil, err
		}
		prompt = "Make a plan for a blog post: ## headings for its parts, each with 2–4 short bullet points of what to say there, so the writer can fill it in. " +
			"Return {\"title\": \"a suggested title\", \"body\": \"…markdown…\"}.\n\nThe post is about: " + clip(firstNonEmpty(in.Text, in.Title), 4000)
	case "improve":
		if err := need(in.Body, "write something first"); err != nil {
			return nil, err
		}
		prompt = "Make this post read better: clearer sentences, better flow, fix mistakes, keep its length, structure, meaning and voice. Return {\"body\": \"…\"}.\n\n" + post()
	case "fix":
		if err := need(in.Body, "write something first"); err != nil {
			return nil, err
		}
		prompt = "Correct only spelling, grammar and punctuation in this post. Change nothing else. Return {\"body\": \"…\"}.\n\n" + post()
	case "shorter":
		if err := need(in.Body, "write something first"); err != nil {
			return nil, err
		}
		prompt = "Make this post about half as long, keeping what matters most and the voice. Return {\"body\": \"…\"}.\n\n" + post()
	case "longer":
		if err := need(in.Body, "write something first"); err != nil {
			return nil, err
		}
		prompt = "Make this post richer and about half again as long: more detail, examples and feeling, using only what is already there (leave [placeholders] for facts you would need). Return {\"body\": \"…\"}.\n\n" + post()
	case "tone":
		if err := need(in.Body, "write something first"); err != nil {
			return nil, err
		}
		prompt = "Rewrite this post so that it sounds: " + clip(firstNonEmpty(in.Instruction, "friendly and personal"), 300) + ". Keep its content and structure. Return {\"body\": \"…\"}.\n\n" + post()
	case "continue":
		if err := need(in.Body, "write something first"); err != nil {
			return nil, err
		}
		prompt = "Continue this post from where it stops with one to three more paragraphs in the same voice. Return only the new part as {\"text\": \"…\"}.\n\n" + post()
	case "rewrite":
		if err := need(in.Text, "select some text first"); err != nil {
			return nil, err
		}
		prompt = "Rewrite this part of a blog post: " + clip(firstNonEmpty(in.Instruction, "make it read better"), 300) + ". Return {\"text\": \"…\"}.\n\nThe part:\n" + clip(in.Text, 20000) +
			"\n\nThe whole post, for context only:\n" + clip(in.Body, 20000)
	case "titles":
		if err := need(in.Body+in.Title, "write something first"); err != nil {
			return nil, err
		}
		prompt = "Suggest 5 different titles for this post: short, honest, inviting, no clickbait, no quotes around them. Return {\"titles\": [\"…\"]}.\n\n" + post()
	case "summary":
		if err := need(in.Body, "write something first"); err != nil {
			return nil, err
		}
		prompt = "Write, for this post: an excerpt (1–2 sentences that make people want to read it, shown in lists), a description for search engines (at most 155 characters) and 2–5 short lowercase tags. " +
			"Return {\"excerpt\": \"…\", \"seo_description\": \"…\", \"tags\": [\"…\"]}.\n\n" + post()
	case "translate":
		if err := need(in.Body, "write something first"); err != nil {
			return nil, err
		}
		to := langName(firstNonEmpty(in.Language, "en"))
		sys = s.aiSystem(firstNonEmpty(in.Language, "en"), st.AIVoice)
		prompt = "Translate this post into " + to + ", naturally, as a native writer would. Return {\"title\": \"…\", \"body\": \"…\"}.\n\n" + post()
	case "setup":
		if err := need(in.Text, "say in a sentence what the blog is about"); err != nil {
			return nil, err
		}
		prompt = "A person is starting a blog and says what it is about. Suggest a short memorable name, a one-line tagline, a warm 2–3 sentence 'about' text in first person, " +
			"and an address handle (2–30 lowercase letters, numbers and dashes). Return {\"title\": \"…\", \"tagline\": \"…\", \"about\": \"…\", \"handle\": \"…\"}.\n\nWhat they said: " + clip(in.Text, 2000)
		if in.Language != "" {
			sys = s.aiSystem(in.Language, st.AIVoice)
		}
	}
	var out aiWriteOut
	if err := cl.JSON(ctx, sys, prompt, &out); err != nil {
		return nil, err
	}
	out.Body = cleanAIMarkdown(out.Body)
	out.Text = cleanAIMarkdown(out.Text)
	for i := range out.Tags {
		out.Tags[i] = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(out.Tags[i]), "#"))
	}
	if out.Handle != "" {
		out.Handle = store.Slugify(out.Handle, 30)
	}
	switch in.Action {
	case "titles":
		if len(out.Titles) == 0 {
			return nil, errors.New("no titles came back")
		}
	case "summary":
		if out.Excerpt == "" && out.SEODescription == "" {
			return nil, errors.New("no summary came back")
		}
	case "continue", "rewrite":
		if out.Text == "" {
			return nil, errors.New("nothing came back")
		}
	case "setup":
		if out.Title == "" {
			return nil, errors.New("nothing came back")
		}
	default:
		if out.Body == "" {
			return nil, errors.New("nothing came back")
		}
	}
	return &out, nil
}

// cleanAIMarkdown drops a ```markdown fence and a leading # title the AI may add.
func cleanAIMarkdown(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i > 0 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	if strings.HasPrefix(s, "# ") {
		if i := strings.Index(s, "\n"); i > 0 {
			s = s[i+1:]
		} else {
			s = ""
		}
	}
	return strings.TrimSpace(s)
}
