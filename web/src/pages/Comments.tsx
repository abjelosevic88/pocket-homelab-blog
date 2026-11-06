// What readers wrote: approve, answer, mark as spam, delete.
import { useCallback, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { announceChange, useData, useOnChange } from '../lib/data'
import { ago } from '../lib/format'
import type { Comment } from '../lib/types'
import { Icon } from '../components/icons'
import { Avatar, Empty, Loading, Segmented, useAsync, useToast } from '../components/ui'

export default function Comments() {
  const { blog } = useData()
  const [sp, setSp] = useSearchParams()
  const status = sp.get('status') || (blog?.pending_comments ? 'pending' : 'approved')
  const { data, setData, reload } = useAsync(() => api.get<Comment[]>(`${V1}/comments?blog=${blog?.id || 0}&status=${status}`), [blog?.id, status])
  useOnChange(useCallback(() => { reload() }, [reload]))
  return (
    <div className="stack loose page">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Comments</h1><p className="muted">{blog?.comments === 'off' ? 'Comments are switched off for this blog (Look & details).' : blog?.comments === 'open' ? 'Comments show at once; ones full of links wait here for you.' : 'A comment shows on your blog once you approve it.'}</p></div>
        <Segmented size="sm" value={status} onChange={v => setSp(new URLSearchParams({ status: v }), { replace: true })} options={[{ value: 'pending', label: `Waiting${blog?.pending_comments ? ` (${blog.pending_comments})` : ''}` }, { value: 'approved', label: 'On the blog' }, { value: 'spam', label: 'Spam' }]} />
      </div>
      {!data ? <Loading /> : data.length === 0 ? <Empty icon="message" title={status === 'pending' ? 'Nothing waiting' : status === 'spam' ? 'No spam' : 'No comments yet'}>{status === 'pending' ? 'New comments appear here. You can also be told about them (Settings → Notifications).' : null}</Empty> :
        <div className="stack">{data.map(c => <CommentCard key={c.id} c={c} onGone={() => setData(d => (d || []).filter(x => x.id !== c.id))} />)}</div>}
    </div>
  )
}

function CommentCard({ c, onGone }: { c: Comment; onGone: () => void }) {
  const { toast } = useToast()
  const [reply, setReply] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const setStatus = async (s: Comment['status']) => {
    setBusy(true)
    try { await api.put(`${V1}/comments/${c.id}`, { status: s }); onGone(); announceChange(); toast(s === 'approved' ? 'Approved — it shows on your blog' : s === 'spam' ? 'Moved to spam' : 'Back to waiting') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  const remove = async () => { setBusy(true); try { await api.del(`${V1}/comments/${c.id}`); onGone(); announceChange() } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) } }
  const send = async () => {
    if (!reply?.trim()) return
    setBusy(true)
    try { await api.post(`${V1}/comments/${c.id}/reply`, { body: reply }); setReply(null); if (c.status === 'pending') onGone(); announceChange(); toast('Your answer is on the blog') } catch (e) { toast((e as Error).message, 'err') } finally { setBusy(false) }
  }
  return (
    <article className={`comment-card card pad ${c.by_author ? 'mine' : ''}`}>
      <div className="cc-head">
        <Avatar name={c.name} size={30} />
        <div className="col grow"><b>{c.name}{c.by_author && <span className="pill accent">You</span>}</b><span className="muted small">on {c.post_url ? <a href={c.post_url} target="_blank" rel="noreferrer">{c.post_title || 'a post'}</a> : c.post_title} · {ago(c.created_at)}{c.email ? ` · ${c.email}` : ''}</span></div>
      </div>
      <p className="cc-body">{c.body}</p>
      {reply !== null && <div className="stack tight"><textarea autoFocus rows={3} value={reply} onChange={e => setReply(e.target.value)} placeholder={`Answer ${c.name}…`} /><div className="row"><button type="button" className="btn primary sm" onClick={send} disabled={busy || !reply.trim()}>Send answer</button><button type="button" className="btn sm" onClick={() => setReply(null)}>Cancel</button></div></div>}
      {reply === null && <div className="row wrap">
        {c.status !== 'approved' && <button type="button" className="btn sm primary" onClick={() => setStatus('approved')} disabled={busy}><Icon name="check" size={14} />Approve</button>}
        {!c.by_author && c.status !== 'spam' && <button type="button" className="btn sm" onClick={() => setReply('')} disabled={busy}><Icon name="message" size={14} />Answer</button>}
        {c.status !== 'spam' && !c.by_author && <button type="button" className="btn sm ghost" onClick={() => setStatus('spam')} disabled={busy}><Icon name="flag" size={14} />Spam</button>}
        {c.status === 'spam' && <button type="button" className="btn sm" onClick={() => setStatus('pending')} disabled={busy}>Not spam</button>}
        <button type="button" className="btn sm ghost danger-text" onClick={remove} disabled={busy}><Icon name="trash" size={14} />Delete</button>
      </div>}
    </article>
  )
}
