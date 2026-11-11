# Lastro docs

The developer documentation site: Vite + React + shadcn/ui (new-york, zinc, the sandbox's tokens), hash routing,
no backend. Any static host serves `dist/`.

Published to GitHub Pages at <https://iurykrieger.github.io/lastro/> by `.github/workflows/docs.yml` on every push to
`main` that touches the docs (or by hand from the Actions tab). The site uses relative asset paths (`base: "./"`), so
it works under the `/lastro/` path without configuration. One-time setup: Settings → Pages → Source: GitHub Actions.

```sh
npm run docs                              # dev server on http://localhost:5174 (from the repo root)
npm run build -w @lastro/docs             # static site in apps/docs/dist
```

| Path | What it is |
|---|---|
| `src/content/nav.ts` | The table of contents: sidebar, search and previous/next links |
| `src/pages/*.tsx` | The pages, written with the blocks in `src/components/doc.tsx` |
| `src/content/outputs.ts` | Real CLI output of the quickstart |
| `examples/shipping` | The quickstart's files, and the same pipeline as TS, Go and .NET programs |
| `test/` | Runs the quickstart through the real `lastro` CLI and the TS library example |
| `e2e/` | Playwright UI tests (every page, navigation, search, SDK tabs, theme, copy, mobile menu) |

## Keeping the docs true

Code samples are imported from `examples/` (`?raw`), not pasted, and the tests run them:

```sh
npm run wasm && npm run build -w lastro
npm run test:examples -w @lastro/docs     # the quickstart + library.ts, through the real SDK
(cd examples/shipping/go-library && go test ./...)
(cd examples/shipping/dotnet && dotnet run -- --replay)
npm run test:e2e -w @lastro/docs          # builds, serves and tests the site in Chromium (desktop and mobile)
```

When the CLI output changes, regenerate `src/content/outputs.ts` from a fresh run of the quickstart.

A new page needs an entry in `src/content/nav.ts` and in `src/pages/index.tsx`; the UI tests visit every page in the nav.
