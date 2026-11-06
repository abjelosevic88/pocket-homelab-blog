import type { AppSlug, Post, Theme } from './types'
import { fmtDate, fmtDateTime } from './format'

export const THEMES: { id: Theme; name: string; help: string }[] = [
  { id: 'classic', name: 'Classic', help: 'Your name centred on top, one column of posts with their pictures. Fits almost anything.' },
  { id: 'magazine', name: 'Magazine', help: 'The newest post big on top, the others as cards in a grid. For blogs with many pictures.' },
  { id: 'minimal', name: 'Minimal', help: 'Just titles and dates, lots of white space. For writers.' },
  { id: 'journal', name: 'Journal', help: 'Warm paper colours, big pictures, classic letters. For travel and life stories.' },
]

export const ACCENTS = ['#3157d5', '#0f766e', '#b4532a', '#be123c', '#7c3aed', '#a16207', '#15803d', '#1f2937']

export const LANGUAGES: { value: string; label: string }[] = [
  { value: 'en', label: 'English' }, { value: 'bs', label: 'Bosanski' }, { value: 'hr', label: 'Hrvatski' }, { value: 'sr', label: 'Srpski' },
  { value: 'de', label: 'Deutsch' }, { value: 'fr', label: 'Français' }, { value: 'es', label: 'Español' }, { value: 'it', label: 'Italiano' },
  { value: 'pt', label: 'Português' }, { value: 'nl', label: 'Nederlands' }, { value: 'sl', label: 'Slovenščina' }, { value: 'pl', label: 'Polski' },
  { value: 'tr', label: 'Türkçe' }, { value: 'sv', label: 'Svenska' },
]

export const SOCIALS: { key: string; label: string; placeholder: string }[] = [
  { key: 'instagram', label: 'Instagram', placeholder: 'https://instagram.com/you' },
  { key: 'facebook', label: 'Facebook', placeholder: 'https://facebook.com/you' },
  { key: 'x', label: 'X', placeholder: 'https://x.com/you' },
  { key: 'mastodon', label: 'Mastodon', placeholder: 'https://mastodon.social/@you' },
  { key: 'bluesky', label: 'Bluesky', placeholder: 'https://bsky.app/profile/you' },
  { key: 'linkedin', label: 'LinkedIn', placeholder: 'https://linkedin.com/in/you' },
  { key: 'youtube', label: 'YouTube', placeholder: 'https://youtube.com/@you' },
  { key: 'github', label: 'GitHub', placeholder: 'https://github.com/you' },
  { key: 'website', label: 'Website', placeholder: 'https://…' },
  { key: 'email', label: 'Email', placeholder: 'you@example.com' },
]

export const APP_INFO: Record<AppSlug, { name: string; icon: string; noun: string; verb: string }> = {
  notes: { name: 'Pocket Notes', icon: 'note', noun: 'A note', verb: 'Turn a note into a post' },
  travel: { name: 'Pocket Travel', icon: 'plane', noun: 'A trip', verb: 'Write the story of a trip' },
  places: { name: 'Pocket Places', icon: 'pin', noun: 'A place', verb: 'Review a place you visited' },
  bookshelf: { name: 'Pocket Bookshelf', icon: 'journal', noun: 'A book', verb: 'Review a book you read' },
  kitchen: { name: 'Pocket Kitchen', icon: 'utensils', noun: 'A recipe', verb: 'Share a recipe' },
  watch: { name: 'Pocket Watch', icon: 'video', noun: 'A film or show', verb: 'Write about a film or show' },
  voice: { name: 'Pocket Voice', icon: 'mic', noun: 'A voice memo', verb: 'Turn a voice memo into a post' },
}

export function statusLabel(p: Pick<Post, 'status' | 'publish_at' | 'published_at'>): string {
  if (p.status === 'published') return p.published_at ? `Published ${fmtDate(p.published_at.slice(0, 10))}` : 'Published'
  if (p.status === 'scheduled') return p.publish_at ? `Goes out ${fmtDateTime(p.publish_at)}` : 'Scheduled'
  return 'Draft'
}

/** Markdown → a short plain line (for lists). */
export function plain(md: string, n = 160): string {
  const t = md.replace(/!\[[^\]]*\]\([^)]*\)/g, '').replace(/\[([^\]]*)\]\([^)]*\)/g, '$1').replace(/[#>*_`~-]+/g, ' ').replace(/\s+/g, ' ').trim()
  return t.length > n ? t.slice(0, n).replace(/\s+\S*$/, '') + '…' : t
}

export function wordCount(md: string): number { return (md.match(/[\p{L}\p{N}]+/gu) || []).length }

export function readingMinutes(words: number): number { return Math.max(1, Math.ceil(words / 200)) }

/** A web address part from a title: "Ljeto u Mostaru!" → "ljeto-u-mostaru". */
export function slugify(s: string): string {
  const map: Record<string, string> = { đ: 'd', Đ: 'd', ß: 'ss', æ: 'ae', ø: 'o', ł: 'l' }
  return s.replace(/[đĐßæøł]/g, c => map[c] || c).normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
    .replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 80)
}

/** A local "YYYY-MM-DDTHH:MM" for <input type=datetime-local>. */
export function localInput(d: Date): string {
  const p = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`
}
