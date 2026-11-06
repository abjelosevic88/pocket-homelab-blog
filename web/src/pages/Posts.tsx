// All posts (or pages) of the blog: drafts, scheduled and published, with search.
import { useCallback, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useData, useOnChange } from '../lib/data'
import type { Post, TagCount } from '../lib/types'
import { openCreate } from '../components/Layout'
import { Icon } from '../components/icons'
import { Empty, Loading, Segmented, useAsync, useDebounce } from '../components/ui'
import { Select } from '../components/Select'
import { PostRow } from './Home'

export default function Posts({ kind }: { kind: 'post' | 'page' }) {
  const { blog } = useData()
  const [sp, setSp] = useSearchParams()
  const status = sp.get('status') || ''
  const tag = sp.get('tag') || ''
  const [q, setQ] = useState('')
  const dq = useDebounce(q, 250)
  const id = blog?.id || 0
  const { data, reload } = useAsync(() => api.get<{ items: Post[]; total: number }>(`${V1}/posts?blog=${id}&kind=${kind}&status=${status}&tag=${encodeURIComponent(tag)}&q=${encodeURIComponent(dq)}&limit=500${kind === 'page' ? '&sort=menu' : ''}`), [id, kind, status, tag, dq])
  const tags = useAsync(() => kind === 'post' ? api.get<TagCount[]>(`${V1}/tags?blog=${id}`) : Promise.resolve([] as TagCount[]), [id, kind])
  const again = useCallback(() => { reload(); tags.reload() }, [reload, tags.reload]) // eslint-disable-line react-hooks/exhaustive-deps
  useOnChange(again)
  const set = (k: string, v: string) => { const n = new URLSearchParams(sp); if (v) n.set(k, v); else n.delete(k); setSp(n, { replace: true }) }
  const items = data?.items
  return (
    <div className="stack loose page">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">{kind === 'page' ? 'Pages' : 'Posts'}</h1><p className="muted">{kind === 'page' ? 'Pages stay put and are not in the list of posts: About, Contact, Now… Put them in the menu at the top of your blog.' : 'Everything you write. Drafts are only yours until you publish them.'}</p></div>
        <button type="button" className="btn primary" onClick={() => openCreate(kind)}><Icon name="plus" size={15} />{kind === 'page' ? 'New page' : 'New post'}</button>
      </div>
      <div className="row wrap filters">
        <input className="search-in" value={q} onChange={e => setQ(e.target.value)} placeholder={`Search ${kind === 'page' ? 'pages' : 'posts'}…`} aria-label="Search" />
        <Segmented size="sm" value={status} onChange={v => set('status', v)} options={[{ value: '', label: 'All' }, { value: 'draft', label: 'Drafts' }, { value: 'scheduled', label: 'Scheduled' }, { value: 'published', label: 'Published' }]} />
        {kind === 'post' && (tags.data?.length || 0) > 0 && <Select ariaLabel="Tag" value={tag} onChange={v => set('tag', v)} placeholder="Any tag" clearLabel="Any tag" options={(tags.data || []).map(t => ({ value: t.tag, label: '#' + t.tag, hint: String(t.count) }))} />}
      </div>
      {!items ? <Loading /> : items.length === 0 ? <Empty icon={kind === 'page' ? 'file' : 'pen'} title={q || status || tag ? 'Nothing here' : kind === 'page' ? 'No pages yet' : 'No posts yet'}>
        {!(q || status || tag) && <button type="button" className="btn primary mt" onClick={() => openCreate(kind)}>{kind === 'page' ? 'Write an About page' : 'Write your first post'}</button>}
      </Empty> : <section className="card"><div className="list">{items.map(p => <PostRow key={p.id} p={p} />)}</div></section>}
      {data && data.total > (items?.length || 0) && <p className="muted small">Showing {items?.length} of {data.total}.</p>}
    </div>
  )
}
