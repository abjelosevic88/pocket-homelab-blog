// Left column: write (and the write menu), the blog you are working on with
// its address, its parts — posts, pages, comments, pictures, readers, look —
// and your other blogs. The Pocket Calendar sidebar.
import { useState } from 'react'
import { Link, NavLink, useNavigate } from 'react-router-dom'
import { useApp } from '../lib/app-context'
import { useData } from '../lib/data'
import { Icon } from './icons'
import { createItems, type CreateKind } from './Layout'
import { type Anchor, Menu, rectOf } from './ui'

export function Sidebar({ onCreate, onNavigate }: { onCreate: (k: CreateKind) => void; onNavigate: () => void }) {
  const { status, settings } = useApp()
  const { blogs, blog, setBlogID } = useData()
  const navigate = useNavigate()
  const [menu, setMenu] = useState<Anchor | null>(null)
  const title = settings?.title || status?.title || 'Pocket Blog'
  const nav: { to: string; label: string; icon: string; end?: boolean; count?: number; warn?: boolean; hint?: string }[] = [
    { to: '/', label: 'Overview', icon: 'home', end: true },
    { to: '/posts', label: 'Posts', icon: 'list', count: blog?.drafts, hint: 'Drafts to finish' },
    { to: '/pages', label: 'Pages', icon: 'file' },
    { to: '/comments', label: 'Comments', icon: 'message', count: blog?.pending_comments, warn: true, hint: 'Comments waiting for your OK' },
    { to: '/pictures', label: 'Pictures', icon: 'image' },
    { to: '/readers', label: 'Readers', icon: 'chart' },
    { to: '/look', label: 'Look & details', icon: 'palette' },
  ]
  return (
    <aside className="sidebar" aria-label="Pocket Blog">
      <div className="sb-brand">
        <img src="/favicon.svg" alt="" className="sb-logo" />
        <span className="sb-title">{title !== 'Pocket Blog' ? title : <>Pocket <b>Blog</b></>}</span>
        {status?.version && <span className="sb-version" title={`Version ${status.version}`}>{/^v?\d+\.\d+/.test(status.version) ? 'v' + status.version.replace(/^v/, '').replace(/[-+].*$/, '') : status.version}</span>}
      </div>
      <div className="create-split">
        <button type="button" className="btn primary create-btn" onClick={() => onCreate('post')} title="Write a new post (N)">
          <Icon name="pen" size={15} strokeWidth={2.2} />Write
        </button>
        <button type="button" className="btn primary create-more" aria-label="More ways to start" aria-haspopup="menu" aria-expanded={!!menu} onClick={e => setMenu(menu ? null : rectOf(e.currentTarget))}>
          <Icon name="chevron-down" size={15} />
        </button>
      </div>
      {menu && <Menu anchor={menu} items={createItems(onCreate)} onClose={() => setMenu(null)} width={260} />}

      {blog && <a className="sb-blog" href={blog.url} target="_blank" rel="noreferrer" title="Open your blog as readers see it">
        <span className="sb-blog-dot" style={{ background: blog.accent }} />
        <span className="grow sb-blog-text"><b className="ellipsis">{blog.title}</b><span className="muted small ellipsis">{blog.url.replace(/^https?:\/\//, '')}</span></span>
        <Icon name="external" size={14} />
      </a>}

      <nav className="sb-nav" aria-label="Sections">
        {nav.map(n => (
          <NavLink key={n.to} to={n.to} end={n.end} onClick={onNavigate} className={({ isActive }) => `sb-nav-item ${isActive ? 'active' : ''}`}>
            <Icon name={n.icon} size={16} />{n.label}
            {!!n.count && <span className={`sb-count ${n.warn ? 'warn' : ''}`} title={n.hint}>{n.count}</span>}
          </NavLink>
        ))}
      </nav>

      {blogs && blogs.length > 0 && <section className="sb-sec">
        <h4 className="sb-h">{blogs.length > 1 ? 'Your blogs' : 'Your blog'}</h4>
        {blogs.map(b => (
          <button key={b.id} type="button" className={`sb-row sb-list ${b.id === blog?.id ? 'active' : ''}`} onClick={() => { setBlogID(b.id); onNavigate(); navigate('/') }}>
            <span className="sb-list-ico" aria-hidden="true"><span className="sb-blog-dot" style={{ background: b.accent }} /></span>
            <span className="grow ellipsis">{b.title}</span>
            <span className="muted small tnum">{b.posts}</span>
          </button>
        ))}
        <Link to="/start" className="sb-row sb-list" onClick={onNavigate}><span className="sb-list-ico" aria-hidden="true"><Icon name="plus" size={14} /></span><span className="grow">Start another blog</span></Link>
      </section>}

      <div className="sb-foot">
        <Link to="/settings" className="sb-foot-link" onClick={onNavigate}><Icon name="settings" size={15} />Settings</Link>
        <button type="button" className="sb-foot-link" onClick={() => { onNavigate(); window.dispatchEvent(new CustomEvent('pbg:shortcuts')) }}><Icon name="keyboard" size={15} />Shortcuts</button>
      </div>
    </aside>
  )
}
