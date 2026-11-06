// The app shell: sidebar, top bar, search palette, shortcuts — the Pocket Calendar layout.
import { createContext, useCallback, useContext, useEffect, useRef, useState } from 'react'
import { Navigate, Outlet, useLocation, useNavigate } from 'react-router-dom'
import { api, V1 } from '../lib/api'
import { useApp } from '../lib/app-context'
import { DataProvider, useData } from '../lib/data'
import { Icon } from './icons'
import { Sidebar } from './Sidebar'
import { CommandPalette, type PaletteCommand } from './CommandPalette'
import { ShortcutsSheet } from './ShortcutsSheet'
import { StartFrom } from './StartFrom'
import { ImportModal } from './ImportModal'
import { Avatar, Loading, type MenuItem, MenuButton, modalOpen, useIsMobile, useMediaQuery } from './ui'
import { AskButton } from './AskButton'

export { announceChange, useOnChange } from '../lib/data'

const SIDEBAR_KEY = 'pbg-sidebar'

/** The page's own name for the top bar breadcrumb. */
const CrumbCtx = createContext<(t: string) => void>(() => {})
export function useCrumb(title: string | undefined) {
  const set = useContext(CrumbCtx)
  useEffect(() => { set(title || ''); return () => set('') }, [title, set])
}

export type CreateKind = 'post' | 'page' | 'from-app' | 'import'

/** Start something from anywhere (a page's button). */
export const openCreate = (k: CreateKind, app?: string, item?: { id: string; title: string }) =>
  window.dispatchEvent(new CustomEvent('pbg:create', { detail: { kind: k, app, item } }))

/** The write menu: the sidebar's caret. */
export function createItems(onCreate: (k: CreateKind) => void): MenuItem[] {
  return [
    { label: 'New post', icon: 'pen', hint: 'N', onSelect: () => onCreate('post') },
    { label: 'New page (About, Contact…)', icon: 'file', hint: 'P', onSelect: () => onCreate('page') },
    { label: 'Start from your apps…', icon: 'layers', hint: 'A', onSelect: () => onCreate('from-app') },
    'sep',
    { label: 'Bring posts from WordPress or files…', icon: 'upload', onSelect: () => onCreate('import') },
  ]
}

export default function Layout() {
  return <DataProvider><Shell /></DataProvider>
}

