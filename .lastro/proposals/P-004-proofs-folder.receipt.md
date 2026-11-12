# Receipt: P-004 Keep proofs in .lastro/proof and derive them from the established use cases

Status: **applied** · submitted by claude · baseline `sha256:3010925e700e5dba481b172a4b3ec4b5147b1cf1331f7197fcb3dabc48ccdc6e` · approval `sha256:eaf5f6470109208232387444fbd3634fa016bf3c6d4fe0a04543e09a7d02b7af`

Proofs move out of the use case files into one file each under .lastro/proof/, and lastro derive --usecases derives proofs straight from the reviewed use cases, without a proposal.

## Use cases

### ✏️ UC-049 Keep proofs as regression tests (modify)

- Before: A proof that passed belongs to its use case, stored in the use case's file with its id, the intent and proposal it came from, its model, subject and check. prove(proposal) runs the proposal's pending proofs and the proofs of every use case except the ones the proposal supersedes, so a change that breaks any use case fails as a regression, on a touched use case or not; a superseded proof that still passes is noted, since the behavior may not have changed; failures go back to the implementer through feedback. lastro regress runs every use case's proofs alone, for example in CI. A use case with no proofs yet is reported as unproved rather than failing.
- After: A proof that passed belongs to its use case, stored in its own file, .lastro/proof/<proof-id>-<slug>.json, with its id, slug, name, description, use case, model, subject, check, status and verdict, and the intent and proposal it came from when it came from one; the workspace finds a use case's proofs by their useCase field. Only passed proofs run as regression tests; a pending or failed proof derived from a use case does not. prove(proposal) runs the proposal's pending proofs and the proofs of every use case except the ones the proposal supersedes, so a change that breaks any use case fails as a regression, on a touched use case or not; a superseded proof that still passes is noted, since the behavior may not have changed; failures go back to the implementer through feedback. lastro regress runs every use case's proofs alone, for example in CI. A use case with no proofs yet is reported as unproved rather than failing.
- **P-004.I-01** `approved` Every proof of a use case is stored in its own file, .lastro/proof/<proof-id>-<slug>.json, with its id, slug, name, description, use case, model, subject, check, status, verdict, and its intent and proposal when it has them.
- Gains: P-004.PF-01

### ✏️ UC-046 Store use cases and proposals (modify)

- Before: The workspace keeps one file per entity: each use case in .lastro/usecases/<id>-<slug>.json (id, slug, name, description, proofs, history) and each proposal in .lastro/proposals/<id>-<slug>.json, with its receipt next to it. Use cases are numbered UC-001, UC-002... and proposals P-001, P-002... with the next free number, never reused even after a removal; intents and proofs are numbered inside their proposal (P-007.I-03, P-007.PF-05) and keep that id when a proof moves to its use case. Slugs are kebab-case and stable: a rename changes only the name. intents.json, spec.json and proof.json are no longer written.
- After: The workspace keeps one file per entity: each use case in .lastro/usecases/<id>-<slug>.json (id, slug, name, description, history), each proof of a use case in .lastro/proof/<proof-id>-<slug>.json and each proposal in .lastro/proposals/<id>-<slug>.json, with its receipt next to it. Use cases are numbered UC-001, UC-002... and proposals P-001, P-002... with the next free number, never reused even after a removal; intents and proofs are numbered inside their proposal (P-007.I-03, P-007.PF-05) and keep that id when a proof moves to its use case; proofs derived straight from a use case are numbered inside it (UC-003.PF-01). Slugs are kebab-case and stable: a rename changes only the name. intents.json, spec.json and proof.json are no longer written. A use case file that still carries a proofs list is read as before, and its proofs move to .lastro/proof/ the next time the workspace writes that use case.
- **P-004.I-02** `approved` Use case files are written without a proofs list; the workspace finds a use case's proofs by their useCase field.
- **P-004.I-03** `approved` A use case file that still has a proofs list is read as before; its proofs move to .lastro/proof/ the next time that use case is written.
- Gains: P-004.PF-02, P-004.PF-03

### ✏️ UC-050 Apply a proposal (modify)

