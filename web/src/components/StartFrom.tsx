// "Start from your apps": pick an app, pick something in it (a trip, a
// book, a recipe…), and Blog writes a first draft from it — with Pocket AI
// in your voice, or as a tidy template — for you to finish.
import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { APP_INFO } from '../lib/blog'
import { announceChange, useData } from '../lib/data'
import { fmtDate } from '../lib/format'
import type { AppSlug, Post, SourceApp, SourceItem } from '../lib/types'
import { Icon } from './icons'
import { Loading, Modal, Switch, useAsync, useDebounce, useToast } from './ui'

export function StartFrom({ app: startApp, item: startItem, onClose, onDone }: { app?: string; item?: { id: string; title: string }; onClose: () => void; onDone: (postID: number) => void }) {
  const { blog } = useData()
  const { toast } = useToast()
  const { data: apps } = useAsync(() => api.get<SourceApp[]>(`${V1}/sources`), [])
  const { data: ai } = useAsync(() => api.get<{ ready: boolean }>(`${V1}/ai/status`), [])
  const [app, setApp] = useState<AppSlug | null>((startApp as AppSlug) || null)
  const [item, setItem] = useState<SourceItem | null>(startItem ? { id: startItem.id, title: startItem.title } : null)
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 300)
  const [items, setItems] = useState<SourceItem[] | null>(null)
  const [err, setErr] = useState('')
  const [useAI, setUseAI] = useState(true)
  const [note, setNote] = useState('')
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!app || item) return
    let alive = true
    setErr('')
    api.get<SourceItem[]>(`${V1}/sources/${app}?q=${encodeURIComponent(dq)}`)
      .then(r => { if (alive) setItems(r) })
      .catch(e => { if (alive) { setItems([]); setErr((e as Error).message) } })
    return () => { alive = false }
  }, [app, dq, item])

  const make = async () => {
    if (!app || !item) return
    setBusy(true)
    try {
      const r = await api.post<{ post: Post; existing?: boolean; ai?: string; ai_error?: string }>(`${V1}/sources/${app}/draft`, { id: item.id, blog_id: blog?.id, ai: useAI && !!ai?.ready, note })
      announceChange()
      if (r.existing) toast('You already started a post from this one — here it is')
      else if (useAI && ai?.ready && !r.ai) toast(`Made a draft without AI: ${r.ai_error || 'the AI did not answer'}`, 'err')
      onDone(r.post.id)
    } catch (e) { toast((e as Error).message, 'err'); setBusy(false) }
  }

  const info = app ? APP_INFO[app] : null
  const title = !app ? 'Start a post from your apps' : !item ? info!.verb : 'Write the first version'
  return (
    <Modal title={title} onClose={onClose} size="lg" footer={app && item ? <>
      <button type="button" className="btn" onClick={() => startItem ? onClose() : setItem(null)} disabled={busy}>Back</button>
      <button type="button" className="btn primary" onClick={make} disabled={busy}>{busy ? <><span className="spinner" />{useAI && ai?.ready ? 'Writing… (up to a minute)' : 'Making the draft…'}</> : <><Icon name="pen" size={15} />Make the draft</>}</button>
    </> : undefined}>
      {!app && (apps ? <div className="stack">
        <p className="muted">Pick where the story is. Blog reads it with your permission and turns it into a draft — nothing is published until you say so.</p>
        <div className="app-grid">
          {apps.map(a => (
            <button key={a.slug} type="button" className={`app-card ${a.connected ? '' : 'off'}`} onClick={() => a.connected && setApp(a.slug)} disabled={!a.connected} title={a.connected ? a.reason : a.detail}>
              <span className="app-card-ico"><Icon name={APP_INFO[a.slug].icon} size={20} /></span>
              <span className="app-card-text"><b>{APP_INFO[a.slug].noun}</b><span className="muted small">{a.connected ? a.reason : `${a.name}: ${a.detail || 'not connected'}`}</span></span>
            </button>
          ))}
        </div>
        {apps.some(a => !a.connected) && <p className="muted small">Greyed-out apps are not connected yet: <Link to="/settings/apps" onClick={onClose}>Settings → Connected apps</Link>.</p>}
      </div> : <Loading />)}

      {app && !item && <div className="stack">
        <div className="row"><button type="button" className="btn sm ghost" onClick={() => { setApp(null); setItems(null); setQ('') }}><Icon name="chevron-left" size={15} />All apps</button></div>
        <input autoFocus value={q} onChange={e => setQ(e.target.value)} placeholder={`Search ${info!.name}…`} aria-label="Search" />
        {err && <div className="callout warn"><Icon name="alert" size={15} /><span>{err}</span></div>}
        {!items ? <Loading /> : items.length === 0 ? <p className="muted">{q ? 'Nothing found.' : `Nothing in ${info!.name} yet.`}</p> :
          <div className="pick-list tall">
            {items.map(it => (
              <button key={it.id} type="button" className="pick-row" onClick={() => setItem(it)}>
                <span className="list-ico"><Icon name={info!.icon} size={16} /></span>
                <span className="grow col"><b className="ellipsis">{it.title}</b>{it.subtitle && <span className="muted small ellipsis">{it.subtitle}</span>}</span>
                {!!it.rating && <span className="muted small">★ {it.rating.toFixed(it.rating % 1 ? 1 : 0)}</span>}
                {it.date && <span className="muted small tnum">{fmtDate(it.date)}</span>}
              </button>
            ))}
          </div>}
      </div>}

      {app && item && <div className="stack loose">
        <div className="picked"><span className="list-ico"><Icon name={info!.icon} size={18} /></span><div className="col"><span className="muted small">{info!.noun} from {info!.name}</span><b>{item.title}</b></div></div>
        <label className="check-row">
          <Switch checked={useAI && !!ai?.ready} onChange={setUseAI} disabled={!ai?.ready} label="Let AI write the first version" />
          <span className="col"><b>Let AI write the first version</b><span className="muted small">{ai?.ready ? 'A readable post in your blog’s language, from what is in the app. You change anything you like before publishing.' : 'No AI is connected (Settings → AI), so you get a tidy template with everything from the app to write around.'}</span></span>
        </label>
        <label className="field"><span className="label">Anything to add? (optional)</span>
          <textarea rows={3} value={note} onChange={e => setNote(e.target.value)} placeholder="What made it special, who you were with, what you would tell a friend…" />
        </label>
      </div>}
    </Modal>
  )
}
