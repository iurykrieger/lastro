// Reads the Lastro workspace files into the board the pane draws. Pure: the files come in as text, so the
// tests feed it what the CLI and the plugin's Stop hook write. The phases match plugins/lastro/scripts/lib.mjs.
import type { Board, IntentRow, Phase, ProofRow } from '../types'

export type Files = {
  config?: string
  intents?: string
  spec?: string
  proof?: string
  /** .lastro/plugin/progress.jsonl: the last proof run's `prove --stream` events. */
  progress?: string
}

type Json = any // eslint-disable-line @typescript-eslint/no-explicit-any

const parse = (text: string | undefined): Json => {
  if (!text) return undefined
  try {
    return JSON.parse(text)
  } catch {
    return undefined
  }
}

const reviewKey = (a: Json) =>
  a
    ? JSON.stringify({
        intents: (a.intents ?? []).map((i: Json) => [i.id, i.statement, i.status]),
        decisions: (a.decisions ?? []).map((d: Json) => [d.id, d.chosen ?? null]),
      })
    : null

const PASSING = new Set(['proved', 'proved-under-assumptions', 'evidenced'])

const short = (v: unknown) => JSON.stringify(v)

export function readBoard(files: Files): Board | null {
  const config = parse(files.config)
  if (!config) return null
  const artifact = parse(files.intents)
  const required = Number(config.requiredApprovals ?? 1)
  const model = String(config.model?.name ?? '')
  const empty: Board = { model, phase: 'no-proposal', intents: [], decisions: [], proofs: [], beyondProof: [], runs: 0 }
  if (!artifact) return empty

  const intents: IntentRow[] = (artifact.intents ?? []).map((i: Json) => {
    const approvals = (i.approvals ?? []).length
    const status = i.status === 'rejected' ? 'rejected' : i.status === 'approved' && approvals >= required ? 'approved' : 'pending'
    return { id: i.id, statement: i.statement, status, approvals, required }
  })
  const decisions = (artifact.decisions ?? []).map((d: Json) => ({ id: d.id, question: d.question, answered: d.chosen !== undefined && d.chosen !== null }))
  const reviewDone = intents.every((i) => i.status !== 'pending') && decisions.every((d: { answered: boolean }) => d.answered)

  const spec = parse(files.spec)
  const specCurrent = !!spec && reviewKey(spec.artifact) === reviewKey(artifact)
  const board: Board = { ...empty, intents, decisions }
  if (!reviewDone) return { ...board, phase: 'review-pending' }
  if (!specCurrent) return { ...board, phase: 'needs-spec' }

  const clauses: Json[] = spec.plan?.clauses ?? []
  board.beyondProof = (spec.plan?.beyondProof ?? []).map((b: Json) => `${b.intentId} (${b.reason})`)
  const rows = new Map<string, ProofRow>(
    clauses.map((c) => [c.id, { id: c.id, intents: c.intentIds ?? [], description: c.description ?? '', status: 'pending' }]),
  )

  // The last finished proof of this spec.
  const proof = parse(files.proof)
  const proofCurrent = !!proof && proof.report?.artifactHash === spec.artifact?.hash
  if (proofCurrent) for (const r of proof.report.clauses ?? []) setVerdict(rows, r)

  // A run under way (or just finished) overrides it, clause by clause, as its events arrive.
  const events: Json[] = (files.progress ?? '')
    .split('\n')
    .map((l) => parse(l))
    .filter(Boolean)
  const start = events.findIndex((e) => e.event === 'build')
  const run = start >= 0 ? events.slice(start) : []
  const started = run.find((e) => e.event === 'start')
  const result = run.find((e) => e.event === 'result')
  const sameSpec = !started || short(started.clauses) === short(clauses.map((c) => c.id))
  let phase: Phase = proofCurrent ? (proof.ok ? 'passed' : 'failed') : 'implementing'
  if (run.length && sameSpec) {
    if (!result) {
      phase = 'running'
      for (const row of rows.values()) row.status = 'running'
      board.runs = Math.max(0, ...run.filter((e) => e.event === 'progress').map((e) => Number(e.runs)))
    }
    for (const e of run) if (e.event === 'clause') setVerdict(rows, e)
    if (result?.error) {
      board.error = String(result.error)
      phase = 'failed'
    } else if (result) phase = result.ok ? 'passed' : 'failed'
  }
  return { ...board, phase, proofs: [...rows.values()] }
}

function setVerdict(rows: Map<string, ProofRow>, r: Json) {
  const row = rows.get(r.clauseId)
  if (!row) return
  row.status = r.verdict
  row.counterexample = r.counterexample ? `${short(r.counterexample.input)} → ${short(r.counterexample.output)}` : undefined
}

export const isPassing = (row: ProofRow) => PASSING.has(row.status)

/** One line: for the band above the prompt and the status line. */
export function summary(b: Board): string {
  const approved = b.intents.filter((i) => i.status === 'approved').length
  const answered = b.decisions.filter((d) => d.answered).length
  const passed = b.proofs.filter(isPassing).length
  const failed = b.proofs.filter((p) => ['violated', 'weak', 'vacuous'].includes(p.status)).length
  switch (b.phase) {
    case 'none':
    case 'no-proposal':
      return 'Lastro · no proposal yet'
    case 'review-pending':
      return `Lastro · review: ${approved}/${b.intents.length} intents approved · ${answered}/${b.decisions.length} decisions answered`
    case 'needs-spec':
      return `Lastro · ${approved}/${b.intents.length} intents approved · proofs not derived yet`
    case 'implementing':
      return `Lastro · ${b.proofs.length} proofs pending`
    case 'running':
      return `Lastro · proving… ${passed + failed}/${b.proofs.length} checked${b.runs ? ` · ${b.runs} runs` : ''}`
    case 'passed':
      return `Lastro · ${passed}/${b.proofs.length} proofs passed`
    case 'failed':
      return b.error ? 'Lastro · the proof run failed' : `Lastro · ${passed}/${b.proofs.length} proofs passed · ${failed} failed`
  }
}
