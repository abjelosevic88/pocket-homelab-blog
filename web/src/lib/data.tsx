// The person's blogs and the one they are working on, shared by every page.
import React, { createContext, useCallback, useContext, useEffect, useState } from 'react'
import { api, V1 } from './api'
import type { Blog } from './types'

const KEY = 'pbg-blog'
const EVENT = 'pbg:changed'

interface DataState {
  blogs: Blog[] | null
  blog: Blog | null
  setBlogID: (id: number) => void
  reload: () => Promise<void>
}
const Ctx = createContext<DataState>({ blogs: null, blog: null, setBlogID: () => {}, reload: async () => {} })
export function useData() { return useContext(Ctx) }

/** Something changed (a post saved, a comment approved): lists and counts reload. */
export function announceChange() { window.dispatchEvent(new CustomEvent(EVENT)) }

export function useOnChange(fn: () => void) {
  useEffect(() => {
    window.addEventListener(EVENT, fn)
    return () => window.removeEventListener(EVENT, fn)
  }, [fn])
}

export function DataProvider({ children }: { children: React.ReactNode }) {
  const [blogs, setBlogs] = useState<Blog[] | null>(null)
  const [id, setID] = useState<number>(() => Number(localStorage.getItem(KEY)) || 0)
  const reload = useCallback(async () => { try { setBlogs(await api.get<Blog[]>(`${V1}/blogs`)) } catch { setBlogs(b => b || []) } }, [])
  useEffect(() => { reload() }, [reload])
  useOnChange(reload)
  const setBlogID = useCallback((n: number) => { setID(n); localStorage.setItem(KEY, String(n)) }, [])
  const blog = blogs?.find(b => b.id === id) || blogs?.[0] || null
  return <Ctx.Provider value={{ blogs, blog, setBlogID, reload }}>{children}</Ctx.Provider>
}
