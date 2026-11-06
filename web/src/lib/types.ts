export interface User { id: number; email: string; name: string; role: 'admin' | 'member'; created_at: string }
export interface APIToken { id: number; user_id: number; name: string; prefix: string; last_used_at: string | null; created_at: string }

export type ConnMode = 'core' | 'direct' | 'off'
export type ConnVia = '' | 'core' | 'direct'
export type AppSlug = 'notes' | 'travel' | 'places' | 'bookshelf' | 'kitchen' | 'watch' | 'voice'

type ConnFields = { [K in AppSlug as `${K}_mode`]: ConnMode } & { [K in AppSlug as `${K}_via`]: ConnVia } & { [K in AppSlug as `${K}_detail` | `${K}_url` | `${K}_token` | `${K}_public_url` | `${K}_open_url`]: string } & { [K in AppSlug as `${K}_configured`]: boolean }

export type Settings = ConnFields & {
  core_configured: boolean
  core_registered?: boolean
  title: string; date_format: string; time_format: string; week_start: number
  autosave: boolean; ai_writing: boolean; ai_voice: string; notes_folder: string
  notify_comment: boolean; notify_published: boolean; notify_digest: boolean; notify_hour: number
  notify_url: string; notify_format: string; notify_configured: boolean
  ai_provider: 'auto' | 'pocket' | 'openai'; ai_pocket_model: string; openai_api_key: string; openai_model: string; openai_base_url: string
  setup_complete: boolean; base_url: string; public_url: string; demo: boolean; max_upload_mb: number
}

export type Theme = 'classic' | 'magazine' | 'minimal' | 'journal'
export type Status = 'draft' | 'scheduled' | 'published'

export interface NavLink { label: string; url: string }

export interface Blog {
  id: number; handle: string; title: string; tagline: string; about: string; author_name: string; author_bio: string; avatar: string
  theme: Theme; accent: string; font: 'serif' | 'sans'; language: string; visibility: 'public' | 'unlisted' | 'private'
  comments: 'off' | 'moderated' | 'open'; per_page: number; show_reading: boolean; footer: string; nav: NavLink[]; socials: Record<string, string>
  domain: string; created_at: string; updated_at: string
  url: string; posts: number; drafts: number; pending_comments: number; last_post_at?: string
}

export interface Post {
  id: number; blog_id: number; kind: 'post' | 'page'; title: string; slug: string; body: string; excerpt: string; cover: string; cover_alt: string
  tags: string; status: Status; publish_at: string; published_at: string; featured: boolean; in_menu: boolean; menu_order: number
  comments: boolean; seo_description: string; preview_key: string; words: number; views: number; source: string; source_ref: string
  created_at: string; updated_at: string; url?: string; cover_url?: string; comments_count: number; pending_count: number
}

export interface Media { id: number; file: string; name: string; mime: string; bytes: number; width: number; height: number; alt: string; source: string; created_at: string; url: string; used: number }

export interface Comment {
  id: number; post_id: number; parent_id: number | null; name: string; email?: string; body: string; status: 'pending' | 'approved' | 'spam'
  by_author: boolean; created_at: string; post_title?: string; post_url?: string; blog_id?: number
}

export interface TagCount { tag: string; count: number }
export interface DayCount { day: string; count: number }
export interface Stats { days: number; from: string; views: number; previous: number; by_day: DayCount[]; top: { id: number; title: string; slug: string; views: number; url?: string }[]; comments: number }

export interface SourceApp { slug: AppSlug; name: string; label: string; reason: string; connected: boolean; detail: string }
export interface SourceItem { id: string; title: string; subtitle?: string; date?: string; rating?: number; why?: string; app?: AppSlug }

export interface AIResult {
  title?: string; body?: string; text?: string; titles?: string[]; excerpt?: string; seo_description?: string; tags?: string[]
  tagline?: string; about?: string; handle?: string; provider?: string
}
