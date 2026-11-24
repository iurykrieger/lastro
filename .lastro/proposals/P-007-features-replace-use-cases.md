# Features replace use cases

The application is its list of features. "Feature" is the word the business already uses for what a product does
for its users, so Lastro uses it everywhere it said "use case": in the files, the ids, the CLI, the SDKs' public
API, the prompts, the review app, the plugin, the board, the receipt and the docs. Only the name changes: a feature
has the same fields, the same history and the same proofs a use case had, and every rule that held for use cases
holds for features.

## Files and ids

Each feature lives in `.lastro/features/<id>-<slug>.json` and is numbered F-001, F-002... with the next free
number, never reused. Proofs derived straight from a feature are numbered inside it (F-003.PF-01); proofs that came
from a proposal keep their proposal id. A proof file names its feature in a `feature` field. A proposal's intents
name the feature they add, modify or remove, and its baseline is the hash over the canonical features.

A workspace that still has `.lastro/usecases/` is read as before: each use case is read as the feature with the same
number (UC-003 becomes F-003) and the proof ids that start with its old id are read with the new one. The next time
the workspace writes, it moves the files to `.lastro/features/`, renames the ids and rewrites every proof and
proposal that names them, and records each old id in the feature's history so a link to UC-003 still resolves.

## Commands and API

`lastro derive --features`, `lastro prove --features` and every other option that took use cases take features.
The old `--usecases` options keep working and print a notice that they were renamed. In the TypeScript, Go and .NET
SDKs every public type, function and field that said use case says feature, and the old names stay as deprecated
aliases for one release. The extraction and derivation prompts and their schemas say feature, and the .NET and Go
prompts are generated from the TypeScript ones as before.

## What people see

The review app, the plugin's review, the progress board and the receipt say feature and show F- ids. The docs
explain features as what the product does for its users, in the user's words, and keep one note saying they were
called use cases before.
