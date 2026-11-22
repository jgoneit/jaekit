#!/usr/bin/env python3
"""Observe the Seal check-authoring guidance in the checked tree.

Each clause names a product document, the section (or table row) that must
hold it and the semantic anchors it must contain there, so an anchor elsewhere
in the file does not count. A clause on a skill document reads the section of
that heading in every document of the skill, SKILL.md first, so a rule may
move between them. Relations compare the guidance with the Core facts
it states, the source identity files and the existing user points it keeps.
Everything is read from the tree this process runs in: no commit, ref or
history is read, so a baseline run on the base commit and a run on the change
judge the same property. A missing product file is an observed violation; a
tool that cannot run is an error, never a violation.
"""
import json
import os
import re
import shutil
import subprocess
import sys

import check as checker

SUITE = "authoring"
P = checker.SEAL
SKILL = P + "SKILL.md"
BUNDLE = P + "references/bundle.md"
HA = P + "references/ha.md"
PLAN = P + "assets/templates/PLAN.md"
REVIEW = P + "assets/templates/REVIEW.md"
CONTRACT = "contracts/bundle.md"
# The skill's check input rules apply to every check written now.
APPLY_NOW = "They apply to every check written or changed under this skill, whichever Seal started the goal."

# A section runs from its heading line to the next heading of the same or a
# higher level. A row is the one table line whose first cell is the label.
SECTIONS = {
    "skill": (SKILL, "## Check inputs"),
    "bundle": (BUNDLE, "## Check inputs"),
    "bundle-review": (BUNDLE, "## REVIEW.md"),
    "ha": (HA, "## What checks read"),
    "plan": (PLAN, "## 조건표"),
    "review": (REVIEW, "## 검증 방법과 환경"),
    "review-decisions": (REVIEW, "## 중요한 미결정"),
    "contract": (CONTRACT, "### 2.1.2 Check inputs and observation"),
    "contract-review": (CONTRACT, "## 4. `REVIEW.md`"),
}
ROW_SOURCES = {"bundle-review": ("검증 방법과 환경", "중요한 미결정"),
               "contract-review": ("`검증 방법과 환경` (verification method and environment)",
                                   "`중요한 미결정` (important open decisions)")}

