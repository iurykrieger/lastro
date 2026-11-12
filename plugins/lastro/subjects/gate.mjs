// The plugin's edit gate as a subject Lastro can prove (model .lastro/models/edit_gate.json): the real phaseOf and
// allowsCodeEdits from scripts/lib.mjs, on a proposal built from the model's inputs.
import { allowsCodeEdits, phaseOf } from "../scripts/lib.mjs";

export function editGate({ approved, proofs, run, stale }) {
  const proposal = {
    hash: approved ? "sha256:approved" : undefined,
    proofs: proofs ? [{ id: "P-001.PF-01" }] : [],
    lastRun: run === "none" ? undefined : { ok: run === "ok" },
  };
  const phase = phaseOf(proposal, stale);
  return { phase, allowEdit: allowsCodeEdits(phase) };
}
