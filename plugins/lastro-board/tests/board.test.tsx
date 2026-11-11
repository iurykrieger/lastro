import type { On } from 'claude-code'
import { expect, mock, test } from 'claude-code/testing'

import { readBoard, summary } from '../hooks/board'
import { toastFor } from '../hooks/register'
import { FAILED, REVIEW, RUNNING, SPECIFIED } from './fixtures'

type Files = Record<string, string | undefined>

const PANE = {
  plugin: 'lastro-board',
  component: 'Pane',
  requestId: 'lastro-board',
  props: { title: 'Lastro', isFocused: false, bodyColumns: 80, placement: 'dock', scroll: { offset: 0, bodyRows: 40 }, view: {} },
} as const

const BAND = {
  plugin: 'lastro-board',
  component: 'AbovePrompt',
  props: { hasSurvey: false, isCollapsed: false, isWorking: false, maxRows: 4, bodyColumns: 80, scroll: { offset: 0, bodyRows: 4 }, view: {} },
} as const

const NAMES: Record<string, string> = {
  '.lastro/config.json': 'config',
  '.lastro/proposals/P-001-shipping-fee.json': 'proposal',
  '.lastro/plugin/progress.jsonl': 'progress',
}

/** The engine beneath the mod: a workspace whose files the test swaps, and a mocked clock. */
function world(on: On, files: { current: Files }) {
  const toasts: string[] = []
  const clock = mock.clock(on)
  on('fs.read', (_, e) => {
    const name = Object.entries(NAMES).find(([suffix]) => e.path.endsWith(suffix))?.[1]
    const text = name === undefined ? undefined : files.current[name]
    if (text === undefined) throw new Error(`ENOENT ${e.path}`)
    return { value: text }
  })
  on('fs.list', (_, e) => {
    if (!e.path.endsWith('.lastro/proposals') || files.current.proposal === undefined) throw new Error(`ENOENT ${e.path}`)
    return { value: ['P-001-shipping-fee.json', 'P-001-shipping-fee.md'] }
  })
  on('command.register', () => ({ value: { command: 'lastro-board' } }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  on('ui.toast', (_, e) => {
    toasts.push(e.text)
    return { value: undefined }
  })
  on('ui.status', () => ({ value: undefined }))
  on('session.start', (_, e) => ({ cwd: e.cwd }))
  return { clock, toasts }
}

test('the board reads the real workspace at each stage', () => {
  const review = readBoard(REVIEW)!
  expect(review.phase).toBe('review-pending')
  expect(review.title).toBe('P-001 Shipping fee at checkout')
  expect(review.intents.map(i => `${i.id} ${i.useCase} ${i.approvals}/${i.required} ${i.status}`)).toEqual([
    'I-01 UC-001 1/2 pending',
    'I-02 UC-001 1/2 pending',
    'I-03 UC-001 1/2 pending',
    'I-04 UC-001 1/2 pending',
  ])
  expect(summary(review)).toBe('Lastro P-001 · review: 0/4 intents approved · 1/1 decisions answered')

  const specified = readBoard(SPECIFIED)!
  expect(specified.phase).toBe('implementing')
  expect(specified.proofs.map(p => `${p.id} ${p.status}`)).toEqual(['PF-01 pending', 'PF-02 pending', 'PF-03 pending', 'PF-04 pending'])
  expect(specified.beyondProof).toEqual(['I-04 (termination-timing)'])

  const running = readBoard(RUNNING)!
  expect(running.phase).toBe('running')
  expect(running.runs).toBe(800)
  expect(running.proofs.map(p => `${p.id} ${p.status}`)).toEqual(['PF-01 violated', 'PF-02 running', 'PF-03 running', 'PF-04 running'])
  expect(summary(running)).toBe('Lastro P-001 · proving… 1/4 checked · 800 runs')

  const failed = readBoard(FAILED)!
  expect(failed.phase).toBe('failed')
  expect(failed.proofs.map(p => `${p.id} ${p.status}`)).toEqual(['PF-01 violated', 'PF-02 proved', 'PF-03 proved', 'PF-04 evidenced'])
  expect(failed.proofs[0]?.counterexample).toBe('{"member":false,"subtotal":100} → {"fee":15}')
  expect(summary(failed)).toBe('Lastro P-001 · 3/4 proofs passed · 1 failed')
})

test('toasts: every intent approved, and the end of a proof run', () => {
  expect(toastFor(readBoard(REVIEW), readBoard(SPECIFIED))).toBe('Lastro: all 4 intents approved')
  expect(toastFor(readBoard(RUNNING), readBoard(FAILED))).toBe('Lastro: 1 of 4 proofs failed')
  expect(toastFor(readBoard(SPECIFIED), readBoard(SPECIFIED))).toBe(undefined)
})

test('the pane shows approvals, then each proof as the run reports it, on every surface', async ($, on) => {
  const files = { current: REVIEW as Files }
  const { clock, toasts } = world(on, files)
  await $.session.start({ cwd: '/repo', surface: 'desktop', isInteractive: true })

  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ ...PANE, surface })
    expect((await ui.find({ type: 'Text', text: /^Review/ }))?.text).toBe('Review · 0/4 intents approved')
    expect((await ui.find({ type: 'Text', text: /I-01/ }))?.text).toContain('(1/2)')
    await ui.unmount()
  }

  // The second reviewer approves and the proofs are derived: the pane lists them, pending.
  files.current = SPECIFIED
  await clock.advance(1000)
  expect(toasts).toEqual(['Lastro: all 4 intents approved'])

  // The Stop hook starts proving: PF-01 has failed, the others are still running.
  files.current = RUNNING
  await clock.advance(1000)
  let ui = await $.ui.mount({ ...PANE, surface: 'desktop' })
  expect((await ui.find({ type: 'Text', text: /PF-01/ }))?.text).toContain('✗ PF-01')
  expect((await ui.find({ type: 'Text', text: /PF-02/ }))?.text).toContain('… PF-02')
  expect((await ui.find({ type: 'Text', text: /counterexample/ }))?.text).toContain('{"member":false,"subtotal":100}')
  await ui.unmount()

  // The run ends.
  files.current = FAILED
  await clock.advance(1000)
  ui = await $.ui.mount({ ...PANE, surface: 'desktop' })
  expect((await ui.find({ type: 'Text', text: /PF-02/ }))?.text).toContain('✓ PF-02')
  expect((await ui.find({ type: 'Text', text: /^Proofs/ }))?.text).toBe('Proofs · 3/4 passed')
  await ui.unmount()
  expect(toasts).toEqual(['Lastro: all 4 intents approved', 'Lastro: 1 of 4 proofs failed'])

  const band = await $.ui.mount({ ...BAND, surface: 'terminal' })
  expect((await band.find({ type: 'Text', text: /Lastro/ }))?.text).toContain('3/4 proofs passed · 1 failed')
  await band.press({ key: 'hide' })
  await band.unmount()
})

test('without a Lastro workspace the mod stays out of the way', async ($, on) => {
  const files = { current: {} as Files }
  world(on, files)
  await $.session.start({ cwd: '/elsewhere', surface: 'terminal', isInteractive: true })
  const ui = await $.ui.mount({ ...PANE, surface: 'terminal' })
  expect((await ui.find({ type: 'Text' }))?.text).toBe('No Lastro workspace in this folder.')
  await ui.unmount()
})
