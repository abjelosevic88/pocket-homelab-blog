// Choose a picture: one you uploaded before, or a new one from this device.
import { useState } from 'react'
import { api, V1 } from '../lib/api'
import type { Media } from '../lib/types'
import { Icon } from './icons'
import { Loading, Modal, useAsync, useToast } from './ui'

export async function uploadPictures(files: FileList | File[]): Promise<Media[]> {
  const fd = new FormData()
  for (const f of Array.from(files)) fd.append('file', f)
  const res = await fetch(`${V1}/media`, { method: 'POST', body: fd, credentials: 'same-origin' })
  const j = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(j.error || res.statusText)
  return j as Media[]
}

export function PicturePicker({ title = 'Choose a picture', onPick, onClose }: { title?: string; onPick: (m: Media) => void; onClose: () => void }) {
  const { data, setData } = useAsync(() => api.get<Media[]>(`${V1}/media`), [])
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const add = async (files: FileList | null) => {
    if (!files?.length) return
    setBusy(true)
    try {
      const ms = await uploadPictures(files)
      setData(d => [...ms, ...(d || [])])
      if (ms.length === 1) onPick(ms[0])
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <Modal title={title} onClose={onClose} size="lg">
      <div className="stack">
        <label className={`drop-zone ${busy ? 'busy' : ''}`} onDragOver={e => e.preventDefault()} onDrop={e => { e.preventDefault(); add(e.dataTransfer.files) }}>
          <Icon name="upload" size={22} />
          <span><b>{busy ? 'Uploading…' : 'Upload from this device'}</b><span className="muted small block-line">or drop pictures here (JPEG, PNG, GIF, WebP)</span></span>
          <input type="file" accept="image/*" multiple hidden onChange={e => { add(e.target.files); e.target.value = '' }} />
        </label>
        {!data ? <Loading /> : data.length === 0 ? <p className="muted">No pictures yet.</p> :
          <div className="pic-grid">
            {data.map(m => (
              <button key={m.id} type="button" className="pic" onClick={() => onPick(m)} title={m.alt || m.name}>
                <img src={m.url} alt={m.alt} loading="lazy" />
              </button>
            ))}
          </div>}
      </div>
    </Modal>
  )
}
