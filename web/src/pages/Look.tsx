// Look & details: name, look, colour, letters, address, who can read it,
// the menu, links and footer — with the real blog shown next to it as you
// change things.
import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useData } from '../lib/data'
import { ACCENTS, LANGUAGES, SOCIALS, THEMES } from '../lib/blog'
import type { Blog, Theme } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { PicturePicker } from '../components/PicturePicker'
import { Confirm, Field, Loading, Segmented, SettingRow, Switch, useDebounce, useIsMobile, useToast } from '../components/ui'

/** A tiny drawing of a look, for picking one. */
export function ThemeThumb({ theme, accent, font }: { theme: Theme; accent: string; font: string }) {
  return (
    <span className={`thumb th-${theme} f-${font}`} style={{ '--a': accent } as React.CSSProperties} aria-hidden="true">
      <span className="th-head"><i /><b /></span>
      {theme === 'magazine' ? <><span className="th-hero" /><span className="th-cards"><i /><i /><i /></span></>
        : theme === 'minimal' ? <span className="th-lines">{[0, 1, 2, 3, 4].map(i => <i key={i} />)}</span>
        : <span className="th-posts">{[0, 1].map(i => <span key={i}><i className="th-pic" /><b /><em /></span>)}</span>}
    </span>
  )
}

