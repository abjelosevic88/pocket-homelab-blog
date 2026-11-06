// The overview: your blog's address, how it is doing, what to finish, and
// ideas for posts from your other apps.
import { useCallback } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useData, useOnChange } from '../lib/data'
import { APP_INFO, statusLabel } from '../lib/blog'
import { ago, fmtDate } from '../lib/format'
import type { Post, SourceItem, Stats } from '../lib/types'
import { openCreate } from '../components/Layout'
import { Icon } from '../components/icons'
import { CopyField, Loading, useAsync } from '../components/ui'

export default function Home() {
  const { blog } = useData()
  const id = blog?.id || 0
  const posts = useAsync(() => api.get<{ items: Post[]; total: number }>(`${V1}/posts?blog=${id}&kind=post&limit=200`), [id])
  const stats = useAsync(() => api.get<Stats>(`${V1}/stats?blog=${id}&days=7`), [id])
  const ideas = useAsync(() => api.get<SourceItem[]>(`${V1}/ideas`), [])
  const reload = useCallback(() => { posts.reload(); stats.reload() }, [posts.reload, stats.reload]) // eslint-disable-line react-hooks/exhaustive-deps
  useOnChange(reload)
  if (!blog) return <Loading />
  const all = posts.data?.items || []
  const drafts = all.filter(p => p.status === 'draft').sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, 4)
  const scheduled = all.filter(p => p.status === 'scheduled')
  const published = all.filter(p => p.status === 'published').slice(0, 5)
  const fresh = blog.posts === 0
  const change = stats.data ? stats.data.views - stats.data.previous : 0
  return (
    <div className="stack loose page">
      <section className="blog-hero card">
        <div className="blog-hero-main">
          <span className="blog-hero-mark" style={{ background: blog.accent }}>{blog.avatar ? <img src={'/media/' + blog.avatar} alt="" /> : blog.title.slice(0, 1).toUpperCase()}</span>
          <div className="col grow">
            <h1>{blog.title}</h1>
            {blog.tagline && <p className="muted">{blog.tagline}</p>}
          </div>
        </div>
        <div className="blog-hero-addr">
          <span className="muted small">Your blog’s address{blog.visibility === 'private' ? ' (private: only you can open it)' : blog.visibility === 'unlisted' ? ' (unlisted: search engines are asked to stay away)' : ''}</span>
          <CopyField value={blog.url} />
        </div>
        <div className="row wrap">
          <button type="button" className="btn primary" onClick={() => openCreate('post')}><Icon name="pen" size={15} />Write a post</button>
          <button type="button" className="btn" onClick={() => openCreate('from-app')}><Icon name="layers" size={15} />Start from your apps</button>
          <a className="btn" href={blog.url} target="_blank" rel="noreferrer"><Icon name="external" size={15} />Open your blog</a>
        </div>
      </section>

      {fresh && <section className="card pad">
        <h2 className="card-h">Getting started</h2>
        <ol className="steps">
          <li className="done"><Icon name="check-circle" size={16} /><span><b>Your blog exists.</b> It has a name and an address.</span></li>
          <li><Icon name="palette" size={16} /><span><b>Make it yours:</b> pick a look, a colour, your picture. <Link to="/look">Look & details</Link></span></li>
          <li><Icon name="user" size={16} /><span><b>Say who you are</b> on an About page. <Link to="/write?kind=page">Write it</Link></span></li>
          <li><Icon name="pen" size={16} /><span><b>Write your first post.</b> Stuck? “Help me write” in the editor gives you a first version from a sentence. <button type="button" className="link-btn" onClick={() => openCreate('post')}>Start</button></span></li>
          <li><Icon name="share" size={16} /><span><b>Share your address</b> with friends — they can follow you with the RSS link at the bottom of every page.</span></li>
        </ol>
      </section>}

      <div className="tiles">
        <Link to="/readers" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="chart" size={14} /></span>Readers this week</div>
          <div className="tile-val tnum">{stats.data ? stats.data.views.toLocaleString() : '–'}{stats.data && stats.data.previous > 0 && <span className={`tile-unit ${change >= 0 ? 'ok' : 'bad'}`}>{change >= 0 ? '+' : ''}{change.toLocaleString()}</span>}</div></Link>
        <Link to="/posts?status=published" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="list" size={14} /></span>Published posts</div><div className="tile-val tnum">{blog.posts}</div></Link>
        <Link to="/posts?status=draft" className="tile"><div className="tile-head"><span className="tile-ico"><Icon name="edit" size={14} /></span>Drafts</div><div className="tile-val tnum">{blog.drafts}</div></Link>
        <Link to="/comments" className={`tile ${blog.pending_comments ? 'tile-warn' : ''}`}><div className="tile-head"><span className="tile-ico"><Icon name="message" size={14} /></span>Comments waiting</div><div className="tile-val tnum">{blog.pending_comments}</div></Link>
      </div>

      <div className="two-grid">
        <section className="card">
          <div className="card-head"><div className="grow"><h2>Keep writing</h2></div><Link to="/posts?status=draft" className="btn sm ghost">All drafts</Link></div>
          {posts.loading && !posts.data ? <Loading /> : drafts.length === 0 && scheduled.length === 0 ? <p className="muted pad">No drafts. Everything you start is kept here until you publish it.</p> :
            <div className="list">{[...scheduled, ...drafts].map(p => <PostRow key={p.id} p={p} />)}</div>}
        </section>
        <section className="card">
          <div className="card-head"><div className="grow"><h2>Ideas from your apps</h2><p className="muted small">Things you did lately that would make a good post.</p></div><button type="button" className="btn sm ghost" onClick={() => ideas.reload()} aria-label="Look again" title="Look again"><Icon name="refresh" size={14} /></button></div>
          {!ideas.data ? <Loading label="Looking in your apps…" /> : ideas.data.length === 0 ? <p className="muted pad">Nothing new right now. Finished a book, came back from a trip, loved a restaurant? It shows up here. <button type="button" className="link-btn" onClick={() => openCreate('from-app')}>Pick something yourself</button></p> :
            <div className="list">{ideas.data.map(it => {
              const a = APP_INFO[it.app!]
              return (
                <div key={it.app + it.id} className="list-row">
                  <span className="list-ico"><Icon name={a?.icon || 'sparkles'} size={16} /></span>
                  <div className="row-main grow"><span className="row-title ellipsis">{it.title}</span><span className="muted small ellipsis">{it.why}{it.date ? ` · ${fmtDate(it.date)}` : ''}</span></div>
                  <button type="button" className="btn sm" onClick={() => openCreate('from-app', it.app, { id: it.id, title: it.title })}>Write about it</button>
                </div>
              )
            })}</div>}
        </section>
      </div>

      {published.length > 0 && <section className="card">
        <div className="card-head"><div className="grow"><h2>Latest on your blog</h2></div><Link to="/posts?status=published" className="btn sm ghost">All posts</Link></div>
        <div className="list">{published.map(p => <PostRow key={p.id} p={p} />)}</div>
      </section>}
    </div>
  )
}

export function PostRow({ p }: { p: Post }) {
  return (
    <Link to={`/posts/${p.id}`} className="list-row post-row">
      {p.cover ? <img className="row-thumb" src={'/media/' + p.cover} alt="" loading="lazy" /> : <span className="list-ico"><Icon name={p.kind === 'page' ? 'file' : 'pen'} size={16} /></span>}
      <div className="row-main grow">
        <span className="row-title ellipsis">{p.title || <span className="muted">Untitled</span>}{p.featured && <Icon name="pin" size={13} className="muted" />}</span>
        <span className="muted small ellipsis">{p.excerpt || '—'}</span>
      </div>
      <div className="row-side">
        <span className={`pill st-${p.status}`}>{p.status === 'published' ? <Icon name="globe" size={12} /> : p.status === 'scheduled' ? <Icon name="clock" size={12} /> : <Icon name="edit" size={12} />}{p.status === 'draft' ? 'Draft' : p.status === 'scheduled' ? 'Scheduled' : 'Live'}</span>
        <span className="muted small" title={statusLabel(p)}>{p.status === 'published' ? `${p.views.toLocaleString()} readers` : `edited ${ago(p.updated_at)}`}</span>
      </div>
    </Link>
  )
}
