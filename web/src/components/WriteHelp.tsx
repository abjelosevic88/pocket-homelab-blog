// "Help me write": Pocket AI as a patient editor. Every answer is shown
// first; nothing changes in the post until "Use this".
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { LANGUAGES } from '../lib/blog'
import type { AIResult } from '../lib/types'
import { Icon } from './icons'
import { Select } from './Select'
import { Modal, useAsync } from './ui'

type Action = 'draft' | 'outline' | 'improve' | 'fix' | 'shorter' | 'longer' | 'tone' | 'continue' | 'rewrite' | 'titles' | 'summary' | 'translate'

const ACTIONS: { id: Action; icon: string; label: string; help: string; needs: 'idea' | 'body' | 'selection' | 'none' }[] = [
  { id: 'draft', icon: 'sparkles', label: 'Write a first version', help: 'Tell it in a few words what the post is about (and paste notes if you have them).', needs: 'idea' },
  { id: 'outline', icon: 'list', label: 'Make a plan', help: 'Headings with what to say under each — you fill them in.', needs: 'idea' },
  { id: 'improve', icon: 'feather', label: 'Make it read better', help: 'Clearer sentences and better flow; your meaning and voice stay.', needs: 'body' },
  { id: 'fix', icon: 'check', label: 'Fix spelling and grammar', help: 'Only mistakes are corrected, nothing else.', needs: 'body' },
  { id: 'shorter', icon: 'minus', label: 'Make it shorter', help: 'About half as long, keeping what matters.', needs: 'body' },
  { id: 'longer', icon: 'plus', label: 'Make it richer', help: 'More detail and feeling, from what is already there.', needs: 'body' },
  { id: 'tone', icon: 'smile', label: 'Change how it sounds', help: 'Friendlier, more formal, funnier, simpler…', needs: 'body' },
  { id: 'continue', icon: 'pen', label: 'Keep writing', help: 'A few more paragraphs where you stopped.', needs: 'body' },
  { id: 'rewrite', icon: 'edit', label: 'Rewrite the selected text', help: 'Select a part in the editor first.', needs: 'selection' },
  { id: 'titles', icon: 'text', label: 'Suggest titles', help: 'Five honest, inviting titles to pick from.', needs: 'body' },
  { id: 'summary', icon: 'tag', label: 'Write the summary and tags', help: 'The short text shown in lists and when shared, plus tags.', needs: 'body' },
  { id: 'translate', icon: 'globe', label: 'Translate', help: 'The whole post into another language.', needs: 'body' },
]

export interface WriteHelpApply {
  title: (t: string) => void
  body: (md: string) => void
  insert: (md: string) => void
  summary: (r: { excerpt?: string; seo_description?: string; tags?: string[] }) => void
}