function Shell() {
  const navigate = useNavigate()
  const loc = useLocation()
  const { blogs } = useData()
  const mobile = useIsMobile()
  const narrow = useMediaQuery('(max-width: 1099px)')
  const drawer = mobile || narrow
  const [sidebarOpen, setSidebarOpen] = useState(() => !drawer && localStorage.getItem(SIDEBAR_KEY) !== '0')
  useEffect(() => { setSidebarOpen(!drawer && localStorage.getItem(SIDEBAR_KEY) !== '0') }, [drawer])
  const drawerOpenedAt = useRef(0)
  const toggleSidebar = () => setSidebarOpen(o => { drawerOpenedAt.current = performance.now(); const n = !o; if (!drawer) localStorage.setItem(SIDEBAR_KEY, n ? '1' : '0'); return n })
  const closeDrawer = () => { if (drawer) setSidebarOpen(false) }
  useEffect(() => { document.querySelector('.content')?.scrollTo({ top: 0 }) }, [loc.pathname])

  const [crumb, setCrumbRaw] = useState('')
  const setCrumb = useCallback((t: string) => setCrumbRaw(t), [])

  const [palette, setPalette] = useState(false)
  const [shortcuts, setShortcuts] = useState(false)
  const [fromApp, setFromApp] = useState<{ app?: string; item?: { id: string; title: string } } | null>(null)
  const [importing, setImporting] = useState(false)

  const create = (k: CreateKind, app?: string, item?: { id: string; title: string }) => {
    closeDrawer()
    if (k === 'post') navigate('/write')
    else if (k === 'page') navigate('/write?kind=page')
    else if (k === 'import') setImporting(true)
    else setFromApp({ app, item })
  }
  const createRef = useRef(create)
  createRef.current = create

  useEffect(() => {
    const open = () => setShortcuts(true)
    const make = (e: Event) => { const d = (e as CustomEvent<{ kind: CreateKind; app?: string; item?: { id: string; title: string } }>).detail; createRef.current(d.kind, d.app, d.item) }
    window.addEventListener('pbg:shortcuts', open)
    window.addEventListener('pbg:create', make)
    return () => { window.removeEventListener('pbg:shortcuts', open); window.removeEventListener('pbg:create', make) }
  }, [])
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      const t = e.target as HTMLElement
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') { e.preventDefault(); setPalette(p => !p); return }
      if (e.metaKey || e.ctrlKey || e.altKey) return
      if (t.closest('input, textarea, select, [contenteditable="true"]') || modalOpen() || document.querySelector('.menu, .popover, .sel-pop')) return
      const go = (to: string) => { e.preventDefault(); navigate(to) }
      const mk = (k: CreateKind) => { e.preventDefault(); createRef.current(k) }
      switch (e.key) {
        case '/': e.preventDefault(); setPalette(true); break
        case '?': e.preventDefault(); setShortcuts(true); break
        case 'n': mk('post'); break
        case 'p': mk('page'); break
        case 'a': mk('from-app'); break
        case 'h': go('/'); break
        case 'o': go('/posts'); break
        case 'c': go('/comments'); break
        case 'i': go('/pictures'); break
        case 'r': go('/readers'); break
        case 'l': go('/look'); break
        case ',': go('/settings'); break
        case '[': e.preventDefault(); toggleSidebar(); break
      }
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }) // eslint-disable-line react-hooks/exhaustive-deps

  const commands: PaletteCommand[] = [
    { id: 'post', label: 'Write a new post', icon: 'pen', hint: 'N', keywords: 'new article write story', run: () => create('post') },
    { id: 'page', label: 'New page', icon: 'file', hint: 'P', keywords: 'about contact menu', run: () => create('page') },
    { id: 'from-app', label: 'Start a post from your apps', icon: 'layers', hint: 'A', keywords: 'trip travel book recipe place film note memo voice', run: () => create('from-app') },
    { id: 'import', label: 'Bring posts from WordPress or files', icon: 'upload', keywords: 'import wordpress markdown hugo jekyll', run: () => create('import') },
    { id: 'home', label: 'Overview', icon: 'home', hint: 'H', run: () => navigate('/') },
    { id: 'posts', label: 'Posts', icon: 'list', hint: 'O', keywords: 'drafts published scheduled', run: () => navigate('/posts') },
    { id: 'drafts', label: 'Drafts', icon: 'edit', keywords: 'unfinished', run: () => navigate('/posts?status=draft') },
    { id: 'pages', label: 'Pages', icon: 'file', keywords: 'about menu', run: () => navigate('/pages') },
    { id: 'comments', label: 'Comments', icon: 'message', hint: 'C', keywords: 'readers approve spam reply', run: () => navigate('/comments') },
    { id: 'pictures', label: 'Pictures', icon: 'image', hint: 'I', keywords: 'photos images media upload', run: () => navigate('/pictures') },
    { id: 'readers', label: 'Readers', icon: 'chart', hint: 'R', keywords: 'stats statistics views visitors', run: () => navigate('/readers') },
    { id: 'look', label: 'Look & details', icon: 'palette', hint: 'L', keywords: 'theme design colour color name address menu about', run: () => navigate('/look') },
    { id: 'new-blog', label: 'Start another blog', icon: 'plus', keywords: 'new blog', run: () => navigate('/start') },
    { id: 'settings', label: 'Settings', icon: 'settings', hint: ',', run: () => navigate('/settings') },
    { id: 'ai', label: 'Settings: AI writing help', icon: 'sparkles', keywords: 'pocket ai openai voice', run: () => navigate('/settings/ai') },
    { id: 'apps', label: 'Settings: connected apps', icon: 'layers', keywords: 'notes travel places bookshelf kitchen watch voice core', run: () => navigate('/settings/apps') },
    { id: 'export', label: 'Settings: export your blog', icon: 'download', keywords: 'backup website zip markdown', run: () => navigate('/settings/data') },
    { id: 'shortcuts', label: 'Keyboard shortcuts', icon: 'keyboard', hint: '?', run: () => setShortcuts(true) },
  ]

  if (blogs && blogs.length === 0 && !loc.pathname.startsWith('/start')) return <Navigate to="/start" replace />
  return (
    <CrumbCtx.Provider value={setCrumb}>
      <div className={`app ${sidebarOpen ? 'with-sidebar' : 'no-sidebar'} ${mobile ? 'is-mobile' : ''} ${drawer ? 'drawer-mode' : ''}`}>
        {drawer && sidebarOpen && <div className="drawer-backdrop" onClick={e => { if (e.timeStamp - drawerOpenedAt.current > 400) setSidebarOpen(false) }} />}
        <div className={`sidebar-wrap ${sidebarOpen ? 'open' : ''}`}>
          <Sidebar onCreate={create} onNavigate={closeDrawer} />
        </div>
        <div className="main">
          <TopBar onMenu={toggleSidebar} onPalette={() => setPalette(true)} mobile={mobile} compact={narrow} sidebarOpen={sidebarOpen} crumb={crumb} />
          <main className="content"><div className="content-inner">{blogs ? <Outlet /> : <Loading />}</div></main>
        </div>
        {mobile && !loc.pathname.startsWith('/posts/') && !loc.pathname.startsWith('/write') && <button type="button" className="fab" aria-label="Write a post" title="Write a post" onClick={() => create('post')}><Icon name="pen" size={24} strokeWidth={2.2} /></button>}
        {palette && <CommandPalette onClose={() => setPalette(false)} commands={commands} />}
        {shortcuts && <ShortcutsSheet onClose={() => setShortcuts(false)} />}
        {fromApp && <StartFrom app={fromApp.app} item={fromApp.item} onClose={() => setFromApp(null)} onDone={id => { setFromApp(null); navigate(`/posts/${id}`) }} />}
        {importing && <ImportModal onClose={() => setImporting(false)} />}
      </div>
    </CrumbCtx.Provider>
  )
}