export default function Look() {
  const { blog } = useData()
  const navigate = useNavigate()
  const { toast } = useToast()
  const mobile = useIsMobile()
  const [f, setF] = useState<Blog | null>(null)
  const [busy, setBusy] = useState(false)
  const [picker, setPicker] = useState(false)
  const [del, setDel] = useState(false)
  const [preview, setPreview] = useState(!mobile)
  const [check, setCheck] = useState<{ ok: boolean; reason: string; suggestion?: string } | null>(null)
  useEffect(() => { if (blog && (!f || f.id !== blog.id)) setF({ ...blog }) }, [blog]) // eslint-disable-line react-hooks/exhaustive-deps
  useCrumb(undefined)
  const dh = useDebounce(f?.handle || '', 300)
  useEffect(() => {
    if (!f || !dh || dh === blog?.handle) { setCheck(null); return }
    api.get<{ ok: boolean; reason: string; suggestion?: string }>(`${V1}/blogs/check-handle?handle=${encodeURIComponent(dh)}&except=${f.id}`).then(setCheck).catch(() => setCheck(null))
  }, [dh]) // eslint-disable-line react-hooks/exhaustive-deps
  const look = useDebounce(f ? `${f.theme}|${f.accent}|${f.font}` : '', 250)
  if (!f || !blog) return <Loading />
  const set = (patch: Partial<Blog>) => setF({ ...f, ...patch })
  const dirty = JSON.stringify(f) !== JSON.stringify(blog)
  const save = async () => {
    setBusy(true)
    try { await api.put<Blog>(`${V1}/blogs/${f.id}`, f); announceChange(); toast('Saved — your blog shows it now') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => {
    try { await api.del(`${V1}/blogs/${f.id}`); announceChange(); toast('Blog deleted'); navigate('/') } catch (e) { toast((e as Error).message, 'err') }
  }
  const [theme, accent, font] = look.split('|')
  const src = `/b/${blog.handle}/?theme=${theme}&accent=${encodeURIComponent(accent || '')}&font=${font}`
  const nav = f.nav || []
  return (
    <div className={`look page ${preview ? 'with-preview' : ''}`}>
      <div className="look-form stack loose">
        <div className="page-head">
          <div className="grow"><h1 className="page-title">Look & details</h1><p className="muted">How your blog looks and what it says about you. Nothing changes for readers until you press Save.</p></div>
          {!preview && <button type="button" className="btn" onClick={() => setPreview(true)}><Icon name="eye" size={15} />Show preview</button>}
        </div>

        <Section title="Look" icon="palette">
          <div className="theme-grid">
            {THEMES.map(t => (
              <button key={t.id} type="button" className={`theme-card ${f.theme === t.id ? 'on' : ''}`} onClick={() => set({ theme: t.id })} aria-pressed={f.theme === t.id}>
                <ThemeThumb theme={t.id} accent={f.accent} font={f.font} /><b>{t.name}</b><span className="muted small">{t.help}</span>
              </button>
            ))}
          </div>
          <div className="field"><span className="label">Colour</span>
            <div className="swatches">{ACCENTS.map(c => <button key={c} type="button" className={`swatch ${f.accent === c ? 'on' : ''}`} style={{ background: c }} onClick={() => set({ accent: c })} aria-label={c} />)}
              <label className="swatch custom" title="Any colour"><input type="color" value={f.accent} onChange={e => set({ accent: e.target.value })} /><Icon name="plus" size={14} /></label></div></div>
          <div className="field"><span className="label">Letters</span>
            <div className="row wrap"><button type="button" className={`font-pick serif ${f.font === 'serif' ? 'on' : ''}`} onClick={() => set({ font: 'serif' })}>Classic<span>like a book</span></button><button type="button" className={`font-pick sans ${f.font === 'sans' ? 'on' : ''}`} onClick={() => set({ font: 'sans' })}>Modern<span>like a website</span></button></div></div>
        </Section>

        <Section title="Name and words" icon="text">
          <Field label="Name"><input value={f.title} onChange={e => set({ title: e.target.value })} /></Field>
          <Field label="Line under the name"><input value={f.tagline} onChange={e => set({ tagline: e.target.value })} /></Field>
          <Field label="About you, on the home page" help="A few sentences. **Bold** and [links](https://…) work."><textarea rows={4} value={f.about} onChange={e => set({ about: e.target.value })} /></Field>
          <div className="two-col tight">
            <Field label="Your name as the writer"><input value={f.author_name} onChange={e => set({ author_name: e.target.value })} /></Field>
            <Field group label="Your picture">
              <div className="row">{f.avatar ? <img className="avatar-pick" src={'/media/' + f.avatar} alt="" /> : <span className="avatar-pick empty" style={{ background: f.accent }}>{f.title.slice(0, 1)}</span>}
                <button type="button" className="btn sm" onClick={() => setPicker(true)}>{f.avatar ? 'Change' : 'Choose'}</button>{f.avatar && <button type="button" className="btn sm ghost" onClick={() => set({ avatar: '' })}>Remove</button>}</div>
            </Field>
          </div>
          <Field label="A line about you under each post"><input value={f.author_bio} onChange={e => set({ author_bio: e.target.value })} placeholder="Walks a lot, cooks most days, reads every night." /></Field>
          <Field group label="Language" help="For “Read more”, dates, and the AI’s help."><Select ariaLabel="Language" value={f.language} onChange={v => set({ language: v })} options={LANGUAGES.some(l => l.value === f.language) ? LANGUAGES : [{ value: f.language, label: f.language }, ...LANGUAGES]} /></Field>
        </Section>

        <Section title="Address" icon="link">
          <Field label="Address on this server" help={`Readers open ${blog.url.replace(/\/b\/.*/, '')}/b/${f.handle || '…'}. Changing it breaks links people already have.`}>
            <input value={f.handle} onChange={e => set({ handle: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '') })} />
          </Field>
          {check && !check.ok && <p className="bad small">{check.reason}{check.suggestion && <> · <button type="button" className="link-btn" onClick={() => set({ handle: check.suggestion! })}>use {check.suggestion}</button></>}</p>}
          <Field label="Your own web address (optional)" help="Bought a name like myblog.com? Point it at this server (your admin adds it to the web server), type it here, and the blog opens there."><input value={f.domain} onChange={e => set({ domain: e.target.value })} placeholder="myblog.com" /></Field>
        </Section>

        <Section title="Readers" icon="users">
          <SettingRow title="Who can read it" help={f.visibility === 'public' ? 'Everyone who has the address; search engines may list it.' : f.visibility === 'unlisted' ? 'Everyone with the address; search engines are asked not to list it.' : 'Only you, signed in. Drafts you share by link still open.'}>
            <Segmented size="sm" value={f.visibility} onChange={v => set({ visibility: v })} options={[{ value: 'public', label: 'Everyone' }, { value: 'unlisted', label: 'With the link' }, { value: 'private', label: 'Only me' }]} />
          </SettingRow>
          <SettingRow title="Comments" help={f.comments === 'moderated' ? 'Readers write; you approve each one before it shows.' : f.comments === 'open' ? 'Comments show at once (ones full of links still wait for you).' : 'No comment form under posts.'}>
            <Segmented size="sm" value={f.comments} onChange={v => set({ comments: v })} options={[{ value: 'moderated', label: 'I approve' }, { value: 'open', label: 'Open' }, { value: 'off', label: 'Off' }]} />
          </SettingRow>
          <SettingRow title="Posts per page"><input type="number" min={3} max={50} className="num-sm" value={f.per_page} onChange={e => set({ per_page: Number(e.target.value) || 10 })} /></SettingRow>
          <SettingRow title="Show reading time" help="“4 min read” next to each post."><Switch checked={f.show_reading} onChange={v => set({ show_reading: v })} label="Reading time" /></SettingRow>
        </Section>

        <Section title="Menu and links" icon="list">
          <p className="muted small">Pages you mark “In the menu” are listed first. Add other links here (your shop, a newsletter…).</p>
          {nav.map((l, i) => (
            <div key={i} className="nav-row">
              <input value={l.label} placeholder="Name" onChange={e => set({ nav: nav.map((x, j) => j === i ? { ...x, label: e.target.value } : x) })} />
              <input value={l.url} placeholder="https://…" onChange={e => set({ nav: nav.map((x, j) => j === i ? { ...x, url: e.target.value } : x) })} />
              <button type="button" className="btn ghost icon-only" aria-label="Remove" onClick={() => set({ nav: nav.filter((_, j) => j !== i) })}><Icon name="x" size={15} /></button>
            </div>
          ))}
          <div className="row"><button type="button" className="btn sm" onClick={() => set({ nav: [...nav, { label: '', url: '' }] })}><Icon name="plus" size={14} />Add a link</button></div>
          <details className="wp-more"><summary>Your profiles elsewhere (at the bottom of every page)</summary>
            <div className="two-col tight">{SOCIALS.map(s => <Field key={s.key} label={s.label}><input value={(f.socials || {})[s.key] || ''} placeholder={s.placeholder} onChange={e => set({ socials: { ...(f.socials || {}), [s.key]: e.target.value } })} /></Field>)}</div>
          </details>
          <Field label="Footer text" help="At the bottom of every page. Empty: © the year and your blog’s name."><input value={f.footer} onChange={e => set({ footer: e.target.value })} /></Field>
        </Section>

        <div className="save-bar">
          <button type="button" className="btn primary" onClick={save} disabled={busy || !dirty}>{busy ? 'Saving…' : 'Save'}</button>
          <button type="button" className="btn" onClick={() => setF({ ...blog })} disabled={!dirty}>Undo changes</button>
          <span className="spacer" />
          <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}>Delete this blog…</button>
        </div>
      </div>

      {preview && <aside className="look-preview">
        <div className="lp-head"><span className="muted small">Preview{dirty ? ' (look only, until you save)' : ''}</span><span className="spacer" /><a className="btn sm ghost" href={blog.url} target="_blank" rel="noreferrer"><Icon name="external" size={14} />Open</a><button type="button" className="btn sm ghost icon-only" onClick={() => setPreview(false)} aria-label="Hide preview"><Icon name="x" size={14} /></button></div>
        <iframe key={src} title="Your blog" src={src} />
      </aside>}

      {picker && <PicturePicker title="Your picture" onClose={() => setPicker(false)} onPick={m => { set({ avatar: m.file }); setPicker(false) }} />}
      {del && <Confirm title={`Delete “${blog.title}”?`} message="The blog, all its posts, pages and comments are deleted for good. Pictures stay in your pictures. Export it first (Settings → Backup & data) if you may want it back." confirmLabel="Delete the blog" onCancel={() => setDel(false)} onConfirm={remove} />}
    </div>
  )
}

function Section({ title, icon, children }: { title: string; icon: string; children: React.ReactNode }) {
  return (
    <section className="set-section">
      <div className="set-section-head"><span className="set-app-ico"><Icon name={icon} size={17} /></span><div className="grow"><h3>{title}</h3></div></div>
      <div className="set-section-body stack">{children}</div>
    </section>
  )
}
