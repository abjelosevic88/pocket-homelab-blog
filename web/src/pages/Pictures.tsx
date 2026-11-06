// Every picture you uploaded: add more, say what each shows, copy its
// address, delete the ones you no longer use.
import { useState } from 'react'
import { api, V1 } from '../lib/api'
import { fmtBytes, fmtDate } from '../lib/format'
import type { Media } from '../lib/types'
import { Icon } from '../components/icons'
import { uploadPictures } from '../components/PicturePicker'
import { Confirm, CopyField, Empty, Field, Loading, Modal, useAsync, useToast } from '../components/ui'

export default function Pictures() {
  const { data, setData } = useAsync(() => api.get<Media[]>(`${V1}/media`), [])
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const [open, setOpen] = useState<Media | null>(null)
  const add = async (files: FileList | null) => {
    if (!files?.length) return
    setBusy(true)
    try { const ms = await uploadPictures(files); setData(d => [...ms, ...(d || [])]); toast(ms.length === 1 ? 'Picture added' : `${ms.length} pictures added`) } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <div className="stack loose page" onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); add(e.dataTransfer.files) }}>
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Pictures</h1><p className="muted">Photos from your phone are made smaller for the web and lose the hidden camera data (like where they were taken).</p></div>
        <label className={`btn primary ${busy ? 'disabled' : ''}`}><Icon name="upload" size={15} />{busy ? 'Uploading…' : 'Add pictures'}<input type="file" accept="image/*" multiple hidden onChange={e => { add(e.target.files); e.target.value = '' }} /></label>
      </div>
      {!data ? <Loading /> : data.length === 0 ? <Empty icon="image" title="No pictures yet">Add them here, or straight in a post: drop a picture into the text, or type / and choose Image.</Empty> :
        <div className="pic-grid big">
          {data.map(m => (
            <button key={m.id} type="button" className="pic" onClick={() => setOpen(m)} title={m.alt || m.name}>
              <img src={m.url} alt={m.alt} loading="lazy" />
              {m.used === 0 && <span className="pic-badge">not used</span>}
            </button>
          ))}
        </div>}
      {open && <PictureSheet m={open} onClose={() => setOpen(null)} onSaved={x => { setData(d => (d || []).map(y => y.id === x.id ? { ...y, alt: x.alt } : y)); setOpen(null) }} onGone={() => { setData(d => (d || []).filter(y => y.id !== open.id)); setOpen(null) }} />}
    </div>
  )
}

function PictureSheet({ m, onClose, onSaved, onGone }: { m: Media; onClose: () => void; onSaved: (m: Media) => void; onGone: () => void }) {
  const { toast } = useToast()
  const [alt, setAlt] = useState(m.alt)
  const [del, setDel] = useState(false)
  const save = async () => { try { onSaved(await api.put<Media>(`${V1}/media/${m.id}`, { alt })); toast('Saved') } catch (e) { toast((e as Error).message, 'err') } }
  const remove = async () => { try { await api.del(`${V1}/media/${m.id}`); onGone(); toast('Picture deleted') } catch (e) { toast((e as Error).message, 'err') } }
  const abs = location.origin + m.url
  return (
    <Modal title={m.name || 'Picture'} onClose={onClose} size="lg" footer={<>
      <button type="button" className="btn ghost danger-text" onClick={() => setDel(true)}><Icon name="trash" size={15} />Delete</button>
      <span className="spacer" />
      <button type="button" className="btn" onClick={onClose}>Close</button>
      <button type="button" className="btn primary" onClick={save} disabled={alt === m.alt}>Save</button>
    </>}>
      <div className="stack">
        <img className="pic-full" src={m.url} alt={m.alt} />
        <p className="muted small">{m.width} × {m.height} · {fmtBytes(m.bytes)} · added {fmtDate(m.created_at.slice(0, 10))} · {m.used ? `in ${m.used} ${m.used === 1 ? 'post' : 'posts'}` : 'not in any post'}</p>
        <Field label="What the picture shows" help="Read aloud to blind readers, and shown when the picture cannot load."><input value={alt} onChange={e => setAlt(e.target.value)} /></Field>
        <Field group label="Its address"><CopyField value={abs} /></Field>
        <Field group label="To put it in a post"><CopyField value={`![${alt}](${m.url})`} /></Field>
      </div>
      {del && <Confirm title="Delete this picture?" message={m.used ? `It is in ${m.used} ${m.used === 1 ? 'post' : 'posts'}; there it will show as missing.` : 'It is not in any post.'} confirmLabel="Delete" onCancel={() => setDel(false)} onConfirm={remove} />}
    </Modal>
  )
}