- Before: apply(proposal), in the library and as lastro apply <proposal>, refuses unless the proposal is approved, unchanged since approval, has no open decision, and every proof, new and regression, passes. It then writes each resulting use case (adds the use case file, replaces the description and name, or deletes the file of a removed use case), moves each passing proof into its use case as a regression test, removes the superseded proofs, appends the proposal id to each touched use case's history, records the new baseline hash in the receipt and sets the proposal's status to applied.
- After: apply(proposal), in the library and as lastro apply <proposal>, refuses unless the proposal is approved, unchanged since approval, has no open decision, and every proof, new and regression, passes. It then writes each resulting use case (adds the use case file, replaces the description and name, or deletes the file of a removed use case), writes each passing proof to .lastro/proof/ as a regression test of its use case, deletes the files of the superseded proofs, appends the proposal id to each touched use case's history, records the new baseline hash in the receipt and sets the proposal's status to applied.
- **P-004.I-04** `approved` apply writes each passing proof of the proposal to .lastro/proof/ and deletes the files of the proofs it supersedes.
- Gains: P-004.PF-04

### ✏️ UC-036 Share one workspace across CLIs (modify)

- Before: The TypeScript, .NET and Go CLIs read and write the same .lastro/ files (config.json, usecases/, proposals/ and llm/), so one can propose, another review, and a third derive, prove and apply. A proposal approved by one SDK has a valid approval hash for the others, and proofs and baselines hash the same in all three.
- After: The TypeScript, .NET and Go CLIs read and write the same .lastro/ files (config.json, usecases/, proof/, proposals/ and llm/), so one can propose, another review, and a third derive, prove and apply. A proposal approved by one SDK has a valid approval hash for the others, and proofs and baselines hash the same in all three.
- **P-004.I-05** `approved` The TypeScript, Go and .NET workspaces read and write the same .lastro/proof/ files.
- Gains: P-004.PF-05

### ➕ UC-054 Derive proofs from the established use cases (add)

- After: lastro derive --usecases derives proofs straight from the established use cases, with no proposal and no intents: by default for every use case that has no proofs, or for the ids it is given (--usecases UC-003,UC-016). The derive step reads each use case's description, the models and subjects declared in .lastro/config.json and the proofs other use cases already have, and writes pending proofs numbered inside their use case (UC-003.PF-01) to .lastro/proof/. A use case the models cannot express gets no proof and is listed as unprovable with the reason. A use case that already has proofs is refused: its proofs change only through a proposal. lastro prove --usecases runs the pending proofs of the use cases together with every passed proof. A pending proof that passes becomes passed and from then on runs as a regression test, in prove and in lastro regress. A pending proof that fails stays failed and is reported with the note that the code is the reference: either the description is wrong, and a proposal fixes it, or the code has a bug the user decides about; it never goes back to the code as feedback.
- **P-004.I-06** `approved` lastro derive --usecases derives proofs from use case descriptions, with no proposal and no intents, for every use case without proofs or for the ids given.
- **P-004.I-07** `approved` Proofs derived from a use case are pending and numbered inside it, UC-003.PF-01, reading the description, the declared models and subjects and the other use cases' proofs.
- **P-004.I-08** `approved` A use case the declared models cannot express gets no proof and is listed as unprovable with the reason.
- **P-004.I-09** `approved` derive --usecases refuses a use case that already has proofs; those change only through a proposal.
- **P-004.I-10** `approved` lastro prove --usecases runs the pending use case proofs with every passed proof; a pending proof that passes becomes a regression test in prove and regress.
- **P-004.I-11** `approved` A pending use case proof that fails stays failed and is reported with the note that the code is the reference; it never goes back to the code as feedback.
- Gains: P-004.PF-06, P-004.PF-07, P-004.PF-08, P-004.PF-09, P-004.PF-10, P-004.PF-11

## Proofs

- **P-004.PF-01** ✅ proved — A use case's proof is read from its own complete file in .lastro/proof/. (P-004.I-01, proof_files/proof_files)
  - `requires ¬in.legacy; ensures (out.proofFiles = 1) ∧ out.fileComplete ∧ (out.loaded = 1)`
