# Documentation site

This directory holds the source for the project documentation site, built with
[Hugo](https://gohugo.io/) and the [Hextra](https://imfing.github.io/hextra/)
theme. The site is published to GitHub Pages by
[`.github/workflows/docs.yml`](../.github/workflows/docs.yml) on every push to
`main` that touches `docs/`.

Published site: <https://ymotongpoo.github.io/xk6-otel-gen/>

## Layout

- `hugo.yaml` — site configuration.
- `content/` — page sources (Markdown with Hugo front matter and Hextra
  shortcodes). Excluded from markdownlint; the rendered site is the source of
  truth.
- `go.mod` / `go.sum` — Hugo Modules manifest for the Hextra theme. This is a
  nested Go module and is **not** part of the main Go module (`go build ./...`
  at the repo root does not descend into it).

## Build locally

Requires the **extended** edition of Hugo (Hextra needs it).

```bash
cd docs
hugo mod get -u          # fetch / update the Hextra theme module
hugo server              # live preview at http://localhost:1313/xk6-otel-gen/
hugo --minify            # production build into docs/public/ (git-ignored)
```

## Add a page

1. Create a Markdown file under `content/<section>/`.
2. Add front matter with at least `title` and a `weight` (controls sidebar
   order within the section).
3. Link between pages with `{{</* relref "/section/page" */>}}` so links stay
   valid under the site's base URL.