export function WriteHelp({ blogID, title, body, selection, language, apply, onClose }: {
  blogID?: number; title: string; body: string; selection: string; language: string; apply: WriteHelpApply; onClose: () => void
}) {
  const { data: ai } = useAsync(() => api.get<{ ready: boolean; provider: string; detail: string }>(`${V1}/ai/status`), [])
  const [action, setAction] = useState<Action | null>(selection ? 'rewrite' : body.trim() ? null : 'draft')
  const [idea, setIdea] = useState(title)
  const [instruction, setInstruction] = useState('')
  const [lang, setLang] = useState(language === 'en' ? 'bs' : 'en')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [res, setRes] = useState<AIResult | null>(null)
  const hasBody = body.trim().length > 0
  const cur = ACTIONS.find(a => a.id === action)

  const run = async () => {
    if (!action) return
    setBusy(true); setErr(''); setRes(null)
    try {
      const r = await api.post<AIResult>(`${V1}/ai/write`, { action, blog_id: blogID, title, body, text: action === 'rewrite' ? selection : idea, instruction, language: lang })
      setRes(r)
    } catch (e) { setErr((e as Error).message) } finally { setBusy(false) }
  }
  const use = () => {
    if (!res) return
    switch (action) {
      case 'draft': case 'outline': case 'translate':
        if (res.title) apply.title(res.title)
        if (res.body) apply.body(res.body)
        if (res.excerpt || res.tags?.length) apply.summary({ excerpt: res.excerpt, tags: res.tags })
        break
      case 'continue': apply.insert('\n\n' + (res.text || '')); break
      case 'rewrite': apply.insert(res.text || ''); break
      case 'summary': apply.summary(res); break
      default: if (res.body) apply.body(res.body)
    }
    onClose()
  }

  if (ai && !ai.ready) return (
    <Modal title="Help me write" onClose={onClose} size="sm">
      <div className="callout info"><Icon name="info" size={15} /><div>No AI is connected{ai.detail ? ` (${ai.detail})` : ''}. Turn on Pocket AI or add your own key in <Link to="/settings/ai" onClick={onClose}>Settings → AI</Link>.</div></div>
    </Modal>
  )

  return (
    <Modal title={cur ? cur.label : 'Help me write'} onClose={onClose} size="lg" footer={action ? <>
      <button type="button" className="btn" onClick={() => { setAction(null); setRes(null); setErr('') }} disabled={busy}>Back</button>
      {res ? <>
        <button type="button" className="btn" onClick={run} disabled={busy}><Icon name="refresh" size={15} />Try again</button>
        {action !== 'titles' && <button type="button" className="btn primary" onClick={use}><Icon name="check" size={15} />Use this</button>}
      </> : <button type="button" className="btn primary" onClick={run} disabled={busy || (cur?.needs === 'idea' && !idea.trim() && !hasBody) || (cur?.needs === 'body' && !hasBody) || (cur?.needs === 'selection' && !selection)}>
        {busy ? <><span className="spinner" />Writing… (up to a minute)</> : <><Icon name="sparkles" size={15} />Go</>}
      </button>}
    </> : undefined}>
      {!action && <div className="ai-grid">
        {ACTIONS.map(a => {
          const off = (a.needs === 'body' && !hasBody) || (a.needs === 'selection' && !selection)
          return (
            <button key={a.id} type="button" className="ai-card" disabled={off} onClick={() => { setAction(a.id); setRes(null) }} title={off ? (a.needs === 'selection' ? 'Select some text in the editor first' : 'Write something first') : a.help}>
              <Icon name={a.icon} size={18} />
              <span className="col"><b>{a.label}</b><span className="muted small">{a.help}</span></span>
            </button>
          )
        })}
      </div>}

      {action && !res && <div className="stack">
        <p className="muted">{cur?.help}</p>
        {(action === 'draft' || action === 'outline') && <label className="field"><span className="label">What is the post about?</span>
          <textarea autoFocus rows={3} value={idea} onChange={e => setIdea(e.target.value)} placeholder="Our weekend in Mostar: the train, the old bridge early in the morning, where we ate, what I would do differently" /></label>}
        {action === 'draft' && hasBody && <p className="muted small">What is already in the editor is used as your notes.</p>}
        {(action === 'tone' || action === 'rewrite' || action === 'draft') && <label className="field"><span className="label">{action === 'tone' ? 'How should it sound?' : 'Anything else? (optional)'}</span>
          <input value={instruction} onChange={e => setInstruction(e.target.value)} placeholder={action === 'tone' ? 'friendlier and more personal' : action === 'rewrite' ? 'simpler, for beginners' : 'with practical tips at the end'} /></label>}
        {action === 'rewrite' && <blockquote className="sel-quote">{selection}</blockquote>}
        {action === 'translate' && <label className="field" onClick={e => e.preventDefault()}><span className="label">Into</span><Select ariaLabel="Language" value={lang} onChange={setLang} options={LANGUAGES} /></label>}
        {err && <div className="callout warn"><Icon name="alert" size={15} /><span>{err}</span></div>}
      </div>}

      {res && <div className="stack">
        {action === 'titles' ? <div className="pick-list">
          {(res.titles || []).map(t => <button key={t} type="button" className="pick-row" onClick={() => { apply.title(t); onClose() }}><Icon name="text" size={15} /><span className="grow">{t}</span><span className="muted small">Use</span></button>)}
        </div> : action === 'summary' ? <dl className="set-dl">
          {res.excerpt && <div><dt>Summary</dt><dd>{res.excerpt}</dd></div>}
          {res.seo_description && <div><dt>For search engines</dt><dd>{res.seo_description}</dd></div>}
          {res.tags?.length ? <div><dt>Tags</dt><dd>{res.tags.map(t => '#' + t).join(' ')}</dd></div> : null}
        </dl> : <>
          {res.title && <h3 className="ai-title">{res.title}</h3>}
          <div className="ai-result">{res.body || res.text}</div>
          {(action === 'draft' || action === 'improve' || action === 'translate' || action === 'shorter' || action === 'longer' || action === 'tone' || action === 'fix' || action === 'outline') && hasBody && <p className="muted small">“Use this” replaces what is in the editor now.</p>}
        </>}
        <p className="muted small">Written by {res.provider === 'pocket' ? 'Pocket AI' : 'your AI key'}. Check names, places and facts — it can be wrong.</p>
      </div>}
    </Modal>
  )
}
