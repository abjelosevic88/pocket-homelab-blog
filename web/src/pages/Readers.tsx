// How many people read the blog, day by day, and which posts. Counted on
// this server without cookies: one reader per post per day, robots left out.
import { useState } from 'react'
import { Link } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useData } from '../lib/data'
import { fmtDate } from '../lib/format'
import type { Stats } from '../lib/types'
import { Icon } from '../components/icons'
import { Loading, Segmented, useAsync } from '../components/ui'

export default function Readers() {
  const { blog } = useData()
  const [days, setDays] = useState(30)
  const { data } = useAsync(() => api.get<Stats>(`${V1}/stats?blog=${blog?.id || 0}&days=${days}`), [blog?.id, days])
  const max = Math.max(1, ...(data?.by_day.map(d => d.count) || [1]))
  const change = data && data.previous > 0 ? Math.round((data.views - data.previous) / data.previous * 100) : null
  const avg = data ? Math.round(data.views / data.days) : 0
  return (
    <div className="stack loose page">
      <div className="page-head">
        <div className="grow"><h1 className="page-title">Readers</h1><p className="muted">Counted here, without cookies or trackers: each reader once a day per post, search robots left out.</p></div>
        <Segmented size="sm" value={days} onChange={setDays} options={[{ value: 7, label: '7 days' }, { value: 30, label: '30 days' }, { value: 90, label: '3 months' }, { value: 365, label: 'A year' }]} />
      </div>
      {!data ? <Loading /> : <>
        <div className="stats-row">
          <div className="stat"><span className="stat-l">Readers</span><span className="stat-v tnum">{data.views.toLocaleString()}</span>{change !== null && <span className={`small ${change >= 0 ? 'ok' : 'bad'}`}>{change >= 0 ? '+' : ''}{change}% vs the {days} days before</span>}</div>
          <div className="stat"><span className="stat-l">A day, on average</span><span className="stat-v tnum">{avg.toLocaleString()}</span></div>
          <div className="stat"><span className="stat-l">Comments</span><span className="stat-v tnum">{data.comments}</span></div>
        </div>
        <section className="card pad">
          <div className={`daybars ${data.by_day.length > 120 ? 'dense' : ''}`} role="img" aria-label={`Readers per day over ${days} days`}>
            {data.by_day.map(d => <span key={d.day} className="daybar" style={{ height: `${Math.max(2, d.count / max * 100)}%` }} title={`${fmtDate(d.day)}: ${d.count}`} />)}
          </div>
          <div className="daybars-axis muted small"><span>{fmtDate(data.from)}</span><span>today</span></div>
        </section>
        <section className="card">
          <div className="card-head"><div className="grow"><h2>Most read</h2></div></div>
          {data.top.length === 0 ? <p className="muted pad">No readers in this time yet. Share your posts!</p> :
            <div className="list">{data.top.map((p, i) => (
              <div key={p.id} className="list-row">
                <span className="rank tnum">{i + 1}</span>
                <Link to={`/posts/${p.id}`} className="row-main grow"><span className="row-title ellipsis">{p.title || 'Untitled'}</span></Link>
                <span className="bar-cell"><span style={{ width: `${p.views / data.top[0].views * 100}%` }} /></span>
                <span className="tnum small">{p.views.toLocaleString()}</span>
                {p.url && <a className="btn ghost icon-only sm" href={p.url} target="_blank" rel="noreferrer" aria-label="Open"><Icon name="external" size={14} /></a>}
              </div>
            ))}</div>}
        </section>
      </>}
    </div>
  )
}