CLAUSES = {
    "AC-1": [
        ("skill", ("every check in the condition table", "`change`, `maintain`, `EX-n` and task commands",
                   "producer, configuration or helper file", "the tree being checked",
                   "external inputs its condition names", "start or base commit", "fixed commit",
                   "start record or Core output", "other commits, refs or objects",
                   "full clone and a history-less clone", "synthetic repository",
                   "references/bundle.md#check-inputs")),
        ("bundle", ("same verdict", "full clone and a history-less clone", "pinned SHA",
                    "base read from the start record or Core output", "diff against an earlier commit",
                    "remote-tracking branch", "`HEAD~n`", "merge-base", "log range", "another worktree",
                    "synthetic repository the check builds itself", "producer, configuration or helper file",
                    "`change`, `maintain`, `EX-n` and task commands")),
        ("review", ("repository history",)),
    ],
    "AC-2": [
        ("skill", ("`maintain`", "property the goal documents require", "content, output, format or behavior",
                   "unchanged since the base commit", "does not observe that property",
                   "outside `바꿀 수 있는 경로`", "`out_of_scope`", "expected content written in a tracked check file")),
        ("bundle", ("content, output, format or behavior", "not an observation of that property",
                    "outside `바꿀 수 있는 경로`", "`out_of_scope`", "since the base commit", "tracked check file")),
        ("plan", ("`maintain`", "property the goal documents require", "unchanged since the base commit",
                  "`out_of_scope`")),
    ],
    "AC-3": [
        ("skill", ("only by Core's baseline run", "never checks out, reads or compares the base commit",
                   "violation on the base commit and a pass after the change")),
        ("bundle", ("temporary worktree of the base commit", "check paths copied from HEAD",
                    "never checks out, reads or compares the base commit",
                    "violation on the base commit and a pass after the change")),
        ("ha", ("baseline", "temporary worktree of the base commit", "`out_of_scope`",
                "`test_definition_changed`", "Checks themselves")),
    ],
    "AC-4": [
        ("skill", ("about repository history or refs", "tag's type", "commit message format",
                   "only the range and refs the condition names",
                   "error or unobserved", "neither a violation nor a pass", "pre-execution review")),
        ("bundle", ("tag's type", "commit message format", "only the range and refs the condition names",
                    "unobserved, environment error or skip", "neither a violation nor a pass",
                    "records why the check needs history and the range it reads")),
        ("review", ("range it reads",)),
    ],
    "AC-5": [
        ("skill", ("declares its own scope or trust boundary", "pinned to a version or release",
                   "preserving past verdicts", "isolating imports", "limiting its inputs",
                   "current behavior", "keeps the declaration", "explicit assumption", "the report repeats",
                   "without asking the user", "same import boundary", "document-conflict rule")),
        ("bundle", ("pinned to a version or release", "preserves past verdicts", "isolates imports",
                    "limits its inputs", "current behavior", "explicit assumption", "repeat it in the report",
                    "inside the boundary it declares", "same import boundary", "without asking the user",
                    "document-conflict rule")),
        ("review-decisions", ("declared scope or trust boundary",)),
    ],
    "AC-6": [
        ("skill", ("add no user confirmation, approval or question",
                   "the executor applies them and decides whether an existing check needs rewriting")),
        ("bundle", ("add no user confirmation, approval or question",
                    "decides whether an existing check is worth rewriting")),
        ("contract", ("add no user confirmation, approval or question step",
                      "The executor applies the rules and judges whether to fix existing checks")),
    ],
    "AC-8": [
        ("skill", (APPLY_NOW, "never require rewriting or converting an existing goal's checks, records, "
                              "`PROGRESS.md` or `REVIEW.md` for this reason alone")),
        ("bundle", (APPLY_NOW, "never require rewriting or converting an existing goal's checks, records, "
                               "`PROGRESS.md` or `REVIEW.md` for this reason alone")),
        ("contract", ("apply to checks written or changed from Seal 0.1.14 on", "Which Seal version started the goal does not matter",
                      "never require rewriting or converting an existing goal's checks, records, PROGRESS or REVIEW")),
    ],
    "AC-9": [
        ("contract", (
            # AC-1
            "`change`, `maintain`, `EX-n`, task commands", "the producer, configuration and helper files",
            "the tree being checked", "the external inputs the condition names", "start or base commit", "fixed commit",
            "the start record or Core output", "other commits, refs or objects", "a diff against a past commit",
            "merge-base", "remote-tracking branch", "`HEAD~n`", "log range", "another worktree",
            "a full clone and a history-less clone", "synthetic repository",
            # AC-2
            "`maintain`", "property the goal documents require (content, output, format, behavior)",
            "not an observation of that property", "outside `바꿀 수 있는 경로`", "`out_of_scope`",
            "expected content written in a tracked check file",
            # AC-3
            "observed only by Core's baseline run", "temporary worktree", "checking out, reading or comparing",
            "a violation on the base and a pass after the change",
            # AC-4
            "a tag's type or a commit message format", "only the range and refs", "neither a violation nor a pass",
            "an error or unobserved", "Write in the pre-execution review",
            # AC-5
            "scope or trust boundary",
            "pinning to a specific version or release, preserving past verdicts, isolating imports and limited inputs",
            "current behavior", "explicit assumption", "repeat it in the report", "Do not ask the user anew",
            "same import boundary", "document-conflict rule",
            # AC-6, AC-8
            "add no user confirmation, approval or question step", "Seal 0.1.14")),
    ],
}