- **P-004.PF-02** ✅ proved — A use case file written by the workspace has no proofs list. (P-004.I-02, proof_files/proof_files)
  - `requires ¬in.legacy ∨ in.rewrite; ensures ¬out.embedded`
- **P-004.PF-03** ✅ proved — A use case file with a proofs list is read with its proof, which moves to .lastro/proof/ when the use case is written. (P-004.I-03, proof_files/proof_files)
  - `requires in.legacy; ensures (out.loaded = 1) ∧ (in.rewrite ⇒ ((out.proofFiles = 1) ∧ ¬out.embedded))`
- **P-004.PF-04** ✅ proved — Apply writes the new passing proof's file and deletes the superseded proof's file, and only it. (P-004.I-04, apply_proof_files/apply_proof_files)
  - `ensures out.applied ∧ out.newFile ∧ (out.oldFile ⇔ ¬in.supersede)`
- **P-004.PF-05** 🟡 evidenced — Proof files in the shared .lastro/proof/ layout are read as the use case's proofs. (P-004.I-05, proof_files/proof_files)
  - `input {"legacy":false,"rewrite":false}; expect (out.loaded = 1) ∧ (out.proofFiles = 1)`
- **P-004.PF-06** ✅ proved — Without ids, every use case with no passed proof that the models express gets a proof, with no proposal. (P-004.I-06, usecase_derive/usecase_derive)
  - `requires ¬in.explicit; ensures ¬out.needsProposal ∧ ¬out.refused ∧ (out.derived ⇔ (¬in.hasPassed ∧ in.expressible))`
- **P-004.PF-07** ✅ proved — A derived proof is numbered after the use case's existing proofs (PF-01, or PF-02 after a pending one). (P-004.I-07, usecase_derive/usecase_derive)
  - `ensures out.derived ⇒ (out.number = (if in.hasPending then 2 else 1))`
- **P-004.PF-08** ✅ proved — A target use case the models cannot express gets no proof and is listed as unprovable. (P-004.I-08, usecase_derive/usecase_derive)
  - `ensures (out.unprovable ⇔ ((¬in.hasPassed ∧ (¬in.explicit ∨ in.listed)) ∧ ¬in.expressible)) ∧ (out.unprovable ⇒ ¬out.derived)`
- **P-004.PF-09** ✅ proved — Naming a use case that has passed proofs is refused and derives nothing. (P-004.I-09, usecase_derive/usecase_derive)
  - `ensures (out.refused ⇔ (in.explicit ∧ in.listed ∧ in.hasPassed)) ∧ (out.refused ⇒ ¬out.derived)`
- **P-004.PF-10** ✅ proved — When the code satisfies it, a use case proof ends passed and runs in regress. (P-004.I-10, usecase_prove/usecase_prove)
  - `requires in.codeOk; ensures (out.status = "passed") ∧ out.regression`
- **P-004.PF-11** ✅ proved — A pending or failed use case proof that fails ends failed, with the note, without feedback to the code. (P-004.I-11, usecase_prove/usecase_prove)
  - `requires ¬in.codeOk ∧ (in.status ≠ "passed"); ensures (out.status = "failed") ∧ out.note ∧ ¬out.feedback ∧ ¬out.ok ∧ ¬out.regression`

## Regression

26 use-case proof(s) ran; 0 failed. Engines agree: yes.

- Unproved use cases (no proofs yet): UC-003, UC-004, UC-006, UC-008, UC-011, UC-012, UC-014, UC-015, UC-016, UC-017, UC-018, UC-019, UC-020, UC-021, UC-022, UC-023, UC-024, UC-025, UC-026, UC-027, UC-028, UC-029, UC-030, UC-031, UC-032, UC-033, UC-034, UC-035, UC-037, UC-038, UC-040, UC-041, UC-042, UC-043, UC-044, UC-045

## Decisions

- **D1** Must a person approve each proof derived from a use case before it counts as a regression test, or is passing against the code enough? → Passing is enough (iury)

## Not promised

- Proofs a proposal derived stay inside the proposal file until apply; only use case proofs live in .lastro/proof/.
- derive --usecases writes no models or subjects: a use case it lists as unprovable needs a model added to .lastro/config.json first.
- The usage of the other CLI commands (UC-030) changes.

