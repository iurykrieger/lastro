export type IntentRow = {
  id: string
  statement: string
  /** approved | pending | rejected, with approvals counted against the policy. */
  status: 'approved' | 'pending' | 'rejected'
  approvals: number
  required: number
}

export type ProofRow = {
  id: string
  intents: string[]
  description: string
  /** pending: not run on this spec · running: in the run under way · else the clause's verdict. */
  status: 'pending' | 'running' | 'proved' | 'proved-under-assumptions' | 'evidenced' | 'violated' | 'weak' | 'vacuous'
  counterexample?: string
}

export type Phase = 'none' | 'no-proposal' | 'review-pending' | 'needs-spec' | 'implementing' | 'running' | 'failed' | 'passed'

export type Board = {
  model: string
  phase: Phase
  intents: IntentRow[]
  decisions: { id: string; question: string; answered: boolean }[]
  proofs: ProofRow[]
  beyondProof: string[]
  /** Runs of the code under proof in the run under way (from the `progress` events). */
  runs: number
  error?: string
}

declare module 'claude-code' {
  interface PluginState {
    'lastro-board': { board: Shaped<Board | null>; isHidden: boolean }
  }
}
