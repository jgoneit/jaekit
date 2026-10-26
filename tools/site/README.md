# Site source

The [documentation site](https://jgoneit.github.io/jaekit/) is built from the repository: the home page from `home.html` and `static/`, and every documentation page from a tracked Markdown file directly under `guides/` (`X.md` in Korean, `X.en.md` in English) or `contracts/`. The release version comes from `metadata.version` in `.claude-plugin/marketplace.json`. Nothing in this folder copies those documents, and the built site is never committed: the [site workflow](../../.github/workflows/pages.yml) builds and publishes it from `main`.

## Build it locally

The build runs on macOS and Linux with Git and Python 3.10 or later (the pinned Markdown parser needs 3.10). From the repository root:

```bash
python3 tools/site/build.py
```

The site is written to `tools/site/_site/`, which Git ignores. To look at it, serve that folder and open the address it prints:

```bash
python3 -m http.server 8000 --directory tools/site/_site
```

Options: `--out DIR` writes elsewhere, `--base /jaekit/` sets the path the not-found page uses for its links, and `--cache DIR` moves the download cache. The output folder is replaced only when it is new, empty or an earlier build, which leaves a `.jaekit-site` file in it; any other folder stops the build.

The first build downloads the pinned third-party files listed in [vendor.json](vendor.json) into `tools/site/.cache/`, also ignored, and checks each one against its recorded digest: the Markdown parser (markdown-it-py and mdurl, used only while building), the Geist, Geist Mono and Noto Sans KR fonts, and the mermaid renderer. The published site serves these files itself; a page makes no request outside the site. Their license notices are published as `assets/licenses.txt`.

Exit codes: 0 built; 1 an input cannot be used, such as a link to a missing file or heading or a missing release version, and nothing is written; 2 a usage error; 3 a pinned download could not be fetched or verified; 4 a tool the build needs cannot run, such as an older Python, or Git cannot read the repository, or the build stopped unexpectedly. Nothing is written unless the build succeeds.

## Checks

The site's checks are Go tests in `tools/goal-checks/pages-site/`, behind the `pagessite` build tag, so the repository's default validation does not run them. They build the site, read it, and drive headless Chrome for the page behavior. The tests the site workflow runs are:

```bash
go test -tags pagessite -count=1 -run "TestPagesSiteAC(2|3|4|5|6|7|8|9|10|11)_" ./tools/goal-checks/pages-site
```

The other tests read the published site and GitHub's settings, and one runs the whole repository validation. Without Chrome, network access or `gh`, the tests that need them are skipped.
