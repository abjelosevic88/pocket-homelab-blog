// Making a blog, in three small steps: what it is about (Pocket AI can
// suggest a name), how it looks, and its address. Everything can be changed
// later in Look & details.
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { announceChange, useData } from '../lib/data'
import { ACCENTS, LANGUAGES, THEMES } from '../lib/blog'
import type { AIResult, Blog, Theme } from '../lib/types'
import { Icon } from '../components/icons'
import { Select } from '../components/Select'
import { useAsync, useDebounce, useToast } from '../components/ui'
import { ThemeThumb } from './Look'

export default function Start() {
  const { user, settings } = useApp()
  const root = (settings?.public_url || settings?.base_url || location.origin).replace(/^https?:\/\//, '')
  const { blogs, setBlogID } = useData()
  const navigate = useNavigate()
  const { toast } = useToast()
  const { data: ai } = useAsync(() => api.get<{ ready: boolean }>(`${V1}/ai/status`), [])
  const [step, setStep] = useState(1)
  const [about, setAbout] = useState('')
  const [title, setTitle] = useState('')
  const [tagline, setTagline] = useState('')
  const [aboutText, setAboutText] = useState('')
  const [language, setLanguage] = useState(() => (navigator.language || 'en').slice(0, 2))
  const [theme, setTheme] = useState<Theme>('classic')
  const [accent, setAccent] = useState(ACCENTS[0])
  const [font, setFont] = useState<'serif' | 'sans'>('serif')
  const [handle, setHandle] = useState('')
  const [handleTouched, setHandleTouched] = useState(false)
  const [busy, setBusy] = useState(false)
  const [suggesting, setSuggesting] = useState(false)
  const dh = useDebounce(handle, 300)
  const [check, setCheck] = useState<{ ok: boolean; reason: string; suggestion?: string } | null>(null)

  useEffect(() => { if (!handleTouched) setHandle(title.toLowerCase().normalize('NFD').replace(/[̀-ͯ]/g, '').replace(/đ/g, 'd').replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 30)) }, [title, handleTouched])
  useEffect(() => {
    if (!dh) { setCheck(null); return }
    api.get<{ ok: boolean; reason: string; suggestion?: string }>(`${V1}/blogs/check-handle?handle=${encodeURIComponent(dh)}&name=${encodeURIComponent(title)}`).then(setCheck).catch(() => setCheck(null))
  }, [dh]) // eslint-disable-line react-hooks/exhaustive-deps

  const suggest = async () => {
    setSuggesting(true)
    try {
      const r = await api.post<AIResult>(`${V1}/ai/write`, { action: 'setup', text: about, language })
      if (r.title) setTitle(r.title)
      if (r.tagline) setTagline(r.tagline)
      if (r.about) setAboutText(r.about)
      if (r.handle) { setHandle(r.handle); setHandleTouched(true) }
    } catch (e) { toast((e as Error).message, 'err') } finally { setSuggesting(false) }
  }
  const create = async () => {
    setBusy(true)
    try {
      const b = await api.post<Blog>(`${V1}/blogs`, { title, tagline, handle, about: aboutText, about_page: !!aboutText.trim(), theme, accent, font, language, author_name: user?.name })
      setBlogID(b.id)
      announceChange()
      toast('Your blog is ready!')
      navigate('/')
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const first = !blogs || blogs.length === 0
  return (
    <div className="start page">
      <div className="start-head">
        <img src="/favicon.svg" alt="" width={44} height={44} />
        <div><h1>{first ? 'Let’s make your blog' : 'Start another blog'}</h1><p className="muted">Three small steps. You can change everything later.</p></div>
      </div>
      <ol className="start-steps" aria-label="Steps">
        {['What it’s about', 'How it looks', 'Its address'].map((s, i) => <li key={s} className={step === i + 1 ? 'on' : step > i + 1 ? 'done' : ''}><span>{step > i + 1 ? <Icon name="check" size={13} /> : i + 1}</span>{s}</li>)}
      </ol>

      {step === 1 && <section className="card pad stack loose">
        <label className="field"><span className="label">In one sentence, what will you write about?</span>
          <textarea rows={2} value={about} onChange={e => setAbout(e.target.value)} placeholder="Weekend trips around Bosnia, the food we find, and the books I read on the way" /></label>
        {ai?.ready && <div className="row"><button type="button" className="btn" onClick={suggest} disabled={!about.trim() || suggesting}>{suggesting ? <><span className="spinner" />Thinking…</> : <><Icon name="sparkles" size={15} />Suggest a name and a short text</>}</button></div>}
        <label className="field"><span className="label">Your blog’s name</span><input value={title} onChange={e => setTitle(e.target.value)} placeholder="Slow Roads" autoFocus /></label>
        <label className="field"><span className="label">A line under the name (optional)</span><input value={tagline} onChange={e => setTagline(e.target.value)} placeholder="Trips, kitchens and good books" /></label>
        <label className="field"><span className="label">A few words about you (optional)</span><textarea rows={3} value={aboutText} onChange={e => setAboutText(e.target.value)} placeholder="Hi, I’m Ana. I write about…" /><span className="help">Shown on your home page; we also make an About page from it.</span></label>
        <label className="field" onClick={e => e.preventDefault()}><span className="label">Language of the blog</span><Select ariaLabel="Language" value={language} onChange={setLanguage} options={LANGUAGES} /><span className="help">The words around your posts (“Read more”, dates) and the AI’s help are in this language.</span></label>
        <div className="row end"><button type="button" className="btn primary" disabled={!title.trim()} onClick={() => setStep(2)}>Next<Icon name="chevron-right" size={15} /></button></div>
      </section>}

      {step === 2 && <section className="card pad stack loose">
        <div className="theme-grid">
          {THEMES.map(t => (
            <button key={t.id} type="button" className={`theme-card ${theme === t.id ? 'on' : ''}`} onClick={() => setTheme(t.id)} aria-pressed={theme === t.id}>
              <ThemeThumb theme={t.id} accent={accent} font={font} />
              <b>{t.name}</b><span className="muted small">{t.help}</span>
            </button>
          ))}
        </div>
        <div className="field"><span className="label">Colour</span><div className="swatches">{ACCENTS.map(c => <button key={c} type="button" className={`swatch ${accent === c ? 'on' : ''}`} style={{ background: c }} onClick={() => setAccent(c)} aria-label={c} />)}</div></div>
        <div className="field"><span className="label">Letters</span>
          <div className="row wrap"><button type="button" className={`font-pick serif ${font === 'serif' ? 'on' : ''}`} onClick={() => setFont('serif')}>Classic<span>like a book</span></button><button type="button" className={`font-pick sans ${font === 'sans' ? 'on' : ''}`} onClick={() => setFont('sans')}>Modern<span>like a website</span></button></div></div>
        <div className="row end"><button type="button" className="btn" onClick={() => setStep(1)}>Back</button><button type="button" className="btn primary" onClick={() => setStep(3)}>Next<Icon name="chevron-right" size={15} /></button></div>
      </section>}

      {step === 3 && <section className="card pad stack loose">
        <label className="field"><span className="label">Your blog’s address</span>
          <div className="handle-row"><span className="muted">{root}/b/</span><input value={handle} onChange={e => { setHandleTouched(true); setHandle(e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, '')) }} /></div>
          {check && (check.ok ? <span className="help ok"><Icon name="check" size={13} /> Free — it’s yours.</span> : <span className="help bad">{check.reason}{check.suggestion && <> · <button type="button" className="link-btn" onClick={() => setHandle(check.suggestion!)}>use {check.suggestion}</button></>}</span>)}
          <span className="help">Small letters, numbers and dashes. You can also give the blog its own web address (like myblog.com) later.</span>
        </label>
        <div className="row end"><button type="button" className="btn" onClick={() => setStep(2)}>Back</button><button type="button" className="btn primary" onClick={create} disabled={busy || !title.trim() || !check?.ok}>{busy ? 'Making it…' : <><Icon name="check" size={15} />Make my blog</>}</button></div>
      </section>}
    </div>
  )
}
