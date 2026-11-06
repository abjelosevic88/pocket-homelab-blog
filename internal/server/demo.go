package server

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"math"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-blog/internal/store"
)

// ---- demo data (DEMO_DATA=true): one lived-in blog ----

type demoPost struct {
	title, slug, tags, excerpt, body string
	daysAgo                          int // published so many days ago; <0 = scheduled in so many days
	status                           string
	cover                            int // palette index, -1 = none
	featured                         bool
}

var demoPalettes = [][3]color.RGBA{
	{{0x1f, 0x4e, 0x79, 255}, {0x5f, 0xa8, 0xd3, 255}, {0xf4, 0xd3, 0x5e, 255}}, // sea and sun
	{{0x3a, 0x5a, 0x40, 255}, {0x8f, 0xb3, 0x6b, 255}, {0xf1, 0xe9, 0xd2, 255}}, // mountains
	{{0x8c, 0x2f, 0x1b, 255}, {0xe0, 0x7a, 0x3f, 255}, {0xfb, 0xe3, 0xb8, 255}}, // kitchen
	{{0x2d, 0x2a, 0x4a, 255}, {0x7c, 0x5c, 0xbf, 255}, {0xf2, 0xc6, 0xde, 255}}, // evening
	{{0x24, 0x3b, 0x55, 255}, {0xc2, 0x58, 0x4b, 255}, {0xf6, 0xe7, 0xcb, 255}}, // old town
	{{0x10, 0x3c, 0x3a, 255}, {0x3e, 0x9e, 0x8a, 255}, {0xe8, 0xf3, 0xe8, 255}}, // river
}