const SECTION: Record<string, [string, string]> = { '': ['Overview', '/'], posts: ['Posts', '/posts'], write: ['Posts', '/posts'], pages: ['Pages', '/pages'], comments: ['Comments', '/comments'], pictures: ['Pictures', '/pictures'], readers: ['Readers', '/readers'], look: ['Look & details', '/look'], start: ['New blog', '/start'] }

function TopBar({ onMenu, onPalette, mobile, compact, sidebarOpen, crumb }: { onMenu: () => void; onPalette: () => void; mobile: boolean; compact: boolean; sidebarOpen: boolean; crumb: string }) {
  const { user, themeMode, setThemeMode, status, setUser } = useApp()
  const navigate = useNavigate()
  const loc = useLocation()
  const [section, sectionTo] = SECTION[loc.pathname.split('/')[1] || ''] || ['', '/']
  const signOut = async () => { await api.post(`${V1}/auth/logout`).catch(() => {}); setUser(null); navigate('/login') }
  const avatarMenu = (): MenuItem[] => [
    { heading: user?.name || user?.email || '' },
    { label: 'Profile', icon: 'user', onSelect: () => navigate('/settings/profile') },
    { label: 'Settings', icon: 'settings', onSelect: () => navigate('/settings'), hint: ',' },
    { label: 'Keyboard shortcuts', icon: 'keyboard', onSelect: () => window.dispatchEvent(new CustomEvent('pbg:shortcuts')), hint: '?' },
    'sep',
    { heading: 'Appearance' },
    { label: 'System', checked: themeMode === 'system', onSelect: () => setThemeMode('system') },
    { label: 'Light', checked: themeMode === 'light', onSelect: () => setThemeMode('light') },
    { label: 'Dark', checked: themeMode === 'dark', onSelect: () => setThemeMode('dark') },
    ...(status?.auth_mode === 'local' ? ['sep' as const, { label: 'Sign out', icon: 'log-out', onSelect: signOut }] : []),
  ]
  return (
    <header className="topbar">
      <button type="button" className="btn ghost icon-only" onClick={onMenu} aria-label={sidebarOpen ? 'Hide the sidebar' : 'Show the sidebar'} title="Menu ([)"><Icon name={mobile || compact ? 'menu' : 'sidebar'} size={18} /></button>
      <h1 className="range-title">
        {crumb ? <><button type="button" className="crumb" onClick={() => navigate(sectionTo)}>{section}</button><Icon name="chevron-right" size={16} className="crumb-sep" /><span className="ellipsis">{crumb}</span></> : section}
      </h1>
      <span className="spacer" />
      <button type="button" className="btn ghost icon-only" onClick={onPalette} aria-label="Search" title="Search (⌘K or /)"><Icon name="search" size={18} /></button>
      <AskButton />
      <MenuButton label="Account" className="avatar-btn" align="right" width={220} items={avatarMenu}>
        <Avatar name={user?.name} email={user?.email} size={30} />
      </MenuButton>
    </header>
  )
}
