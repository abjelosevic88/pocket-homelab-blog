// Bring posts in: a WordPress export, Markdown files (Hugo, Jekyll,
// Obsidian…), or a Pocket Blog export. Doing it twice adds nothing twice.
import { useState } from 'react'
import { uploadForm, V1 } from '../lib/api'
import { announceChange, useData } from '../lib/data'
import { Icon } from './icons'
import { Modal, useToast } from './ui'

export function ImportModal({ onClose }: { onClose: () => void }) {
  const { blog } = useData()
  const { toast } = useToast()
  const [busy, setBusy] = useState(false)
  const [done, setDone] = useState<Record<string, number> | null>(null)
  const go = async (f: File) => {
    setBusy(true)
    try {
      const r = await uploadForm<Record<string, number>>(`${V1}/import`, f, { blog: String(blog?.id || '') })
      setDone(r)
      announceChange()
    } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const words: Record<string, string> = { posts: 'posts', pages: 'pages', comments: 'comments', pictures: 'pictures', blogs: 'blogs', skipped: 'already here' }
  return (
    <Modal title="Bring your posts here" onClose={onClose} footer={<button type="button" className="btn" onClick={onClose}>{done ? 'Done' : 'Cancel'}</button>}>
      <div className="stack loose">
        <div className="import-ways">
          <div><b>From WordPress</b><p className="muted small">In WordPress: Tools → Export → All content → Download. Choose that .xml file here. Posts, pages, tags and approved comments come over; pictures stay where they are.</p></div>
          <div><b>From Markdown files</b><p className="muted small">One .md file, or a .zip of them with their pictures (Hugo, Jekyll, Obsidian, Ghost’s Markdown export…). The title, date and tags on top of each file are used.</p></div>
          <div><b>From Pocket Blog</b><p className="muted small">An export from Settings → Backup & data (the JSON, or the Markdown ZIP with pictures).</p></div>
        </div>
        {blog && <p className="muted small">Posts go into <b>{blog.title}</b>. Published posts keep their dates; drafts stay drafts.</p>}
        <label className={`btn primary ${busy ? 'disabled' : ''}`}><Icon name="upload" size={15} />{busy ? 'Bringing them in…' : 'Choose a file…'}
          <input type="file" hidden accept=".xml,.json,.md,.markdown,.zip,.txt" disabled={busy} onChange={e => { const f = e.target.files?.[0]; e.target.value = ''; if (f) go(f) }} />
        </label>
        {done && <div className="callout ok"><Icon name="check-circle" size={15} /><span>Done: {Object.entries(done).filter(([, n]) => n > 0).map(([k, n]) => `${n} ${words[k] || k}`).join(', ') || 'nothing new'}.</span></div>}
      </div>
    </Modal>
  )
}
