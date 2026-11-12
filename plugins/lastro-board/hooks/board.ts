// Reads the Lastro workspace files into the board the pane draws. Pure: the files come in as text, so the
// tests feed it what the CLI and the plugin's Stop hook write. The phases match plugins/lastro/scripts/lib.mjs.
import type { Board, IntentRow, Phase, ProofRow } from '../types'

export type Files = {
  config?: string
  /** The current proposal: the latest .lastro/proposals/<id>-<slug>.json that is not applied or closed. */
  proposal?: string
  /** .lastro/plugin/progress.jsonl: the last proof run's `prove --stream` events. */
  progress?: string
}

type Json = any // eslint-disable-line @typescript-eslint/no-explicit-any

export const parse = (text: string | undefined): Json => {
  if (!text) return undefined
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

/** A proposal that still needs work: not applied, not closed. */
export const isOpen = (p: Json) => !!p && p.status !== 'applied' && p.status !== 'closed'

const PASSING = new Set(['proved', 'evidenced'])

const json = (v: unknown) => JSON.stringify(v)

export function readBoard(files: Files): Board | null {
  if (!parse(files.config)) return null
  const p = parse(files.proposal)
  const empty: Board = { title: '', phase: 'no-proposal', intents: [], decisions: [], proofs: [], beyondProof: [], regressions: [], runs: 0 }
  if (!isOpen(p)) return empty

  const short = (id: string) => (id.startsWith(`${p.id}.`) ? id.slice(p.id.length + 1) : id)
  const required = Math.max(1, Number(p.policy?.approvals ?? 1))
  const counts = (i: Json) =>
    i.status === 'approved' || i.status === 'amended'
      ? (i.approvals ?? []).filter((a: string) => p.policy?.submitterCanApprove !== false || a !== p.submittedBy).length
      : 0
  const intents: IntentRow[] = (p.intents ?? []).map((i: Json) => {
    const approvals = counts(i)
    const status = i.status === 'rejected' ? 'rejected' : approvals >= required ? 'approved' : 'pending'
    return { id: short(i.id), useCase: i.useCase, statement: i.description, status, approvals, required, replaces: (i.replaces ?? []).map((r: Json) => r.proof) }
  })
  const decisions = (p.decisions ?? []).map((d: Json) => ({ id: d.id, question: d.question, answered: d.chosen !== undefined && d.chosen !== null }))
  const board: Board = {
    ...empty,
    title: `${p.id} ${p.name}`,
    intents,
    decisions,
    beyondProof: (p.intents ?? []).filter((i: Json) => i.provability === 'beyond-proof' && i.status !== 'rejected').map((i: Json) => `${short(i.id)} (${i.beyondProofReason})`),
  }
  if (!p.hash) return { ...board, phase: 'review-pending' }
  if (!(p.proofs ?? []).length) return { ...board, phase: 'needs-proofs' }

  const rows = new Map<string, ProofRow>(
    (p.proofs as Json[]).map((x) => [x.id, { id: short(x.id), intents: [short(x.intent)], description: x.description ?? '', status: 'pending' }]),
  )
  // The last finished run of this proposal.
  const last = p.lastRun
  if (last) {
    for (const o of last.outcomes ?? []) if (o.role === 'proposal') setVerdict(rows, o)
    board.regressions = (last.regressions ?? []).slice()
  }

  // A run under way (or just finished) overrides it, proof by proof, as its events arrive.
  const events: Json[] = (files.progress ?? '')
    .split('\n')
    .map((l) => parse(l))
    .filter(Boolean)
  const start = events.findIndex((e) => e.event === 'build')
  const run = start >= 0 ? events.slice(start) : []
  const started = run.find((e) => e.event === 'start')
  const result = run.find((e) => e.event === 'result')
  const same = !started || (started.proposal === p.id && json(started.proofs) === json(p.proofs.map((x: Json) => x.id)))
  let phase: Phase = last ? (last.ok ? 'passed' : 'failed') : 'implementing'
  if (run.length && same) {
    if (!result) {
      phase = 'running'
      for (const row of rows.values()) row.status = 'running'
      board.runs = Math.max(0, ...run.filter((e) => e.event === 'progress').map((e) => Number(e.runs)))
    }
    const regressions: string[] = []
    for (const e of run.filter((x) => x.event === 'proof')) {
      if (e.role === 'proposal') setVerdict(rows, e)
      else if (e.role === 'regression' && !e.passed) regressions.push(e.proof)
    }
    if (run.some((x) => x.event === 'proof')) board.regressions = regressions
    if (result?.error) {
      board.error = String(result.error)
      phase = 'failed'
    } else if (result) phase = result.ok ? 'passed' : 'failed'
  }
  return { ...board, phase, proofs: [...rows.values()] }
}

function setVerdict(rows: Map<string, ProofRow>, r: Json) {
  const row = rows.get(r.proof)
  if (!row) return
  row.status = r.verdict
  row.counterexample = r.counterexample ? `${json(r.counterexample.input)} → ${json(r.counterexample.output)}` : undefined
}

export const isPassing = (row: ProofRow) => PASSING.has(row.status)

/** One line: for the band above the prompt and the status line. */
export function summary(b: Board): string {
  const approved = b.intents.filter((i) => i.status === 'approved').length
  const answered = b.decisions.filter((d) => d.answered).length
  const passed = b.proofs.filter(isPassing).length
  const failed = b.proofs.filter((p) => ['violated', 'weak', 'vacuous', 'not-run'].includes(p.status)).length
  const head = b.title ? `Lastro ${b.title.split(' ')[0]}` : 'Lastro'
  const reg = b.regressions.length ? ` · ${b.regressions.length} regressions` : ''
  switch (b.phase) {
    case 'none':
    case 'no-proposal':
      return 'Lastro · no proposal yet'
    case 'review-pending':
      return `${head} · review: ${approved}/${b.intents.length} intents approved · ${answered}/${b.decisions.length} decisions answered`
    case 'needs-proofs':
      return `${head} · ${approved}/${b.intents.length} intents approved · proofs not derived yet`
    case 'implementing':
      return `${head} · ${b.proofs.length} proofs pending`
    case 'running':
      return `${head} · proving… ${passed + failed}/${b.proofs.length} checked${b.runs ? ` · ${b.runs} runs` : ''}`
    case 'passed':
      return `${head} · ${passed}/${b.proofs.length} proofs passed`
    case 'failed':
      return b.error ? `${head} · the proof run failed` : `${head} · ${passed}/${b.proofs.length} proofs passed · ${failed} failed${reg}`
  }
}
