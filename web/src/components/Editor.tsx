// The writing surface: the post is edited as it will read (Milkdown Crepe,
// like Pocket Notes) and stays Markdown on the server. "/" opens the list of
// blocks (heading, list, quote, picture, table…), selecting text shows the
// formatting bar, a picture can be dropped or pasted straight in.
import { forwardRef, useEffect, useImperativeHandle, useRef } from 'react'
import { Crepe, CrepeFeature } from '@milkdown/crepe'
import { editorViewCtx } from '@milkdown/kit/core'
import { insert, replaceAll } from '@milkdown/kit/utils'
import '@milkdown/crepe/theme/common/prosemirror.css'
import '@milkdown/crepe/theme/common/reset.css'
import '@milkdown/crepe/theme/common/block-edit.css'
import '@milkdown/crepe/theme/common/cursor.css'
import '@milkdown/crepe/theme/common/link-tooltip.css'
import '@milkdown/crepe/theme/common/list-item.css'
import '@milkdown/crepe/theme/common/placeholder.css'
import '@milkdown/crepe/theme/common/toolbar.css'
import '@milkdown/crepe/theme/common/table.css'
import '@milkdown/crepe/theme/common/image-block.css'
import '../styles/editor.css'

export interface EditorHandle {
  /** The text the person has selected ("" when none). */
  selection: () => string
  /** Put Markdown where the cursor is (replacing a selection). */
  insert: (md: string) => void
  /** Replace the whole post. */
  replace: (md: string) => void
  focus: () => void
}

interface Props {
  /** Changes when another post is opened: the editor starts over. */
  docKey: string | number
  value: string
  placeholder?: string
  onChange: (md: string) => void
  onUpload: (file: File) => Promise<string>
}

export const Editor = forwardRef<EditorHandle, Props>(function Editor({ docKey, value, placeholder, onChange, onUpload }, ref) {
  const root = useRef<HTMLDivElement>(null)
  const crepe = useRef<Crepe | null>(null)
  const ready = useRef(false)
  const change = useRef(onChange)
  change.current = onChange
  const upload = useRef(onUpload)
  upload.current = onUpload

  useEffect(() => {
    if (!root.current) return
    ready.current = false
    const up = (f: File) => upload.current(f)
    const c = new Crepe({
      root: root.current,
      defaultValue: value,
      features: { [CrepeFeature.Latex]: false, [CrepeFeature.TopBar]: false, [CrepeFeature.AI]: false, [CrepeFeature.CodeMirror]: false },
      featureConfigs: {
        [CrepeFeature.Placeholder]: { text: placeholder || 'Start writing… (type / for a heading, list, quote or picture)', mode: 'doc' },
        [CrepeFeature.ImageBlock]: {
          onUpload: up, inlineOnUpload: up, blockOnUpload: up,
          blockUploadPlaceholderText: 'or paste a picture’s web address',
          blockCaptionPlaceholderText: 'Write a caption (also read aloud to blind readers)',
          inlineUploadPlaceholderText: 'or paste a web address',
        },
      },
    })
    c.on(l => l.markdownUpdated((_ctx, md, prev) => { if (ready.current && md !== prev) change.current(md) }))
    let alive = true
    c.create().then(() => { if (alive) { crepe.current = c; ready.current = true } })
    return () => { alive = false; ready.current = false; crepe.current = null; c.destroy() }
  }, [docKey]) // eslint-disable-line react-hooks/exhaustive-deps

  useImperativeHandle(ref, () => ({
    selection: () => {
      const c = crepe.current
      if (!c) return ''
      return c.editor.action(ctx => {
        const v = ctx.get(editorViewCtx)
        const { from, to } = v.state.selection
        return from === to ? '' : v.state.doc.textBetween(from, to, '\n\n')
      })
    },
    insert: (md: string) => { crepe.current?.editor.action(insert(md)) },
    replace: (md: string) => { crepe.current?.editor.action(replaceAll(md, true)); change.current(md) },
    focus: () => { crepe.current?.editor.action(ctx => ctx.get(editorViewCtx).focus()) },
  }), [])

  return <div className="post-editor" ref={root} />
})