// demoCover paints a calm landscape: sky gradient, a sun, two hill lines.
func demoCover(rng *rand.Rand, pal [3]color.RGBA) []byte {
	w, h := 1200, 675
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	sunX, sunY, sunR := float64(200+rng.Intn(800)), float64(120+rng.Intn(160)), float64(50+rng.Intn(40))
	ph1, ph2 := rng.Float64()*6, rng.Float64()*6
	mix := func(a, b color.RGBA, t float64) color.RGBA {
		return color.RGBA{uint8(float64(a.R)*(1-t) + float64(b.R)*t), uint8(float64(a.G)*(1-t) + float64(b.G)*t), uint8(float64(a.B)*(1-t) + float64(b.B)*t), 255}
	}
	for y := 0; y < h; y++ {
		sky := mix(pal[2], pal[1], float64(y)/float64(h)*0.9)
		for x := 0; x < w; x++ {
			c := sky
			if d := math.Hypot(float64(x)-sunX, float64(y)-sunY); d < sunR {
				c = mix(pal[2], color.RGBA{255, 255, 255, 255}, 0.55)
			}
			hill1 := float64(h)*0.62 + 40*math.Sin(float64(x)/170+ph1) + 18*math.Sin(float64(x)/53+ph2)
			hill2 := float64(h)*0.78 + 30*math.Sin(float64(x)/120+ph2) + 10*math.Sin(float64(x)/37+ph1)
			if float64(y) > hill1 {
				c = mix(pal[1], pal[0], 0.55)
			}
			if float64(y) > hill2 {
				c = pal[0]
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 82})
	return buf.Bytes()
}

func demoPosts() []demoPost {
	return []demoPost{
		{title: "A weekend in Mostar, slowly", slug: "weekend-in-mostar", tags: "travel, bosnia", featured: true, daysAgo: 3, cover: 4,
			excerpt: "Two days, one bridge, far too much coffee — and the best way to see the old town before the buses arrive.",
			body: `We left Sarajevo early on Saturday, before the fog had lifted off the river, and took the train south. The line through the Neretva canyon is one of the most beautiful railway journeys in Europe, and it costs less than a lunch.

## Before the buses arrive

The trick with Mostar is to be on the Old Bridge by eight. The stones are still cool, the shops are only just opening and you can hear the river instead of the crowd. By ten the tour buses are in and it is a different place.

- **Coffee**: sit down at any of the small places along Kujundžiluk and order a Bosnian coffee. Take your time; that is the point.
- **Lunch**: grilled trout by the water. Ask for the tomato salad.
- **Sunset**: walk up to the Partisan Memorial Cemetery. Almost nobody goes, and the view back over the town is quiet and wide.

## The jump

Yes, they still jump. A diver collects money from the crowd first, then stands on the edge for what feels like five minutes. Twenty-one metres into a cold, green river.

> I asked one of them whether he is still afraid. "Every time," he said. "That is why I do it."

## Practical

The train back leaves at 18:20. Buy the ticket at the station; the conductor does not sell them. Bring cash — many small places still do not take cards.`},
		{title: "Grandma's spinach pie (zeljanica)", slug: "grandmas-zeljanica", tags: "recipes, bosnian food", daysAgo: 9, cover: 2,
			excerpt: "The recipe I begged for, written down at last. No exact measures, she said — but I measured anyway.",
			body: `My grandmother never measured anything. "You will feel it," she said, which is not helpful when you are standing in her kitchen with flour up to your elbows. So last month I followed her around with a notebook.

*Serves 6 · about 1 hour 20 minutes*

## Ingredients

- 500 g filo pastry (or home-made, if you are brave)
- 500 g fresh spinach, washed and chopped
- 250 g fresh cheese (or feta, crumbled)
- 3 eggs
- 200 ml sour cream
- 100 ml oil, plus a little for the tin
- 1 teaspoon salt

## Steps

1. Wilt the spinach with the salt for two minutes, squeeze out the water, let it cool.
2. Mix the cheese, eggs, sour cream and spinach.
3. Oil a round tin. Lay two sheets of filo, sprinkle oil, spread some filling, roll loosely and coil it into the tin like a snail.
4. Repeat until the tin is full. Brush the top with oil.
5. Bake at 200 °C for 40–45 minutes, until golden. Cover with a clean towel for ten minutes before cutting.

## The secret

She pours a small glass of hot water mixed with a spoon of sour cream over the pie as soon as it comes out of the oven. It sounds wrong. It is not.`},
		{title: "What I read in October", slug: "read-in-october", tags: "books", daysAgo: 16, cover: 3,
			excerpt: "Four books, one I will not stop talking about, and one I gave up on (sorry).",
			body: `October was a good reading month: long evenings, rain, and a stack of library books with due dates that kept me honest.

## The Overstory — Richard Powers ★★★★★

Nine people, and trees. It took me fifty pages to understand what the book was doing and then I could not put it down. I will never walk through a forest the same way.

> "The best arguments in the world won't change a person's mind. The only thing that can do that is a good story."

## Klara and the Sun — Kazuo Ishiguro ★★★★☆

Quiet and strange and sad. Told by a robot who watches people very carefully and understands them almost right.

## Tomorrow, and Tomorrow, and Tomorrow — Gabrielle Zevin ★★★★☆

About friendship, and making video games, and the years in between. A bit long in the middle.

## Gave up on

A very famous thriller I will not name. Two hundred pages in and I did not care who did it. Life is short.`},
		{title: "Learning to bake sourdough (and failing)", slug: "sourdough", tags: "recipes, learning", daysAgo: 27, cover: 2,
			excerpt: "Three weeks, eleven loaves, one that was actually good. Notes for anyone who wants to start.",
			body: `Every guide makes it sound simple: flour, water, salt, time. Every guide is lying a little.

## Week one: the starter

I named it Boris. Boris did nothing for five days and then, on day six, tried to climb out of the jar. Feed it at the same time every day; it likes a routine.

## Week two: bricks

My first four loaves could have been used to build a wall. The dough was under-proofed — I was following the clock instead of looking at the dough.

## Week three: the good one

Loaf number eleven had an open crumb, a crackling crust and a little ear. I took more photos of it than of my last holiday.

## What I would tell myself

- Look at the dough, not the recipe. It is ready when it has grown by half and wobbles.
- A cold night in the fridge makes everything better.
- Buy a cheap kitchen scale. Cups are not a unit.`},
		{title: "Hiking Prenj: the hardest day I loved", slug: "hiking-prenj", tags: "travel, hiking, bosnia", daysAgo: 41, cover: 1,
			excerpt: "Eleven hours, 1,600 metres up, and a view that made every step worth it.",
			body: `Prenj is called the Bosnian Himalaya by people who have never been to the Himalaya, but on the ridge, with clouds below you, you understand why.

## The route

We started at Bijele vode at six in the morning. The path climbs steadily through beech forest, then out onto limestone that looks like the surface of the moon.

## What to bring

- More water than you think. There is no spring after the hut.
- Proper boots; the rock is sharp.
- A map downloaded on your phone. Signal comes and goes.
- Somebody who knows the way, the first time.

## The top

Zelena glava, 2,155 metres. We ate cheese and bread and nobody said anything for a long time.`},
		{title: "Lisbon on tram 28 (and why you should walk instead)", slug: "lisbon-tram-28", tags: "travel, portugal", daysAgo: 63, cover: 0,
			excerpt: "The famous yellow tram is lovely. It is also full. Here is the walk that follows its route.",
			body: `Everyone wants to ride tram 28, and so everyone waits for it, for an hour, in the sun. We did it once. Then we walked the route instead, and it was the best afternoon of the trip.

## The walk

Start at Martim Moniz and follow the tracks up into Graça. Stop at the miradouro for the view over the river. Then down through Alfama — get lost on purpose — and up again to the cathedral.

## Eat

- Pastéis de nata at a small bakery, warm, with cinnamon.
- Grilled sardines anywhere with plastic chairs outside.
- Ginjinha, the cherry liqueur, in a chocolate cup.

## Tip

Wear shoes with grip. The white stones are polished by centuries of feet and they are slippery even when dry.`},
		{title: "Why I started this blog", slug: "why-i-started", tags: "personal", daysAgo: 88, cover: -1,
			excerpt: "I kept writing long messages to friends about trips and recipes. This seemed like a better home for them.",
			body: `For years I wrote long messages to friends: where to eat in Lisbon, how my grandmother makes pie, which book to read next. The messages got lost in chats. Some friends asked for the same recipe three times.

So here it is: one place for the things I want to remember and share. Mostly travel, food and books. Sometimes something else.

I am not a writer. I hope that is fine.

If you read something here and try it, tell me how it went — the comments are open.`},
		{title: "Coffee in Sarajevo: a small guide", slug: "coffee-in-sarajevo", tags: "bosnia, food", daysAgo: 104, cover: 4,
			excerpt: "Bosnian coffee is not Turkish coffee. Please do not say that in Baščaršija.",
			body: `It comes in a small copper pot called a džezva, with a cup, a sugar cube and usually a piece of rahat lokum. You pour it yourself, slowly, so the foam goes in first.

## How to drink it

Dip the sugar cube into the coffee, bite off the wet part, then sip. Or do not use sugar at all; nobody will judge you. Much.

## Where

Any place in Baščaršija with low tables and old men playing cards. If it has a menu in six languages, walk one more street.`},
		{title: "Notes from a rainy week at home", slug: "rainy-week", tags: "personal", daysAgo: 120, cover: 5,
			excerpt: "Soup, a jigsaw puzzle and the first chapter of something.",
			body: `It rained for seven days. I made soup four times, finished a thousand-piece puzzle of a Dutch canal, and started writing the first chapter of something I am not ready to talk about yet.

Sometimes doing nothing much is exactly what you need.`},
		{title: "Packing light: one bag for ten days", slug: "packing-light", tags: "travel, tips", daysAgo: 140, cover: 0,
			excerpt: "What went in, what I never used, and the one thing I will never travel without again.",
			body: `Ten days in Portugal with one 30-litre backpack. It is easier than you think, and you never wait at a baggage carousel again.

## What went in

- 3 T-shirts, 1 shirt, 1 jumper
- 1 pair of trousers, 1 pair of shorts
- 4 of everything small
- Light rain jacket
- One pair of good walking shoes (worn on the plane)

## Never used

The second pair of shoes. Leave them at home.

## Never again without

A small bar of laundry soap. Wash in the evening, dry by morning.`},
		{title: "Draft: a list of bakeries worth a detour", slug: "bakeries-worth-a-detour", tags: "food, travel", daysAgo: 0, status: "draft", cover: -1,
			body: `Starting a list. Add the one in Trebinje with the cheese pastry.

- Sarajevo: the one near the cathedral, burek before 9
- Lisbon: Manteigaria for nata
- [add more]`},
		{title: "Autumn in the Rakitnica canyon", slug: "rakitnica-canyon", tags: "travel, hiking, bosnia", daysAgo: -2, status: "scheduled", cover: 1,
			excerpt: "Colours, a hanging bridge and a village with no road.",
			body: `The Rakitnica canyon is one of the deepest in Europe, and in late October the beech trees on its walls turn every colour at once.

We walked from Lukomir, the highest village in Bosnia, down the old shepherds' path. It is steep, narrow in places, and absolutely worth it.`},
	}
}

func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if bs, _ := s.store.ListBlogs(ctx, uid); len(bs) > 0 {
		return nil
	}
	rng := rand.New(rand.NewSource(42))
	u, err := s.store.GetUser(ctx, uid)
	if err != nil {
		return err
	}
	avatarImg := demoCover(rng, demoPalettes[3])
	avatar, err := s.savePicture(ctx, uid, avatarImg, "avatar.jpg", "demo", "avatar", "")
	if err != nil {
		return err
	}
	b := &store.Blog{Handle: "slow-roads", Title: "Slow Roads", Tagline: "Trips, kitchens and good books, written down slowly",
		About:      "Hi, I'm " + firstNonEmpty(u.Name, "Ana") + ". I write about the places I walk, the food I learn to cook and the books that stay with me. Mostly Bosnia, sometimes further.",
		AuthorName: firstNonEmpty(u.Name, "Ana"), AuthorBio: "Walks a lot, cooks most days, reads every night. Lives in Sarajevo.", Avatar: avatar.File,
		Theme: "journal", Accent: "#b4532a", Font: "serif", Language: "en", Visibility: "public", Comments: "moderated", PerPage: 8, ShowReading: true,
		Footer: "Words and pictures © Slow Roads. Say hello any time.", Nav: []store.NavLink{},
		Socials: map[string]string{"instagram": "https://instagram.com/", "email": "mailto:hello@example.com"}}
	if err := s.store.CreateBlog(ctx, uid, b); err != nil {
		return err
	}
	n := 0
	cover := func(i int) string {
		if i < 0 {
			return ""
		}
		n++
		m, err := s.savePicture(ctx, uid, demoCover(rng, demoPalettes[i%len(demoPalettes)]), fmt.Sprintf("cover-%d.jpg", n), "demo", fmt.Sprintf("cover-%d", n), "")
		if err != nil {
			return ""
		}
		return m.File
	}
	now := time.Now()
	var published []*store.Post
	for _, d := range demoPosts() {
		p := &store.Post{BlogID: b.ID, Kind: "post", Title: d.title, Slug: d.slug, Body: d.body, Excerpt: d.excerpt, Tags: d.tags, Featured: d.featured,
			Comments: true, Cover: cover(d.cover), CoverAlt: d.title}
		switch {
		case d.status == "draft":
			p.Status = store.StatusDraft
		case d.daysAgo < 0:
			p.Status, p.PublishAt = store.StatusScheduled, now.AddDate(0, 0, -d.daysAgo).Truncate(time.Hour).Add(9*time.Hour).UTC().Format(time.RFC3339Nano)
		default:
			p.Status = store.StatusPublished
			p.PublishedAt = now.AddDate(0, 0, -d.daysAgo).Add(-time.Duration(rng.Intn(600)) * time.Minute).UTC().Format(time.RFC3339Nano)
		}
		if err := s.store.CreatePost(ctx, uid, p); err != nil {
			return err
		}
		if p.Status == store.StatusPublished {
			published = append(published, p)
		}
	}
	pages := []store.Post{
		{Kind: "page", Title: "About", Slug: "about", InMenu: true, MenuOrder: 1, Body: "I'm " + firstNonEmpty(u.Name, "Ana") + ", and this is where I keep the things I want to remember.\n\n" +
			"## What you will find here\n\n- **Travel**: slow trips, mostly in the Balkans, on foot when possible.\n- **Food**: family recipes and the ones I am still learning.\n- **Books**: what I read each month, honestly.\n\n" +
			"## Say hello\n\nWrite a comment under any post, or send me an email. I answer everything, eventually."},
		{Kind: "page", Title: "Now", Slug: "now", InMenu: true, MenuOrder: 2, Body: "*What I am doing now (updated in November).*\n\n- Reading *The Overstory* for the second time.\n- Learning to make proper filo by hand.\n- Planning a winter week in the mountains."},
	}
	for i := range pages {
		p := &pages[i]
		p.BlogID, p.Status, p.PublishedAt, p.Comments = b.ID, store.StatusPublished, now.AddDate(0, 0, -90).UTC().Format(time.RFC3339Nano), false
		if err := s.store.CreatePost(ctx, uid, p); err != nil {
			return err
		}
	}
	// readers: three months, more on fresh posts and weekends
	for _, p := range published {
		pub, _ := time.Parse(time.RFC3339Nano, p.PublishedAt)
		for d := 0; d < 90; d++ {
			day := now.AddDate(0, 0, -d)
			if day.Before(pub.Add(-24 * time.Hour)) {
				break
			}
			age := day.Sub(pub).Hours() / 24
			base := 4.0 + 60*math.Exp(-age/4)
			if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
				base *= 1.3
			}
			if p.Featured {
				base *= 1.6
			}
			n := int(base * (0.6 + rng.Float64()*0.8))
			if n > 0 {
				_ = s.store.SetViews(ctx, uid, p.ID, day.Format("2006-01-02"), n)
			}
		}
	}
	type demoComment struct {
		post          int
		name, body    string
		status        string
		hoursAgo      int
		reply         string
		replyHoursAgo int
	}
	comments := []demoComment{
		{0, "Mirela", "We went last spring and did exactly this — the cemetery at sunset was the highlight. Thank you for the train tip!", "approved", 30, "So glad you went up there! Hardly anyone does.", 26},
		{0, "Tom", "Is the train running all year? Planning a trip in February.", "pending", 5, "", 0},
		{1, "Lejla", "My nana does the hot water trick too!! I thought it was only her.", "approved", 190, "Must be a grandmother thing ❤️", 180},
		{1, "Daniel", "Made this on Sunday with feta. Gone in twenty minutes.", "approved", 120, "", 0},
		{1, "Sara K.", "Can I use frozen spinach?", "pending", 10, "", 0},
		{2, "Ivan", "The Overstory changed how I see trees too. Have you read Bewilderment?", "approved", 300, "Not yet — it's next on my list.", 290},
		{3, "Marko", "Loaf eleven looks amazing. Boris is a great name.", "approved", 500, "", 0},
		{4, "Hana", "We did Prenj in August and ran out of water. Your list is spot on.", "approved", 900, "", 0},
		{5, "Pedro", "As a Lisboeta: yes, walk! And thank you for the kind words about our city.", "approved", 1400, "Obrigada, Pedro!", 1390},
		{0, "cheap-pills-online", "Great post!! Visit my site for best prices http://spam.example http://spam.example http://spam.example", "spam", 12, "", 0},
	}
	for _, dc := range comments {
		if dc.post >= len(published) {
			continue
		}
		p := published[dc.post]
		c := &store.Comment{UserID: uid, PostID: p.ID, Name: dc.name, Body: dc.body, Status: dc.status, CreatedAt: now.Add(-time.Duration(dc.hoursAgo) * time.Hour).UTC().Format(time.RFC3339Nano)}
		if err := s.store.AddComment(ctx, c); err != nil {
			return err
		}
		if dc.reply != "" {
			r := &store.Comment{UserID: uid, PostID: p.ID, ParentID: &c.ID, Name: b.AuthorName, Body: dc.reply, Status: "approved", ByAuthor: true,
				CreatedAt: now.Add(-time.Duration(dc.replyHoursAgo) * time.Hour).UTC().Format(time.RFC3339Nano)}
			if err := s.store.AddComment(ctx, r); err != nil {
				return err
			}
		}
	}
	s.log.Info("demo data seeded", "blog", b.Handle, "posts", len(demoPosts()))
	return nil
}
