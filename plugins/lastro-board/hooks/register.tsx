// lastro-board: a Claude Code mod that shows where a Lastro change stands, beside the chat.
//
// It reads the workspace the `lastro` CLI and the Lastro plugin keep in .lastro/ once a second: the intents and their
// approvals, the decisions, the proofs derived from them, and the live events of the proof run under way
// (.lastro/plugin/progress.jsonl, written by the plugin's Stop hook with `lastro prove --stream`). It decides nothing.
import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Board, ProofRow } from '../types'
import { isPassing, readBoard, summary } from './board'
import type { Files } from './board'

const PANE = 'lastro-board'
// Change the tag when Board's fields change: it declines what an earlier version stored.
const board = atom({ plugin: 'lastro-board', key: 'board' } as const, null, { shape: 'board-1' })
const isHidden = atom({ plugin: 'lastro-board', key: 'isHidden' } as const, false)

const MARK: Record<ProofRow['status'], string> = {
  pending: '○',
  running: '…',
  proved: '✓',
  'proved-under-assumptions': '✓',
  evidenced: '≈',
  violated: '✗',
  weak: '!',
  vacuous: '!',
}

const COLOR: Partial<Record<ProofRow['status'], 'success' | 'error' | 'warning' | 'subtle'>> = {
  proved: 'success',
  'proved-under-assumptions': 'success',
  evidenced: 'warning',
  violated: 'error',
  weak: 'warning',
  vacuous: 'warning',
  pending: 'subtle',
  running: 'subtle',
}

/** A workspace file's text, or undefined when it is missing. */
async function readFile($: EngineInterface, dir: string, name: string): Promise<string | undefined> {
  try {
    return await $.fs.read(`${dir}/${name}`)
  } catch {
    return undefined
  }
}

/** What changed enough to say out loud. */
export function toastFor(before: Board | null, after: Board | null): string | undefined {
  if (!after || !before) return undefined
  if (before.phase === 'review-pending' && after.phase !== 'review-pending' && after.intents.length)
    return `Lastro: all ${after.intents.length} intents approved`
  if (before.phase === 'running' && (after.phase === 'passed' || after.phase === 'failed')) {
    if (after.error) return 'Lastro: the proof run failed'
    const passed = after.proofs.filter(isPassing).length
    return after.phase === 'passed'
      ? `Lastro: all ${after.proofs.length} proofs passed`
      : `Lastro: ${after.proofs.length - passed} of ${after.proofs.length} proofs failed`
  }
  return undefined
}

export const register: Register = on => {
  let dir = '.lastro'

  on('session.start', async ($, e, next) => {
    await $.command.register({ name: 'lastro-board', description: 'Show the Lastro board: approvals and proofs, live' })
    let started = false
    try {
      const plugin = JSON.parse(await $.fs.read('.lastro/plugin.json'))
      if (typeof plugin.dir === 'string') dir = plugin.dir
      started = true
    } catch {
      started = (await readFile($, dir, 'config.json')) !== undefined
    }

    const poll = async () => {
      const files: Files = {
        config: await readFile($, dir, 'config.json'),
        intents: await readFile($, dir, 'intents.json'),
        spec: await readFile($, dir, 'spec.json'),
        proof: await readFile($, dir, 'proof.json'),
        progress: await readFile($, dir, 'plugin/progress.jsonl'),
      }
      const fresh = readBoard(files)
      const before = await read($, board)
      if (JSON.stringify(before) === JSON.stringify(fresh)) return
      await update($, board, () => fresh)
      const toast = toastFor(before, fresh)
      if (toast) $.ui.toast(toast)
      $.ui.status(fresh ? summary(fresh) : undefined)
    }

    if (started) {
      await poll()
      $.clock.every(1000, () => void poll())
      void $.ui.open({ id: PANE, title: 'Lastro' })
    }
    return next(e)
  })

  on('command.run', { command: 'lastro-board' }, async $ => {
    await update($, isHidden, () => false)
    await $.ui.open({ id: PANE, title: 'Lastro' })
    return { text: 'Lastro board opened.' }
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Text } = $.ui.resolve(e)
    const b = await read($, board)
    if (!b) return <Text dimColor>No Lastro workspace in this folder.</Text>

    const approved = b.intents.filter(i => i.status === 'approved').length
    return (
      <Box flexDirection="column">
        <Text bold>{summary(b)}</Text>
        {b.model !== '' && <Text dimColor>{b.model}</Text>}

        <Text> </Text>
        <Text bold>
          Review · {approved}/{b.intents.length} intents approved
        </Text>
        {b.decisions.map(d => (
          <Text color={d.answered ? 'success' : 'warning'}>
            {d.answered ? '✓' : '?'} {d.id} {d.question}
          </Text>
        ))}
        {b.intents.map(i => (
          <Text color={i.status === 'approved' ? 'success' : i.status === 'rejected' ? 'error' : undefined}>
            {i.status === 'approved' ? '✓' : i.status === 'rejected' ? '✗' : '○'} {i.id}
            {i.required > 1 ? ` (${i.approvals}/${i.required})` : ''} {i.statement}
          </Text>
        ))}

        {b.proofs.length > 0 && (
          <Box flexDirection="column">
            <Text> </Text>
            <Text bold>
              Proofs · {b.proofs.filter(isPassing).length}/{b.proofs.length} passed
              {b.phase === 'running' && b.runs > 0 ? ` · ${b.runs} runs` : ''}
            </Text>
            {b.proofs.map(p => (
              <Box flexDirection="column">
                <Text color={COLOR[p.status]}>
                  {MARK[p.status]} {p.id} {p.description} [{p.intents.join(', ')}]
                </Text>
                {p.counterexample !== undefined && <Text dimColor>    counterexample {p.counterexample}</Text>}
              </Box>
            ))}
            {b.beyondProof.map(x => (
              <Text dimColor>– {x} beyond proof: needs a human sign-off</Text>
            ))}
          </Box>
        )}
        {b.error !== undefined && <Text color="error">{b.error}</Text>}
      </Box>
    )
  })

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const b = await read($, board)
    if (e.props.hasSurvey || !b || b.phase === 'none' || (await read($, isHidden))) return next(e)
    const { Box, Button, Text } = $.ui.resolve(e)
    const color = b.phase === 'passed' ? 'success' : b.phase === 'failed' ? 'error' : b.phase === 'review-pending' ? 'warning' : undefined
    return (
      <Box>
        <Text color={color}>{summary(b)} </Text>
        <Button key="open" label="Open" onPress={() => void $.ui.open({ id: PANE, title: 'Lastro' })} />
        <Button key="hide" label="Hide" onPress={() => update($, isHidden, () => true)} />
      </Box>
    )
  })
}
