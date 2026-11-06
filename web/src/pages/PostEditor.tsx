// Writing a post (or a page): a big title, a picture on top, the text as it
// will read, and the details on the side. Everything saves by itself; the
// post reaches readers only when you press Publish.
import { useCallback, useEffect, useRef, useState } from 'react'
import { Link, useLocation, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { APP_INFO, localInput, readingMinutes, slugify, statusLabel, wordCount } from '../lib/blog'
import { fmtDateTime } from '../lib/format'
import type { AppSlug, Media, Post } from '../lib/types'
import { useCrumb } from '../components/Layout'
import { Editor, type EditorHandle } from '../components/Editor'
import { WriteHelp } from '../components/WriteHelp'
import { PicturePicker, uploadPictures } from '../components/PicturePicker'
import { Icon } from '../components/icons'
import { Confirm, CopyField, Field, Loading, type MenuItem, MenuButton, Modal, Switch, copyText, useIsMobile, useToast } from '../components/ui'

type Draft = Pick<Post, 'title' | 'body' | 'slug' | 'excerpt' | 'cover' | 'cover_alt' | 'tags' | 'featured' | 'in_menu' | 'menu_order' | 'comments' | 'seo_description'> & { kind: Post['kind'] }

const empty = (kind: Post['kind']): Draft => ({ kind, title: '', body: '', slug: '', excerpt: '', cover: '', cover_alt: '', tags: '', featured: false, in_menu: kind === 'page', menu_order: 0, comments: kind === 'post', seo_description: '' })

export default function PostEditor() {
  const { id } = useParams()
  const [sp] = useSearchParams()
  const loc = useLocation()
  const navigate = useNavigate()
  const { settings } = useApp()
  const { blog } = useData()
  const { toast } = useToast()
  const mobile = useIsMobile()
  const editor = useRef<EditorHandle>(null)

  const [post, setPost] = useState<Post | null>(null)
  const [d, setD] = useState<Draft | null>(null)
  const [docKey, setDocKey] = useState<string>('')
  const [err, setErr] = useState('')
  const [state, setState] = useState<'saved' | 'dirty' | 'saving' | 'error'>('saved')
  const [slugTouched, setSlugTouched] = useState(false)
  const [panel, setPanel] = useState(!mobile)
  const [help, setHelp] = useState<{ selection: string } | null>(null)
  const [picker, setPicker] = useState(false)
  const [publishing, setPublishing] = useState(false)
  const [confirm, setConfirm] = useState<'delete' | 'unpublish' | null>(null)
  const createdHere = useRef<number>(0)
  const saving = useRef<Promise<Post | null> | null>(null)
  const latest = useRef<Draft | null>(null)
  latest.current = d

  // open a post, or start a new one
  useEffect(() => {
    if (!id) {
      setPost(null); setD(empty(sp.get('kind') === 'page' ? 'page' : 'post')); setDocKey('new-' + Date.now()); setState('saved'); setSlugTouched(false)
      return
    }
    if (Number(id) === createdHere.current) return
    let alive = true
    setErr('')
    api.get<Post>(`${V1}/posts/${id}`).then(p => {
      if (!alive) return
      setPost(p); setD({ ...p }); setDocKey(String(p.id)); setState('saved'); setSlugTouched(true)
    }).catch(e => alive && setErr((e as Error).message))
    return () => { alive = false }
  }, [id, loc.key]) // eslint-disable-line react-hooks/exhaustive-deps
  useCrumb(d ? (d.title || (d.kind === 'page' ? 'New page' : 'New post')) : undefined)

  const change = (patch: Partial<Draft>) => {
    setD(cur => {
      if (!cur) return cur
      const next = { ...cur, ...patch }
      if ('title' in patch && !slugTouched && (!post || post.status !== 'published')) next.slug = slugify(patch.title || '')
      return next
    })
    setState('dirty')
  }

  // save: the first time it creates the post, after that it updates it
  const save = useCallback(async (): Promise<Post | null> => {
    if (saving.current) await saving.current
    const cur = latest.current
    if (!cur || !blog) return null
    if (!post && !cur.title.trim() && !cur.body.trim()) return null
    setState('saving')
    const run = (async () => {
      try {
        const p = post
          ? await api.put<Post>(`${V1}/posts/${post.id}`, cur)
          : await api.post<Post>(`${V1}/posts`, { ...cur, blog_id: blog.id })
        if (!post) { createdHere.current = p.id; navigate(`/posts/${p.id}`, { replace: true }) }
        setPost(p)
        setD(x => x ? { ...x, slug: p.slug } : x)
        setState(latest.current === cur ? 'saved' : 'dirty')
        announceChange()
        return p
      } catch (e) { setState('error'); toast((e as Error).message, 'err'); return null } finally { saving.current = null }
    })()
    saving.current = run
    return run
  }, [post, blog, navigate, toast])

  // autosave a moment after the last change
  useEffect(() => {
    if (state !== 'dirty' || settings?.autosave === false) return
    const t = setTimeout(() => { save() }, 1500)
    return () => clearTimeout(t)
  }, [d, state, save, settings?.autosave])
  // ⌘S saves now; leaving with unsaved changes asks first
  useEffect(() => {
    const k = (e: KeyboardEvent) => { if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 's') { e.preventDefault(); save() } }
    const bye = (e: BeforeUnloadEvent) => { if (state === 'dirty' || state === 'saving') e.preventDefault() }
    window.addEventListener('keydown', k)
    window.addEventListener('beforeunload', bye)
    return () => { window.removeEventListener('keydown', k); window.removeEventListener('beforeunload', bye) }
  }, [save, state])

  const upload = useCallback(async (f: File) => { const [m] = await uploadPictures([f]); return m.url }, [])
  const preview = async () => {
    const w = window.open('', '_blank')
    const p = (state === 'saved' ? post : await save()) || post
    if (p?.url && w) w.location.href = p.url
    else w?.close()
  }
  const act = async (path: string, body?: unknown) => {
    await save()
    const cur = createdHere.current && !post ? createdHere.current : post?.id
    if (!cur) { toast('Write something first', 'err'); return null }
    try { const p = await api.post<Post>(`${V1}/posts/${cur}/${path}`, body); setPost(p); setState('saved'); announceChange(); return p } catch (e) { toast((e as Error).message, 'err'); return null }
  }
  const unpublish = async () => { setConfirm(null); if (await act('unpublish')) toast('Back to a draft: readers no longer see it') }
  const remove = async () => {
    setConfirm(null)
    if (post) { try { await api.del(`${V1}/posts/${post.id}`); announceChange() } catch (e) { toast((e as Error).message, 'err'); return } }
    navigate(d?.kind === 'page' ? '/pages' : '/posts')
  }
  const duplicate = async () => { const p = await act('duplicate'); if (p) navigate(`/posts/${p.id}`) }
  const newKey = async () => { const p = await act('preview-key'); if (p) toast('The old share link no longer works') }

  if (err) return <div className="callout warn"><Icon name="alert" size={15} /><span>{err}</span></div>
  if (!d || !blog) return <Loading />
  const words = wordCount(d.body)
  const published = post?.status === 'published'
  const scheduled = post?.status === 'scheduled'
  const status = state === 'saving' ? 'Saving…' : state === 'dirty' ? (settings?.autosave === false ? 'Not saved' : 'Saving soon') : state === 'error' ? 'Not saved' : post ? 'Saved' : ''
  const address = `${blog.url}/${d.slug || 'your-title'}`
  const menu = (): MenuItem[] => [
    { label: 'Preview', icon: 'eye', onSelect: preview },
    ...(post ? [{ label: 'Make a copy', icon: 'copy', onSelect: duplicate }] : []),
    ...(published || scheduled ? [{ label: published ? 'Unpublish (back to draft)' : 'Cancel the schedule', icon: 'eye-off', onSelect: () => setConfirm('unpublish') }] : []),
    'sep',
    { label: post ? 'Delete' : 'Throw away', icon: 'trash', danger: true, onSelect: () => post ? setConfirm('delete') : remove() },
  ]
  const source = post?.source && APP_INFO[post.source as AppSlug]

  return (
    <div className={`writer ${panel ? 'with-panel' : ''}`}>
      <div className="writer-bar">
        <span className={`save-state ${state}`} aria-live="polite">{status}</span>
        <span className="pill">{post ? statusLabel(post) : 'New ' + d.kind}</span>
        <span className="spacer" />
        <button type="button" className="btn" onClick={() => setHelp({ selection: editor.current?.selection() || '' })} title="Pocket AI helps you write"><Icon name="sparkles" size={15} /><span className="hide-sm">Help me write</span></button>
        {!mobile && <button type="button" className="btn" onClick={preview} title="See it as readers will (opens a new tab)"><Icon name="eye" size={15} />Preview</button>}
        <button type="button" className={`btn icon-only ${panel ? 'active' : ''}`} onClick={() => setPanel(p => !p)} aria-label="Post details" title="Details: address, tags, summary…"><Icon name="sliders" size={16} /></button>
        <button type="button" className="btn primary" onClick={() => published ? save().then(p => p && toast('Updated — readers see the new version')) : setPublishing(true)}>
          {published ? 'Update' : scheduled ? 'Scheduled…' : 'Publish'}
        </button>
        <MenuButton label="More" align="right" items={menu}><Icon name="more" size={16} /></MenuButton>
      </div>

      <div className="writer-body">
        <div className="writer-main">
          {d.cover ? <figure className="cover-pick">
            <img src={'/media/' + d.cover} alt={d.cover_alt} />
            <div className="cover-actions"><button type="button" className="btn sm" onClick={() => setPicker(true)}><Icon name="image" size={14} />Change</button><button type="button" className="btn sm" onClick={() => change({ cover: '' })}><Icon name="x" size={14} />Remove</button></div>
          </figure> : <button type="button" className="cover-add" onClick={() => setPicker(true)}><Icon name="image" size={16} />Add a picture on top</button>}
          <textarea className="title-input" rows={1} value={d.title} placeholder={d.kind === 'page' ? 'Page title (About, Contact…)' : 'Title'} aria-label="Title"
            onChange={e => change({ title: e.target.value.replace(/\n/g, ' ') })} onKeyDown={e => { if (e.key === 'Enter') { e.preventDefault(); editor.current?.focus() } }}
            ref={el => { if (el) { el.style.height = 'auto'; el.style.height = el.scrollHeight + 'px' } }} />
          {docKey && <Editor ref={editor} docKey={docKey} value={d.body} onChange={md => change({ body: md })} onUpload={upload}
            placeholder={d.kind === 'page' ? 'What should visitors know? (type / for headings, lists and pictures)' : 'Tell your story… (type / for headings, lists, quotes and pictures; select text to make it bold or a link)'} />}
          <p className="writer-foot muted small">{words.toLocaleString()} words · {readingMinutes(words)} min read{post?.updated_at ? ` · saved ${fmtDateTime(post.updated_at)}` : ''}</p>
        </div>

        {panel && <aside className="writer-panel" aria-label="Details">
          <div className="wp-head"><h3>Details</h3><button type="button" className="btn ghost icon-only sm" onClick={() => setPanel(false)} aria-label="Close"><Icon name="x" size={16} /></button></div>
          <Field label="Address" help={published ? 'Changing it breaks links people already shared.' : 'Made from the title; change it if you like.'}>
            <div className="slug-row"><span className="muted small ellipsis">{blog.url.replace(/^https?:\/\//, '')}/</span><input value={d.slug} onChange={e => { setSlugTouched(true); change({ slug: e.target.value.toLowerCase().replace(/[^a-z0-9-]+/g, '-') }) }} onBlur={() => change({ slug: slugify(d.slug) || slugify(d.title) })} /></div>
          </Field>
          {d.kind === 'post' && <Field label="Tags" help="Words that group posts, separated by commas: travel, recipes"><input value={d.tags} onChange={e => change({ tags: e.target.value })} placeholder="travel, bosnia" /></Field>}
          <Field label="Summary" help="Shown in lists and when the post is shared. Empty: the first lines of the post."><textarea rows={3} value={d.excerpt} onChange={e => change({ excerpt: e.target.value })} /></Field>
          {d.cover && <Field label="What the picture shows" help="Read aloud to blind readers and shown when it cannot load."><input value={d.cover_alt} onChange={e => change({ cover_alt: e.target.value })} /></Field>}
          {d.kind === 'post' ? <>
            <label className="check-row"><Switch checked={d.featured} onChange={v => change({ featured: v })} label="Pin to the top" /><span className="col"><b>Pin to the top</b><span className="muted small">Stays first on your home page.</span></span></label>
            <label className="check-row"><Switch checked={d.comments} onChange={v => change({ comments: v })} label="Comments" disabled={blog.comments === 'off'} /><span className="col"><b>Comments</b><span className="muted small">{blog.comments === 'off' ? 'Off for the whole blog (Look & details).' : 'Readers may write under this post.'}</span></span></label>
          </> : <>
            <label className="check-row"><Switch checked={d.in_menu} onChange={v => change({ in_menu: v })} label="In the menu" /><span className="col"><b>In the menu</b><span className="muted small">A link to this page at the top of every page.</span></span></label>
            {d.in_menu && <Field label="Place in the menu" help="Lower numbers come first."><input type="number" className="num-sm" value={d.menu_order} onChange={e => change({ menu_order: Number(e.target.value) || 0 })} /></Field>}
          </>}
          <details className="wp-more">
            <summary>For search engines and sharing</summary>
            <Field label="Description for search engines" help="About 150 characters. Empty: the summary."><textarea rows={2} value={d.seo_description} maxLength={300} onChange={e => change({ seo_description: e.target.value })} /></Field>
            {published && post && <Field label="Date shown"><input type="datetime-local" defaultValue={localInput(new Date(post.published_at))} onBlur={e => e.target.value && api.put<Post>(`${V1}/posts/${post.id}`, { published_at: e.target.value }).then(p => { setPost(p); toast('Date changed') }).catch(er => toast((er as Error).message, 'err'))} /></Field>}
          </details>
          {post && !published && <div className="wp-share">
            <b>Show it to someone first</b>
            <p className="muted small">Anyone with this link can read the draft (nobody else can).</p>
            <CopyField value={post.url || ''} />
            <button type="button" className="link-btn small" onClick={newKey}>Stop the old link working</button>
          </div>}
          {published && post?.url && <div className="wp-share">
            <b>On your blog</b>
            <div className="row wrap"><a className="btn sm" href={post.url} target="_blank" rel="noreferrer"><Icon name="external" size={14} />Open</a><button type="button" className="btn sm" onClick={() => copyText(post.url!).then(() => toast('Link copied'))}><Icon name="link" size={14} />Copy link</button></div>
            <p className="muted small">{post.views.toLocaleString()} readers · {post.comments_count} comments{post.pending_count ? ` (${post.pending_count} waiting)` : ''}</p>
          </div>}
          {source && <p className="muted small"><Icon name={source.icon} size={13} /> Started from {source.name}.</p>}
          <p className="muted small">Address of this post: <span className="break">{address}</span></p>
        </aside>}
      </div>

      {help && <WriteHelp blogID={blog.id} title={d.title} body={d.body} selection={help.selection} language={blog.language} onClose={() => setHelp(null)} apply={{
        title: t => change({ title: t }),
        body: md => editor.current?.replace(md),
        insert: md => editor.current?.insert(md),
        summary: r => change({ ...(r.excerpt ? { excerpt: r.excerpt } : {}), ...(r.seo_description ? { seo_description: r.seo_description } : {}), ...(r.tags?.length ? { tags: Array.from(new Set([...d.tags.split(',').map(x => x.trim()).filter(Boolean), ...r.tags])).join(', ') } : {}) }),
      }} />}
      {picker && <PicturePicker title="A picture for the top of the post" onClose={() => setPicker(false)} onPick={(m: Media) => { change({ cover: m.file, cover_alt: d.cover_alt || m.alt }); setPicker(false) }} />}
      {publishing && <PublishSheet post={post} title={d.title} address={address} onClose={() => setPublishing(false)} onPublish={async at => {
        const p = await act('publish', { at })
        setPublishing(false)
        if (p) toast(p.status === 'scheduled' ? `It goes out ${fmtDateTime(p.publish_at)}` : 'Published! Your readers can see it now.')
      }} />}
      {confirm === 'delete' && <Confirm title={`Delete this ${d.kind}?`} message={published ? 'It disappears from your blog, with its comments. This cannot be undone.' : 'The draft is gone for good.'} confirmLabel="Delete" onCancel={() => setConfirm(null)} onConfirm={remove} />}
      {confirm === 'unpublish' && <Confirm title={published ? 'Take it off your blog?' : 'Cancel the schedule?'} message="It becomes a draft again: readers no longer see it, and you can publish it later." confirmLabel={published ? 'Unpublish' : 'Cancel schedule'} danger={false} onCancel={() => setConfirm(null)} onConfirm={unpublish} />}
    </div>
  )
}

function PublishSheet({ post, title, address, onClose, onPublish }: { post: Post | null; title: string; address: string; onClose: () => void; onPublish: (at: string) => Promise<void> }) {
  const [when, setWhen] = useState<'now' | 'later'>(post?.status === 'scheduled' ? 'later' : 'now')
  const [at, setAt] = useState(() => post?.publish_at ? localInput(new Date(post.publish_at)) : localInput(new Date(Date.now() + 24 * 3600e3)))
  const [busy, setBusy] = useState(false)
  const go = async () => { setBusy(true); await onPublish(when === 'now' ? 'now' : at); setBusy(false) }
  return (
    <Modal title="Publish" onClose={onClose} size="sm" footer={<>
      <button type="button" className="btn" onClick={onClose}>Not yet</button>
      <button type="button" className="btn primary" onClick={go} disabled={busy || !title.trim()}>{busy ? 'Publishing…' : when === 'now' ? 'Publish now' : 'Schedule'}</button>
    </>}>
      <div className="stack">
        {!title.trim() && <div className="callout warn"><Icon name="alert" size={15} /><span>Give it a title first.</span></div>}
        <label className="radio-row"><input type="radio" checked={when === 'now'} onChange={() => setWhen('now')} /><span className="col"><b>Now</b><span className="muted small">Readers see it right away.</span></span></label>
        <label className="radio-row"><input type="radio" checked={when === 'later'} onChange={() => setWhen('later')} /><span className="col"><b>Later</b><span className="muted small">It goes out by itself at the time you pick.</span></span></label>
        {when === 'later' && <input type="datetime-local" value={at} onChange={e => setAt(e.target.value)} aria-label="When" />}
        <p className="muted small">It will be at <span className="break">{address}</span></p>
        <p className="muted small"><Link to="/look" onClick={onClose}>Who can read your blog</Link> is set in Look & details.</p>
      </div>
    </Modal>
  )
}
