# Review intents first, and see the feature map and the code before and after

The review app is where a team reviews a proposal before any code exists, and where it comes back to see what the
change did. It already lets reviewers approve, amend and reject intents; this proposal makes that review the app's
first screen, adds a map of the features and how they depend on each other, and shows what the implementation did
to the code: the cyclomatic complexity of its functions and the dependencies between its modules, before and after.
None of this judges the code: proofs stay the only verdicts, and the new views are evidence for the reviewer.

## Reviewing intents

Every open proposal has an Intents tab, the one the app opens first while the proposal has intents to review. It
lists the proposal's intents in order, each with the feature it adds, modifies or removes, the feature's
description before and after as a word diff, the quote it came from, the proofs it replaces and its threads, and
the reviewer approves, amends or rejects each one there, as in the feature view. It filters to the intents that are
not settled or that wait for the reviewer, and the keyboard moves between intents (j and k) and opens approve,
amend and reject on the selected one (a, m and r).

A reviewer can approve every intent that waits for them at once; the app first says how many intents that is and
which proofs they replace, and approves only after the reviewer confirms. An amended or rejected intent is never
approved that way.

The header shows how many intents wait for the reviewer across every open proposal, and links to an inbox that
lists them grouped by proposal, oldest proposal first.

## The feature map

A Map page draws every feature as a node and every dependency as an arrow from the feature to the one it depends
on, from the features' surfaces and dependencies (proposal P-006). Features are laid out in columns by how deep
their dependencies go, in a deterministic order, so the same workspace always draws the same map. Each node shows
the feature's name, its surfaces and the state of its proofs: every proof passed, some only evidenced, one failing,
or no proofs. A feature with no surface is drawn as not reachable by a driver. A dependency on a feature that does
not exist or was removed, and every dependency cycle, are drawn in red and listed above the map with the features
they involve.

The map can show one proposal: the features it adds, modifies and removes are colored by change, the features that
depend on them, directly or not, are marked as affected, and the others fade. A proposal's page links to its map.
Clicking a feature opens its page, which lists what it depends on and what depends on it. The map can be filtered
by text.

## The code before and after

Two more snapshot kinds, taken like the other snapshots of a proposal (before when it is extracted, on the current
code; after when it is proved, on the new code) but for the repository's code rather than one feature, so they live
in `.lastro/snapshots/<proposal>/code/before|after/`. They are never committed to git and the proposal records the
SHA-256 of each one.

- complexity: the cyclomatic complexity of every function in the repository's source files: one plus its decision
  points (each if, loop, case, catch, conditional expression and short-circuit operator). TypeScript and JavaScript
  are read with a parser; Rust, Go and C# with a scanner that skips comments and strings. Files in other languages
  are listed as not measured.
- imports: the dependency graph between the repository's source files: an arrow for every import that resolves to
  another file of the repository (relative and workspace imports in TypeScript and JavaScript, `mod` and `use crate`
  in Rust, packages of the same module in Go). Files whose imports cannot be resolved are listed as not mapped.

Which files count follows `.gitignore` and the `code` section of `.lastro/config.json` (paths to include and to
exclude), and the same files give the same snapshot on every machine.

The comparison of a complexity pair lists every function added, removed or whose complexity changed, with its
file, before, after and difference, and each file's total and highest complexity before and after. A function above
the threshold (`code.complexityThreshold`, 10 by default) is marked, and one that crossed it in this proposal is
flagged. The comparison of an imports pair lists the arrows added and removed, the files added and removed, how
many files each changed file imports and is imported by, before and after, and every import cycle the change
introduced, by its files. Neither comparison blocks a proposal. The TypeScript, Go and .NET SDKs take and compare
these snapshots with identical results, like the other snapshot kinds.

## Seeing it in the app

A proposal has a Code tab. It shows the complexity comparison as a table of changed files that opens into their
functions, sortable by difference, and the imports comparison as a graph of the changed files and the files next to
them, with added arrows in green, removed ones dashed in red and new cycles highlighted. Until the proposal is
proved, the after side is taken from the working tree each time the page loads and is labeled as not yet
snapshotted, so reviewers can follow the implementation while it is written.