# Table rows that must carry a clause: (criterion, section key, row label, anchor).
ROWS = [
    ("AC-1", "bundle-review", "검증 방법과 환경", "repository history"),
    ("AC-4", "bundle-review", "검증 방법과 환경", "range it reads"),
    ("AC-5", "bundle-review", "중요한 미결정", "declared scope or trust boundary"),
    ("AC-9", "contract-review", "`검증 방법과 환경` (verification method and environment)", "repository history"),
    ("AC-9", "contract-review", "`중요한 미결정` (important open decisions)", "scope or trust boundary"),
]

# Existing rules this goal keeps (SPEC: 범위와 비목표, 유지).
KEPT = [
    (SKILL, ("Python 3 is not a prerequisite for Seal", "does not require an official adapter",
             "Existing `/1` and `/2` goals retain their result semantics", "## Assurance policy")),
    (BUNDLE, ("No particular runner or adapter is required", "Any other value is the lint problem `other_bundles_invalid`")),
    (CONTRACT, ("Existing `/1` and `/2` goals keep their earlier result semantics",)),
]

# The existing points that reach the user (SPEC AC-6): document conflicts,
# outcome-changing open decisions, out-of-scope changes, changed tests and
# manual conditions. Their statements stay, and the user-resolved Core reasons
# are exactly these.
USER_POINTS = [
    (SKILL, ("ha note <goal> confirm <AC-n | scope:<path> | tests:<path> | spec>",
             "When the goal, scope, or criteria look wrong or need to change, ask the user",
             "`needs_user` means asking the user about those items")),
    (BUNDLE, ("Changes outside them need the user's confirmation",
              "Editing or deleting a test that existed at base needs the user's confirmation",
              "Never resolve them by editing the goal documents; ask",
              "Close each with the user's quoted answer or an explicit assumption that the report repeats")),
]
USER_REASONS = {"record_invalid", "error_limit", "manual_unconfirmed", "record_integrity",
                "out_of_scope", "test_definition_changed", "spec_changed", "review_open_question"}
ASKING = re.compile(r"(?i)\b(ask|asks|asking|approval|approve|confirmation|confirm|permission)\b"
                    r"|\b(?:question|quer(?:y|ie))(?:s|d|ed|ing)?(?=\s+(?:the\s+|a\s+|any\s+)?users?\b)"
                    r"|질문한다|질문하고|질문해|사용자에게 질문"
                    r"|묻는다|묻고|물어|승인을 받|확인을 받|사용자 결정이 필요|사용자 확인이 필요|사용자에게 확인")
# A phrase that says no step is added, or that names an existing point the
# user already resolves, exempts only the asking words it governs: those inside
# it, those right after it with only an object or a list between ("never ask
# the user for permission", "add no user confirmation, approval or question"),
# and, for a Korean phrase, which negates afterwards, those earlier in the same
# clause. Other asking words in the sentence are still new steps.
NOT_ASKING = re.compile(r"(?i)without asking|never ask|not ask|add no user|no user confirmation"
                        r"|document-conflict rule|only the user resolves|묻지 않|새로 묻지|더하지 않|문서 충돌 규칙|사용자만 해소")
GOVERNED_WORDS = {"the", "a", "an", "any", "user", "users", "for", "to", "of", "or", "and"}
# "without asking" governs gerunds, not a following base-form verb, so only these
# phrases carry their negation over to another asking verb.
NEGATED_VERB = re.compile(r"(?i)never ask|not ask")
# A list the phrase governs continues after a comma only with another asking noun.
ASKING_NOUNS = re.compile(r"(?i)approval|confirmation|permission")

