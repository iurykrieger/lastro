export type IntentRow = {
  /** The id inside its proposal: "I-03". */
  id: string
  /** The use case it adds, changes or removes. */
  useCase: string
  statement: string
  /** approved | pending | rejected, with approvals counted against the proposal's policy. */
  status: 'approved' | 'pending' | 'rejected'
  approvals: number
  required: number
  /** Existing proofs this intent breaks on purpose. */
  replaces: string[]
}

export type ProofRow = {
  id: string
  intents: string[]
  description: string
  /** pending: not run yet · running: in the run under way · else the proof's verdict. */
  status: 'pending' | 'running' | 'proved' | 'evidenced' | 'violated' | 'weak' | 'vacuous' | 'not-run'
  counterexample?: string
}

export type Phase = 'none' | 'no-proposal' | 'review-pending' | 'needs-proofs' | 'implementing' | 'running' | 'failed' | 'passed'

export type Board = {
  /** "P-001 Shipping fee at checkout". */
  title: string
  phase: Phase
  intents: IntentRow[]
  decisions: { id: string; question: string; answered: boolean }[]
  proofs: ProofRow[]
  beyondProof: string[]
  /** Use-case proofs the last run broke. */
  regressions: string[]
  /** Runs of the code under proof in the run under way (from the `progress` events). */
  runs: number
  error?: string
}

declare module 'claude-code' {
  interface PluginState {
    'lastro-board': { board: Shaped<Board | null>; isHidden: boolean }
  }
}
