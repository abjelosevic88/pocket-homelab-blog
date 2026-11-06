// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds a lived-in blog):
//
//   DATA_DIR=/tmp/blog-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Ana PORT=18246 ./pocket-blog
//   CDP=http://127.0.0.1:9335 BASE=http://127.0.0.1:18246 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18246'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|Content Security Policy/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1500, frozen = true) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  if (frozen) await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png' }) }
const postId = async (page, q) => page.evaluate(t => fetch('/api/v1/search?q=' + encodeURIComponent(t)).then(r => r.json()).then(j => j.results[0].id), q)

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 2000)
  assert.ok((await page.content()).includes('Slow Roads'), 'the overview shows the blog')
  await shot(page, 'desktop-overview')
  const mostar = await postId(page, 'Mostar')
  await go(page, `/posts/${mostar}`, 2500)
  assert.ok((await page.content()).includes('Before the buses arrive'), 'the editor shows the post')
  await shot(page, 'desktop-editor')
  await page.getByRole('button', { name: /Help me write/ }).click()
  await page.waitForTimeout(800)
  await shot(page, 'desktop-help')
  await go(page, '/posts', 1500)
  await shot(page, 'desktop-posts')
  await go(page, '/look', 3000)
  await shot(page, 'desktop-look')
  await go(page, '/readers', 1500)
  await shot(page, 'desktop-readers')
  await go(page, '/comments?status=approved', 1500)
  await shot(page, 'desktop-comments')
  await go(page, '/b/slow-roads/', 1500, false)
  assert.ok((await page.content()).includes('A weekend in Mostar'), 'the public blog lists posts')
  await shot(page, 'desktop-blog')
  await go(page, '/b/slow-roads/weekend-in-mostar', 1500, false)
  await shot(page, 'desktop-post')
  for (const t of ['classic', 'magazine', 'minimal']) {
    await go(page, `/b/slow-roads/?theme=${t}&accent=%233157d5`, 1200, false)
    await shot(page, `desktop-theme-${t}`)
  }
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 2000)
  await shot(page, 'phone-overview')
  const mostar = await postId(page, 'Mostar')
  await go(page, `/posts/${mostar}`, 2500)
  await shot(page, 'phone-editor')
  await go(page, '/b/slow-roads/', 1500, false)
  await shot(page, 'phone-blog')
  await go(page, '/b/slow-roads/grandmas-zeljanica', 1500, false)
  await shot(page, 'phone-post')
  await ctx.close()
}

// ---- dark ----
{
  const { ctx, page } = await session({ dark: true })
  await go(page, '/', 2000)
  await shot(page, 'desktop-overview-dark')
  await go(page, '/b/slow-roads/hiking-prenj', 1500, false)
  await shot(page, 'desktop-post-dark')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