PRIVATE = (
    (re.compile(r"(?<![0-9A-Za-z])[0-9a-f]{7,40}(?![0-9A-Za-z])"), "commit-like hexadecimal"),
    (re.compile(r"(?i)\bPR ?#\d+|/pull/\d+|discussion_r\d+|(?<![\w`])#\d{2,}\b"), "pull request reference"),
    # Checkout-local absolute paths on macOS, Linux, containers and Windows.
    # Any absolute path of two or more segments that is not part of a URL or a
    # relative path, a bare well-known root, a worktree folder or a Windows drive.
    (re.compile(r"(?<![\w.:/~<>-])/[^\s/`]+/[^\s/`]"
                r"|(?<![\w.:/~<>-])/(?:Users|home|root|workspaces?|private|var|tmp|mnt|srv|opt|builds|usr|Volumes)(?![\w/-])(?!\.\w)"
                r"|-worktrees/|(?<![A-Za-z])[A-Za-z]:[\\/][^\s`]"), "local path"),
    # A placeholder segment is exempt only when the path ends there or continues
    # with "/": "docs/specs/example/SPEC.md", not "docs/specs/example-private/".
    (re.compile(r"docs/specs?/(?!(?:<goal>|x|example|sample)(?:/|(?![\w.-])|\.(?![\w-])))[A-Za-z0-9]"),
     "concrete goal path"),
    (re.compile(r"\.\./[A-Za-z0-9._-]+/(?:SPEC|CONTEXT|PLAN|REVIEW|PROGRESS)\.md"), "link to another goal's documents"),
    (re.compile(r"runs\.jsonl:\d+|seq \d+ \("), "execution record reference"),
)


def governed(between, word, negated_verb):
    """Whether text between an exemption phrase and a later asking word is only
    the phrase's object or a list it governs. After an asking noun, the list goes
    on with more asking nouns joined by commas, "or" or "and". A negated verb
    ("never ask for approval or confirm") also governs another asking verb joined
    by "or" or "and"; a negated noun list ("add no user approval and ask the
    user") does not, and an asking verb after a comma always starts a new step."""
    tokens = re.findall(r"@[^@]*@|\w+|[^\w\s]", ASKING.sub(lambda m: f"@{m.group(0)}@", between))
    tokens.append(f"@{word}@")

    def noun(token):
        return token.startswith("@") and ASKING_NOUNS.fullmatch(token.strip("@")) is not None

    seen_noun = False
    for index, token in enumerate(tokens):
        if token.startswith("@"):
            joined = index and tokens[index - 1].lower() in ("or", "and")
            if seen_noun and not noun(token) and not (negated_verb and joined):
                return False
            seen_noun = seen_noun or noun(token)
        elif token == ",":
            following = next((t for t in tokens[index + 1:] if t.lower() not in ("or", "and")), "")
            if not (index and noun(tokens[index - 1]) and noun(following)):
                return False
        elif token.lower() not in GOVERNED_WORDS:
            return False
    return True


def new_user_steps(sentence):
    """Asking words in a sentence that no exemption phrase governs."""
    exempt = [match.span() for match in NOT_ASKING.finditer(sentence)]
    steps = []
    for match in ASKING.finditer(sentence):
        start, end = match.span()
        covered = any(
            (left <= start and end <= right)
            or (right <= start and governed(sentence[right:start], match.group(0),
                                            NEGATED_VERB.fullmatch(sentence[left:right]) is not None))
            or (end <= left and re.search(r"[가-힣]", sentence[left:right])
                and not re.search(r"[,;:]", sentence[end:left]))
            for left, right in exempt)
        if not covered:
            steps.append(match.group(0))
    return steps


# History-dependent tokens a template check command must not carry.
HISTORY_TOKENS = re.compile(r"\bgit\b|HEAD[~^]|@\{|origin/|merge-base|(?<![0-9A-Za-z])[0-9a-f]{7,40}(?![0-9A-Za-z])")

# The plugin package runs under go; its tests launch git fixtures and python3
# contract suites. A test that fails because one of them cannot start has not
# observed the guidance, so each of the three is started once before the run.
REQUIRED_TOOLS = {"go": ["go", "version"], "git": ["git", "--version"], "python3": ["python3", "-c", "pass"]}
REQUIRED_GO_TESTS = ("TestManifestsAndVersionsAgree", "TestCompatibilityAC10_SourceVersions",
                     "TestSealSkillPrescribesNoOrder", "TestSealSkillCarriesCompletionRules",
                     "TestExampleFromTemplatesPassesLint", "TestRunRules2SealGuide")


