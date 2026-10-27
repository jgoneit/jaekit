# Site source

The [documentation site](https://jgoneit.github.io/jaekit/) is built from the repository: the home page from `home.html` and `static/`, and every documentation page from a Markdown file directly under `guides/` or `contracts/` in the checked-out commit. `X.en.md` is English and `X.ko.md` Korean; `X.md` is the other language of its `X.ko.md` or `X.en.md`, and Korean when it has neither, so `contracts/regressions.md` is English beside `contracts/regressions.ko.md`. A document that exists in one language only is listed in both with a mark and opens in its language. The builder reads those files from Git as committed, so edits that are not committed, files that are only staged and untracked files do not appear; commit them to see them. Pages show the documents as GitHub shows them: every heading gets GitHub's anchor, which `tools/check_public_docs.py` computes by the same rule, and bare web and e-mail addresses become links. Links are checked by the rules of `tools/check_public_docs.py`, the check the repository's documents pass. A page's address is its file name in lowercase, with a `_` before a name made only of dots. The release version comes from `metadata.version` in `.claude-plugin/marketplace.json`. Nothing in this folder copies those documents, and the built site is never committed: the [site workflow](../../.github/workflows/pages.yml) builds and publishes it from `main`.

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

Exit codes: 0 built; 1 a document or the release metadata in the commit cannot be used, such as a link to a missing file or heading, two files with the same address, or a missing release version; 2 a usage error, including an `--out` folder that is not an earlier build; 3 a pinned download could not be fetched or verified; 4 a tool the build needs cannot run, such as an older Python, or Git cannot read the repository, or the build stopped unexpectedly. Nothing is written unless the build succeeds.

## Checks

The site's checks are Go tests in `tools/goal-checks/pages-site/`, behind the `pagessite` build tag, so the repository's default validation does not run them. They build the site, read it, and drive headless Chrome for the page behavior. The tests the site workflow runs are:

```bash
go test -tags pagessite -count=1 -run "TestPagesSiteAC(2|3|4|5|6|7|8|9|10|11)_" ./tools/goal-checks/pages-site
```

The rendering test compares every page, and a test document of further Markdown forms, with GitHub's own rendering of the file as GitHub shows a file (`gh api -X POST markdown` in `markdown` mode), kept in the download cache. The other tests read the published site and GitHub's settings, and one runs the whole repository validation. Without Chrome, network access or `gh`, the tests that need them are skipped.
