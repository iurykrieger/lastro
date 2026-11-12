# Keep proofs in .lastro/proof and derive them from the established use cases

## Where proofs live

Every proof that belongs to a use case lives in its own file, `.lastro/proof/<proof-id>-<slug>.json`, with its id,
slug, name, description, use case, model, subject, check, status and verdict, and the intent and proposal it came
from when it came from one. Use case files no longer carry the proofs: the workspace finds a use case's proofs by
their `useCase` field. Apply writes each passing proof of the proposal to `.lastro/proof/` and deletes the files of
the proofs the proposal supersedes. A use case file that still carries a `proofs` list is read as before, and its
proofs move to `.lastro/proof/` the next time the workspace writes that use case. The TypeScript, Go and .NET
workspaces read and write the same files.

## Proofs from the established use cases

A use case already describes, and people already reviewed, what the code does. `lastro derive --usecases` derives
proofs straight from those descriptions, with no proposal and no intents: by default for every use case that has no
proofs, or for the ids it is given (`--usecases UC-003,UC-016`). The derive step reads each use case's description,
the models and subjects declared in `.lastro/config.json` and the proofs other use cases already have, and writes
pending proofs numbered inside their use case (`UC-003.PF-01`). A use case the models cannot express gets no proof
and is listed as unprovable with the reason, so a model can be added for it. A use case that already has proofs is
refused: its proofs change only through a proposal.

`lastro prove --usecases` runs the pending proofs of the use cases together with every passed proof. A pending
proof that passes becomes passed and from then on runs as a regression test, in `prove` and in `lastro regress`. A
pending proof that fails stays failed and is reported with the note that the code is the reference: either the
description is wrong, and a proposal fixes it, or the code has a bug the user decides about. It never goes back to
the code as feedback.