def read(root, name):
    path = root / name
    return path.read_text(encoding="utf-8") if path.exists() else None


def section(root, key):
    """The section text, None when the file is absent, '' when the heading is.

    For a skill document, the sections with this heading in every document of
    that skill are read together, SKILL.md first.
    """
    name, heading = SECTIONS[key]
    if read(root, name) is None:
        return None
    names = checker.skill_documents(root, name) if checker.SKILL_DOC.match(name) else [name]
    parts = [part for part in (one_section(read(root, doc), heading) for doc in names) if part is not None]
    return "\n".join(parts)


def section_span(lines, heading):
    """The indexes (start, end) of the lines under heading, or None when it is absent."""
    level = len(heading) - len(heading.lstrip("#"))
    for i, line in enumerate(lines):
        if line.strip() == heading:
            for j in range(i + 1, len(lines)):
                depth = len(lines[j]) - len(lines[j].lstrip("#"))
                if 0 < depth <= level and lines[j][depth:depth + 1] == " ":
                    return i + 1, j
            return i + 1, len(lines)
    return None


def one_section(source, heading):
    """One document's section under heading, or None when it lacks the heading."""
    if source is None:
        return None
    lines = source.split("\n")
    span = section_span(lines, heading)
    return None if span is None else "\n".join(lines[span[0]:span[1]])


def row(root, key, label):
    rows = [line for line in (section(root, key) or "").split("\n") if line.startswith(f"| {label} |")]
    return rows[0] if len(rows) == 1 else None


def json_object(text):
    """A product manifest's object, or None when it is absent or malformed."""
    try:
        value = json.loads(text) if text is not None else None
    except json.JSONDecodeError:
        return None
    return value if isinstance(value, dict) else None


def version(text):
    match = re.search(r'(?m)^\s+version: "(\d+)\.(\d+)\.(\d+)"$', text or "")
    return tuple(int(part) for part in match.groups()) if match else None


def regressions(root):
    """Run the skill and contract consistency regressions in this tree.

    A failing test is an observed contradiction. A missing tool, a build or
    setup failure, a run ended by a signal, or required tests that did not run
    is not an observation and raises, which the caller reports as error. Go flags are overridden with a
    single space: an empty GOFLAGS would fall back to a value persisted with
    `go env -w`, which could skip the required tests. A failure inside a test whose tools
    all started is read as observed; the tests only read tracked files and
    build synthetic fixtures with those tools.
    """
    for tool, probe in REQUIRED_TOOLS.items():
        if shutil.which(tool) is None:
            raise RuntimeError(f"{tool} is not available")
        try:
            started = subprocess.run(probe, cwd=root, capture_output=True, text=True, timeout=60)
        except (OSError, subprocess.TimeoutExpired) as exc:
            raise RuntimeError(f"{tool} cannot start: {exc}") from None
        if started.returncode != 0:
            raise RuntimeError(f"{tool} cannot start: {(started.stderr or started.stdout)[-200:]}")
    env = dict(os.environ, GOFLAGS=" ", GOWORK="off")
    go = subprocess.run(["go", "test", "-count=1", "-json", "./plugins/"], cwd=root, capture_output=True,
                        text=True, env=env)
    results, outputs = {}, {}
    for line in go.stdout.splitlines():
        try:
            event = json.loads(line)
        except json.JSONDecodeError:
            continue
        name = event.get("Test")
        if name and "/" not in name:
            if event.get("Action") in ("pass", "fail", "skip"):
                results[name] = event["Action"]
            elif event.get("Action") == "output":
                outputs[name] = outputs.get(name, "") + event.get("Output", "")
    failed = sorted(name for name, action in results.items() if action == "fail")
    missing = [name for name in REQUIRED_GO_TESTS if results.get(name) not in ("pass", "fail")]
    if go.returncode < 0 or (go.returncode != 0 and not failed):
        raise RuntimeError("plugin regressions could not run: " + (go.stderr or go.stdout)[-400:])
    if missing:
        raise RuntimeError(f"required plugin regressions did not run: {missing}")
    problems = [f"plugin consistency regression {name} failed: {outputs.get(name, '')[-300:]}" for name in failed]
    docs = subprocess.run([sys.executable, "tools/check_public_docs.py"], cwd=root, capture_output=True, text=True)
    if docs.returncode == 1:
        problems.append("public documentation check failed: " + (docs.stdout + docs.stderr)[-400:])
    elif docs.returncode != 0:
        raise RuntimeError("public documentation check could not run: " + (docs.stdout + docs.stderr)[-400:])
    return problems


