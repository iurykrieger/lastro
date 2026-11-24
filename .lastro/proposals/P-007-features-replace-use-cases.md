# Features replace use cases

The application is its list of features. "Feature" is the word the business already uses for what a product does
for its users, so Lastro uses it everywhere it said "use case": in the files, the ids, the CLI, the SDKs' public
API, the prompts, the review app, the plugin, the board, the receipt and the docs. A feature has the same fields, the
same history and the same proofs a use case had, and every rule that held for use cases holds for features.

## What a feature is

A feature is broader than a use case and is written around the functionality itself: what the product lets its users
do, in the business's words, with the complete behavior of that functionality as the code implements it. The
extraction and survey prompts write and group features that way.

An intent is the intention to change the application: it modifies one or more existing features, adds a new one or
removes one. A modify intent names every feature it changes, and each of those features' new description combines all
the intents that change it.

## Files and ids

Each feature lives in `.lastro/features/<id>-<slug>.json` and is numbered F-001, F-002... with the next free
number, never reused. Proofs derived straight from a feature are numbered inside it (F-003.PF-01); proofs that came
from a proposal keep their proposal id. A proof file names its feature in a `feature` field. A proposal's intents
name the features they add, modify or remove, and its baseline is the hash over the canonical features.

This repository's own use cases are migrated once, as part of this change: each use case becomes the feature with
the same number (UC-003 becomes F-003), every proof and proposal that names it is rewritten with the new id, and
`.lastro/usecases/` is removed. Lastro keeps no reader for the old layout and no old names: the framework has no
users yet.

## Commands and API

`lastro derive --features`, `lastro prove --features` and every other option that took use cases take features;
`--usecases` no longer exists. In the TypeScript, Go and .NET SDKs every public type, function and field that said
use case says feature, with no aliases. The extraction and derivation prompts and their schemas say feature, and
the .NET and Go prompts are generated from the TypeScript ones as before.

## What people see

The review app, the plugin's review, the progress board and the receipt say feature and show F- ids. The docs
explain features as what the product does for its users, in the user's words, and nothing in the repository says use
case any more.
