---
name: docs-site-policy
description: Documentation site for xk6-otel-gen is Hugo + Hextra under docs/, deployed to GitHub Pages via GitHub Actions on main
metadata:
  type: project
---

The **xk6-otel-gen** project publishes its documentation as a Hugo + Hextra site,
deployed to GitHub Pages.

**Key decisions:**
- **Source location:** `docs/` (Hugo site root). Page sources live in
  `docs/content/`. Built HTML (`docs/public/`) is git-ignored — never committed.
- **Publishing method:** GitHub Pages **"GitHub Actions" source** (NOT a
  `gh-pages` branch, NOT the `/docs` Jekyll auto-build). The build+deploy
  workflow is `.github/workflows/docs.yml`.
- **Theme:** Hextra, imported as a **Hugo Module** (`docs/go.mod` requires
  `github.com/imfing/hextra`). This is a nested Go module, isolated from the main
  Go module — `go build ./...` at the repo root does not descend into it.
- **Trigger:** deploy runs only on push to `main` with `paths: docs/**` (plus
  `workflow_dispatch`). Consistent with the repo policy that CI runs only on
  `main`; the site updates when `dev` → `main` is merged. See [[project-branching-ci]].
- **README:** kept thin — overview, badges, minimal Quick Start, and links into
  the docs site. Detailed docs live only in `docs/content/` to avoid drift.
- **markdownlint:** `docs/content/**` is excluded in `.markdownlint-cli2.yaml`
  (Hugo front matter / Hextra shortcodes are not CommonMark). `docs/README.md`
  is still linted.
- **Manual one-time setup:** repo Settings → Pages → Source must be set to
  "GitHub Actions" for deployment to work; this cannot be done via code.
- **baseURL:** `https://ymotongpoo.github.io/xk6-otel-gen/` (project page,
  subpath — internal links use `{{< relref >}}`).
- **i18n (multilingual):** English + Japanese, directory-based separation.
  `content/en/` (default, served at `/`) and `content/ja/` (served at `/ja/`)
  are parallel trees with identical file paths and `weight` values. Configured
  via `languages.{en,ja}` in `hugo.yaml` with per-language `contentDir`, `menu`,
  and `editURL`; use Hugo's new `label`/`locale` keys (not the deprecated
  `languageName`/`languageCode`). Cross-page links use `{{< relref "/path" >}}`
  with no language prefix so they resolve within the current language. Hextra
  shows the language switcher automatically. Add a page to BOTH trees and keep
  `weight` identical so the two navs stay aligned.

Pinned versions chosen at setup: Hugo extended `0.163.1`, Hextra `v0.12.3`.

Follow-ups deferred (out of initial scope): PR preview build job, custom domain,
Hextra version-bump automation (Dependabot does not support Hugo Modules).