def changed_texts(root):
    """Every place this goal's guidance is written, for AC-6 and AC-10.

    Each text is named after the document that holds it, and each line of a
    document is scanned at most once, by its position. A skill document's
    section and its table rows are read in each document of the skill that has
    the heading; then the lines stating APPLY_NOW in every document the
    clause lookup reads, except those already read.
    """
    texts, covered = [], {}

    def documents(name):
        return checker.skill_documents(root, name) if checker.SKILL_DOC.match(name) else [name]

    def spans(key):
        """(document, its lines, the span under the key's heading) for each document that has it."""
        name, heading = SECTIONS[key]
        if read(root, name) is None:
            return
        for doc in documents(name):
            source = read(root, doc)
            span = section_span(source.split("\n"), heading) if source is not None else None
            if span is not None:
                yield doc, source.split("\n"), span

    for key in ("skill", "bundle", "ha", "contract", "plan", "review", "review-decisions"):
        for doc, lines, (start, end) in spans(key):
            fresh = [i for i in range(start, end) if i not in covered.get(doc, ())]
            covered.setdefault(doc, set()).update(fresh)
            text = "\n".join(lines[i] for i in fresh)
            if text:
                texts.append((doc, text))
    for key, labels in ROW_SOURCES.items():
        for doc, lines, (start, end) in spans(key):
            for i in range(start, end):
                if i not in covered.get(doc, ()) and any(lines[i].startswith(f"| {label} |") for label in labels):
                    covered.setdefault(doc, set()).add(i)
                    texts.append((doc, lines[i]))
    for doc in documents(SKILL):
        texts.extend((doc, line) for i, line in enumerate((read(root, doc) or "").split("\n"))
                     if APPLY_NOW in line and i not in covered.get(doc, ()))
    return texts


