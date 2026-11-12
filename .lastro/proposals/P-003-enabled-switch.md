# Turn the agent plugin on and off with `enabled`

The plugin's switch in `.lastro/plugin.json` becomes `"enabled": true | false` instead of `"gate": "on" | "off"`.
With `enabled` true (the default when the key is absent), every hook runs: the gate holds code edits until a
proposal is approved and its proofs are derived, Stop proves, and the board is shown. With `enabled` false, every
hook does nothing: no gate, no proofs on Stop, no board. A file written before the change keeps its meaning: a
`"gate": "off"` with no `enabled` key reads as `enabled` false, and `gate` is otherwise ignored.

`lastro init` no longer requires `--model`. Without it, init writes a workspace config with no models and no
subjects and the review policy (`--approvals`, `--submitter-cannot-approve`), so a repository can start writing and
reviewing proposals, or bootstrap its use cases, before any model exists. `--subject` without `--model` is refused
with exit 2. The TypeScript, Go and .NET CLIs behave the same way and print the same result.