def observe(root, suite, criterion):
    if suite != SUITE:
        return base_observe(root, suite, criterion)
    problems = []

    def require(name, text, anchors, where):
        if text is None:
            problems.append(f"{name}: required product artifact absent")
            return
        for anchor in anchors:
            if not checker.has_anchor(text, anchor):
                problems.append(f"{name} {where}: missing {anchor!r}")

    for key, anchors in CLAUSES.get(criterion, []):
        name, heading = SECTIONS[key]
        text = section(root, key)
        if text == "":
            problems.append(f"{name}: section {heading!r} absent")
            continue
        require(name, text, anchors, heading)
    for owner, key, label, anchor in ROWS:
        if owner == criterion:
            line = row(root, key, label)
            if line is None or anchor not in line:
                problems.append(f"{SECTIONS[key][0]}: row {label!r} lacks {anchor!r}")
    if criterion == "AC-2":
        # The template's maintain example observes no history.
        maintain = [line for line in (section(root, "plan") or "").split("\n") if line.startswith("| AC-2 | maintain |")]
        if len(maintain) != 1:
            problems.append(f"{PLAN}: maintain example row absent")
        else:
            cells = [cell.strip() for cell in maintain[0].strip("|").split("|")]
            if len(cells) != 5 or HISTORY_TOKENS.search(cells[2]) or HISTORY_TOKENS.search(cells[3]):
                problems.append(f"{PLAN}: maintain example depends on history: {maintain[0]}")
        # The guidance states Core's out_of_scope; the Core source and reference must say the same.
        require("internal/status/status.go", read(root, "internal/status/status.go"),
                ('add("out_of_scope", "changed since base outside 바꿀 수 있는 경로", outside)',), "out_of_scope")
        require(HA, checker.clause_text(root, HA),
                ("| `out_of_scope` | user | Files changed since base outside `바꿀 수 있는 경로` |",), "Reasons")
    if criterion == "AC-3":
        require(HA, checker.clause_text(root, HA), (
            "Runs `change` conditions on a temporary worktree of the base commit with check paths copied from HEAD: "
            "up to `/7` the condition's own, under `/8` those of every marked change condition",), "Commands")
        require("cmd/ha/baseline.go", read(root, "cmd/ha/baseline.go"), ("baselineFiles",), "baseline overlay")
    if criterion == "AC-8":
        skill = version(read(root, SKILL))
        if skill is None or skill <= (0, 1, 13):
            problems.append(f"Seal source version {skill} is not above 0.1.13")
        else:
            want = ".".join(map(str, skill))
            for host in ("plugins/seal/.claude-plugin/plugin.json", "plugins/seal/.codex-plugin/plugin.json"):
                manifest = json_object(read(root, host))
                if manifest is None:
                    problems.append(f"{host} is absent or not a JSON object")
                elif manifest.get("version") != want:
                    problems.append(f"{host} does not carry {want}")
            market = json_object(read(root, ".claude-plugin/marketplace.json"))
            if market is None:
                problems.append(".claude-plugin/marketplace.json is absent or not a JSON object")
                market = {}
            metadata, entries = market.get("metadata"), market.get("plugins")
            if not isinstance(metadata, dict) or metadata.get("version") != "0.1.8":
                problems.append("marketplace release identity is not 0.1.8")
            if not isinstance(entries, list) or not all(isinstance(entry, dict) for entry in entries):
                problems.append(".claude-plugin/marketplace.json plugins is not a list of objects")
                entries = []
            if [entry.get("version") for entry in entries if entry.get("name") == "seal"] != [want]:
                problems.append(f"marketplace seal entry does not carry {want}")
    if criterion == "AC-9":
        for name, anchors in KEPT:
            require(name, checker.clause_text(root, name), anchors, "kept rules")
        if not problems:
            problems.extend(regressions(root))
    if criterion == "AC-6":
        for name, anchors in USER_POINTS:
            require(name, checker.clause_text(root, name), anchors, "existing user points")
        # The reasons table may be in any document the clause lookup reads.
        found = {reason for doc in checker.skill_documents(root, HA)
                 for reason in re.findall(r"(?m)^\| `([a-z_]+)` \| user \|", read(root, doc) or "")}
        if found != USER_REASONS:
            problems.append(f"user-resolved reasons changed: {sorted(found ^ USER_REASONS)}")
        for name, text in changed_texts(root):
            for sentence in re.split(r"(?<=[.!?])\s+|\n", text):
                if new_user_steps(sentence):
                    problems.append(f"{name}: guidance may add a user step: {sentence.strip()[:160]}")
    if criterion == "AC-10":
        for name in sorted({name for name, _ in SECTIONS.values()}):
            if read(root, name) is None:
                problems.append(f"{name}: required product artifact absent")
        for name, text in changed_texts(root):
            for pattern, label in PRIVATE:
                for match in pattern.finditer(text):
                    problems.append(f"{name}: {label} in new guidance: {match.group(0)!r}")
    target = f"{suite}.{criterion}"
    observation = {"target": target, "attempt": 1, "status": "pass"}
    if problems:
        observation.update(status="violation", violation=f"{target}.missing-contract")
    return observation, problems


base_observe = checker.observe
checker.REQUIREMENTS[SUITE] = {criterion: [] for criterion in
                               ("AC-1", "AC-2", "AC-3", "AC-4", "AC-5", "AC-6", "AC-8", "AC-9", "AC-10")}
checker.observe = observe

if __name__ == "__main__":
    sys.argv[1:1] = [SUITE] if len(sys.argv) == 2 else []
    raise SystemExit(checker.main())
