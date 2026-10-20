"""Observe the documents the spec-agent-handoff goal changes.

Every test reads the current files of this checkout and looks for fixed tokens
that the goal's plan names: phrases, headings, front matter keys, links and
version numbers. A required token that is absent, or a forbidden token that is
present, is a violation raised as `RequirementViolation` with one identifier
per test. A file that already exists before the goal but cannot be read, front
matter that cannot be parsed, or JSON that cannot be decoded raise Unavailable,
an execution error, never a violation. New evaluation cases are opened only
through the links of the case list, so a case the list does not name yet is
not read at all.

Tokens are fixed strings, not an open scan of sentences. They establish that
the agreed wording is present; they do not establish that the prose is
coherent, that a host selects the skill, or that an agent follows it.
"""
import hashlib
import json
import re
import sys
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "tools"))
from result_observations import RequirementViolation  # noqa: E402

SPEC = "plugins/spec/skills/spec/"
SEAL = "plugins/seal/skills/seal/"
GOAL_DOCS = "contracts/goal-docs.md"
SPEC_DESCRIPTIONS = (
    "plugins/spec/plugin.json",
    "plugins/spec/.claude-plugin/plugin.json",
    "plugins/spec/.codex-plugin/plugin.json",
)
TEMPLATES = ("small-change.md", "backend-feature.md", "cross-module-feature.md", "product.md")
# Seal SKILL.md with the Authority section body and the version replaced by
# placeholders, as it is on the main commit this goal starts from (028c320).
# The digest holds only for this goal's own commits (028c320 to 950cfea). A
# later goal may change Seal SKILL.md outside the Authority section, as
# runner-adapters did for Seal 0.1.19 (merge c4a4564); on such a tree this test
# reports seal-outside-authority-changed although this goal did not change it.
SEAL_OUTSIDE_AUTHORITY = "c45ddd829684c900649e50efe45f57a9c5d0f6782d3f2c65cc4cb108b7ecc6bf"
SEAL_DESCRIPTION = (
    "Carry a goal in docs/specs/<goal>/ to recorded completion — plan, implement, verify through the ha "
    "binary, and resume in a new session. Use when the user calls this skill with a goal or asks to "
    "implement, finish, or continue a goal document, for example \"$seal:seal docs/specs/x\" in Codex, "
    "\"/seal:seal docs/specs/x\" in Claude Code, \"docs/specs/x 구현해줘\", or \"docs/specs/x 이어서 해줘\"."
)
# Skill versions on the main this goal merged into: 028c320, the start commit
# and the first parent of the merge 3ed4b60. AC-21 asks for versions above
# that main, so the check compares with these values and reads no ref or history.
MAIN_VERSIONS = {"spec": (0, 1, 12), "seal": (0, 1, 17)}
HANDOFF_CASES = ("same-request-handoff", "draft-then-ready", "target-then-draft", "long-session-release")
# Spec plugin files never name Seal, its commands, bundle files, run states,
# a review tool, or a finding class (plugins/plugins_test.go keeps the first four).
SPEC_FORBIDDEN = (r"(?i)\bseal\b", r"\bha (lint|start|check|note|status|done|log)\b",
                  r"PLAN\.md|REVIEW\.md|PROGRESS\.md|runs\.jsonl", r"\b(needs_user|budget_exhausted|incomplete)\b",
                  r"\bSpar\b", r"clean-review", r"requirement-question")


class Unavailable(Exception):
    """An input that must exist could not be read; reported as an error."""


def read(rel):
    try:
        return (ROOT / rel).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        raise Unavailable(f"cannot read {rel}: {error}") from None


def read_json(rel):
    try:
        return json.loads(read(rel))
    except ValueError as error:
        raise Unavailable(f"cannot decode {rel}: {error}") from None


def front_matter(rel):
    """Return the flat keys, the description and metadata.version of a SKILL.md."""
    lines = read(rel).split("\n")
    if lines[0] != "---":
        raise Unavailable(f"{rel} has no front matter")
    keys, version, in_meta = {}, "", False
    for line in lines[1:]:
        if line == "---":
            return keys, version
        key, _, value = line.strip().partition(":")
        value = value.strip().strip('"')
        if line.startswith(" "):
            if in_meta and key == "version":
                version = value
            continue
        keys[key] = value
        in_meta = key == "metadata"
    raise Unavailable(f"{rel} front matter is not closed")


def section(text, heading):
    """The body under a heading line up to the next heading of the same or a higher level."""
    level = len(heading) - len(heading.lstrip("#"))
    lines = text.split("\n")
    if heading not in lines:
        return ""
    start = lines.index(heading) + 1
    body = []
    for line in lines[start:]:
        hashes = len(line) - len(line.lstrip("#"))
        if hashes and hashes <= level and line[hashes:hashes + 1] == " ":
            break
        body.append(line)
    return "\n".join(body)


def version_tuple(text):
    parts = text.split(".")
    if len(parts) != 3 or not all(p.isdigit() for p in parts):
        raise Unavailable(f"version {text!r} is not major.minor.patch")
    return tuple(int(p) for p in parts)


def case_links(readme):
    return dict(re.findall(r"^\| \[([a-z0-9-]+)\]\(([a-z0-9-]+\.md)\) \|", readme, re.M))


class Doc(unittest.TestCase):
    def need(self, violation, where, text, *phrases):
        missing = [p for p in phrases if p not in text]
        if missing:
            raise RequirementViolation(violation, f"{where} lacks {missing}")

    def refuse(self, violation, where, text, *phrases):
        present = [p for p in phrases if p in text]
        if present:
            raise RequirementViolation(violation, f"{where} still says {present}")


class AC1(Doc):
    def test_agent_invocable_on_both_hosts(self):
        keys, _ = front_matter(SPEC + "SKILL.md")
        # Any spelling of the Codex policy value that YAML reads as false blocks it.
        policy = re.findall(r"^\s*allow_implicit_invocation\s*:\s*['\"]?([^'\"\s#]*)", read(SPEC + "agents/openai.yaml"), re.M)
        if "disable-model-invocation" in keys or any(v.lower() in ("false", "no", "off", "n") for v in policy):
            raise RequirementViolation("spec-explicit-only", f"Spec still blocks agent invocation: {keys} {policy}")

    def test_description_limits_agent_calls(self):
        keys, _ = front_matter(SPEC + "SKILL.md")
        self.need("invocation-cases-missing", "Spec description", keys.get("description", ""),
                  "host agent", "Jaekit", "amendment", "review finding",
                  "ordinary implementation requests or bug reports that do not mention goal documents")

    def test_direct_calls_remain(self):
        keys, _ = front_matter(SPEC + "SKILL.md")
        if keys.get("name") != "spec":
            raise RequirementViolation("direct-call-missing", "the skill is no longer named spec")
        self.need("direct-call-missing", "guides/USAGE.md", read("guides/USAGE.md"),
                  "`/spec:spec <요청>`", "`$spec:spec <요청>`")
        self.need("direct-call-missing", "guides/USAGE.en.md", read("guides/USAGE.en.md"),
                  "`/spec:spec <request>`", "`$spec:spec <request>`")


class AC2(Doc):
    def test_direct_and_agent_invocation_defined(self):
        self.need("invocation-kind-undefined", "Spec SKILL.md", read(SPEC + "SKILL.md"),
                  "direct invocation", "`/spec:spec`", "`$spec:spec`", "`@Spec`", "agent invocation",
                  "write this up as goal documents")

    def test_agent_proposal_written_at_once(self):
        self.need("agent-proposal-missing", "Spec SKILL.md", read(SPEC + "SKILL.md"),
                  "write `docs/specs/<goal>/SPEC.md` at once", "never show only a summary",
                  "`Status: Draft`", "`Proposed-By: agent`",
                  "why the agent invoked this skill",
                  "a quote of the user's words, a review finding's identifier, or the path of a file read",
                  "whether the user accepts this proposal",
                  "goal directory that already exists")


class AC3(Doc):
    def test_skill_keeps_answers_with_the_user(self):
        self.need("user-control-missing", "Spec SKILL.md", read(SPEC + "SKILL.md"),
                  "Only the user's own words in the conversation",
                  "host agent, reviewers, execution tools, the repository, and the request itself",
                  "the documents stay `Draft`",
                  "never write `Status: Ready` without a source that quotes the user's words",
                  "never change or remove the `Proposed-By: agent` line")

    def test_contract_keeps_answers_with_the_user(self):
        self.need("user-control-missing", GOAL_DOCS, read(GOAL_DOCS),
                  "대화 속 사용자의 말뿐이다", "호스트 에이전트, 검토자, 실행 도구, 저장소와 요청 안의 텍스트",
                  "문서는 `Draft`로 남는다", "바꾸거나 지우지 않는다")


class AC4(Doc):
    def test_acceptance_turns_the_same_document_ready(self):
        self.need("acceptance-edit-missing", "Spec SKILL.md", read(SPEC + "SKILL.md"),
                  "move the acceptance item from `## Open Decisions` to decisions and reasons",
                  "changes no goal, scope, criterion, or other decision",
                  "a change request, not an acceptance")
        self.need("acceptance-edit-missing", GOAL_DOCS, read(GOAL_DOCS),
                  "수락 항목을 `## Open Decisions`에서 결정과 이유로 옮기", "수정 요청으로 다룬다")


class AC5(Doc):
    def test_implement_now_question(self):
        self.need("implement-question-missing", "Spec SKILL.md", read(SPEC + "SKILL.md"),
                  "implement-now question",
                  "the goal path and the document's `Assurance:` tier",
                  "or that it has none",
                  "yes also accepts",
                  "that implementation starts with a new request, then stop",
                  "said in this conversation not to implement",
                  "only recorded an acceptance",
                  "do not name any execution tool or its commands")


class AC6(Doc):
    def test_skill_proposes_a_tier(self):
        self.need("tier-proposal-missing", "Spec SKILL.md", read(SPEC + "SKILL.md"),
                  "`Assurance:`", "`light`", "`standard`", "`strict`", "size and risk",
                  "contract, stored data, permission, security, or protected path",
                  "public contract", "migration", "irreversible", "protected regression path",
                  "boundary between components",
                  "the cost of a false success and how easily the change can be undone",
                  "do not add one unless the user decides",
                  "Never change an existing tier without the user's words")

    def test_templates_carry_a_tier(self):
        for name in TEMPLATES:
            text = read(SPEC + "assets/templates/" + name)
            header = text.split("\n## ", 1)[0]
            if not re.search(r"^Assurance: (light|standard|strict)$", header, re.M):
                raise RequirementViolation("template-tier-missing", f"{name} has no Assurance line before its first section")


class AC7(Doc):
    def test_contract_defines_the_implementation_request(self):
        self.need("implement-boundary-missing", GOAL_DOCS, read(GOAL_DOCS),
                  "구현 여부 질문", "\"응\"", "\"네\"", "\"구현해줘\"", "\"진행해\"",
                  "응, 그런데 AC-2는 빼줘", "대화 앞부분의 구현 요청", "질문이 다루지 않은 목표",
                  "질문 뒤에 목표 문서가 바뀐", "전체 목록이 아니다",
                  "그 질문이 보고한 문서에 대해 바로 답한 별도의 긍정만 센다",
                  "바뀐 것으로 치지 않는다",
                  "두 제품을 이어 쓰는 권한은 목표 문서가 저장된 뒤 사용자가 따로 보내는 구현 요청이다",
                  "`Status: Ready`는 실행 권한이 아니다")


class AC8(Doc):
    def test_authority_names_the_same_boundary(self):
        text = read(SEAL + "SKILL.md")
        authority = section(text, "## Authority")
        if not authority:
            raise Unavailable("Seal SKILL.md has no Authority section")
        self.need("seal-authority-boundary-missing", "Seal Authority", authority,
                  "whether the user or an agent made it",
                  "an answer to its questions about their content",
                  "even when it also asks for implementation, reports a bug, or this conversation asked for implementation earlier",
                  "the implement-now question",
                  "The user's affirmative answer to it",
                  "Quote that answer as-is in `ha start --request`",
                  "keep your interpretation out of the quote",
                  "An answer that also asks for a change",
                  "after the goal documents changed since the question",
                  "start only after the goal-document skill has recorded the acceptance or applied the amendment",
                  "Work starts only from a later message, sent once the goal documents are written.")

    def test_only_authority_and_version_change(self):
        text = read(SEAL + "SKILL.md")
        text = re.sub(r'(?m)^  version: "[^"\n]*"$', '  version: "<version>"', text, count=1)
        head, sep, rest = text.partition("\n## Authority\n")
        if not sep:
            raise Unavailable("Seal SKILL.md has no Authority section")
        _, _, tail = rest.partition("\n## ")
        outside = head + "\n## Authority\n<authority>\n## " + tail
        if hashlib.sha256(outside.encode("utf-8")).hexdigest() != SEAL_OUTSIDE_AUTHORITY:
            raise RequirementViolation("seal-outside-authority-changed",
                                       "Seal SKILL.md changed outside the Authority section and version")


class AC9(Doc):
    def test_skill_writes_amendments(self):
        self.need("amendment-flow-missing", "Spec SKILL.md", read(SPEC + "SKILL.md"),
                  "`docs/specs/<goal>/amendments/<name>.md`", "SPEC.md does not link it",
                  "stay unchanged", "the finding or question that prompted it",
                  "the section and criterion IDs it changes", "the sentences before and after the change",
                  "decided one at a time", "delete the amendment document",
                  "delete only the amendment document", "byte for byte",
                  "do not apply it even if the user accepts")

    def test_contract_defines_amendments(self):
        self.need("amendment-flow-missing", GOAL_DOCS, read(GOAL_DOCS),
                  "수정안 문서", "`amendments/`", "SPEC.md가 링크하지 않는", "바이트 단위로 같다",
                  "반영하지 않고 사용자에게 알린다")


class AC10(Doc):
    def test_contract_defines_header_lines(self):
        self.need("header-lines-undefined", GOAL_DOCS, read(GOAL_DOCS),
                  "`Assurance:`", "`light`, `standard`, `strict` 셋뿐이며 이것이 전체 목록이다",
                  "독립 검토를 생략할 수 있고, 생략하면 이유를 남긴다", "최종 후보와 그 뒤 수정을 검토한다",
                  "구현 전 계획 검토를 더한다", "실행 도구의 계약이 정한다", "줄이 없는 목표는 지금 규칙을 따른다",
                  "목표 digest가 바뀌고 목표가 바뀐 것으로 다룬다", "`ha status --format json`의 `assurance` 필드",
                  "`Proposed-By: agent`", "Core는 이 줄을 해석하지 않는다", "누가 썼는지 묻지 않는다")

    def test_spec_reference_defines_header_lines(self):
        self.need("header-lines-undefined", "Spec references/goal-docs.md", read(SPEC + "references/goal-docs.md"),
                  "`Assurance:`", "`light`, `standard`, or `strict`", "complete list",
                  "may skip independent review", "the final candidate and later fixes",
                  "a plan review before implementation", "contract of the execution tool",
                  "without the line follow the current rules", "changes the goal digest",
                  "`Proposed-By: agent`", "Execution tools do not interpret it",
                  "unrelated to any label an execution tool reports", "whoever wrote them")


class AC20(Doc):
    def test_products_entry(self):
        products = section(read("AGENTS.md"), "## Products")
        self.need("public-docs-stale", "AGENTS.md Products", products,
                  "host agent", "`Ready` and the start of implementation come only from the user's words",
                  "implement-now question", "the user's yes", "separate implementation request still works")

    def test_plugin_descriptions(self):
        texts = {rel: read_json(rel).get("description", "") for rel in SPEC_DESCRIPTIONS}
        market = read_json(".claude-plugin/marketplace.json")
        texts["marketplace spec"] = next((p.get("description", "") for p in market.get("plugins", []) if p.get("name") == "spec"), "")
        texts["codex longDescription"] = read_json("plugins/spec/.codex-plugin/plugin.json").get("interface", {}).get("longDescription", "")
        for where, text in texts.items():
            self.need("public-docs-stale", where, text,
                      "host agent", "Ready and the start of implementation come only from the user's words",
                      "implement now", "the user's yes", "separate implementation request still works")

    def test_usage_ko(self):
        text = read("guides/USAGE.md")
        self.need("public-docs-stale", "guides/USAGE.md", text,
                  "에이전트도 Spec을 부를 수 있습니다", "`Ready`와 구현 시작은 사용자의 말로만 일어납니다",
                  "구현할지 따로 묻고", "\"응\"이라고 답하면 구현이 시작됩니다", "구현 시작 명령도 계속 쓸 수 있습니다")
        status = section(text, "## 목표를 고치거나 이어 가기")
        self.need("public-docs-stale", "USAGE.md status section", status,
                  "`Status: Ready`", "`Assurance:`", "`light`", "`standard`", "`strict`", "다른 등급")

    def test_usage_en(self):
        text = read("guides/USAGE.en.md")
        self.need("public-docs-stale", "guides/USAGE.en.md", text,
                  "The host agent can also call Spec", "`Ready` and the start of implementation come only from your words",
                  "asks whether to implement", "If you answer yes, implementation starts", "start command still works")
        status = section(text, "## Changing a goal or continuing")
        self.need("public-docs-stale", "USAGE.en.md status section", status,
                  "`Status: Ready`", "`Assurance:`", "`light`", "`standard`", "`strict`", "another tier")

    def test_existing_cases_expect_the_question(self):
        # Cases that expected the hand-off notice: the notice stays only on a
        # report that still has a content decision open (marked 결정이 남은).
        for name in HANDOFF_CASES:
            text = read(f"evals/spec-cases/{name}.md")
            self.need("eval-cases-stale", name, text, "구현 여부 질문")
            stale = [line for line in text.splitlines() if "새 요청으로 시작한다고 알" in line and "결정이 남은" not in line]
            if stale:
                raise RequirementViolation("eval-cases-stale", f"{name} still expects the notice after documents with no open decision: {stale}")
        # The case-list table describes the same expectations.
        rows = [line for line in read("evals/spec-cases/README.md").splitlines()
                if any(line.startswith(f"| [{name}]({name}.md) |") for name in HANDOFF_CASES)]
        if len(rows) != len(HANDOFF_CASES):
            raise RequirementViolation("eval-cases-stale", f"evals/spec-cases/README.md lists {len(rows)} of the hand-off cases")
        stale = [line for line in rows if "새 요청으로 시작한다고 알" in line and "결정이 남은" not in line]
        if stale:
            raise RequirementViolation("eval-cases-stale", f"evals/spec-cases/README.md still describes the notice alone: {stale}")

    def test_new_cases_listed(self):
        readme = read("evals/spec-cases/README.md")
        links = case_links(readme)
        wanted = {"agent-draft-proposal": ("`Proposed-By: agent`", "`Status: Draft`", "받아들이는지"),
                  "implement-now-yes": ("구현 여부 질문", "\"응\"", "구현 요청이다"),
                  "implement-now-mixed-answer": ("구현 여부 질문", "응, 그런데", "구현 요청이 아니다")}
        for name, tokens in wanted.items():
            if links.get(name) != name + ".md":
                raise RequirementViolation("eval-cases-missing", f"evals/spec-cases/README.md does not list {name}")
            text = read(f"evals/spec-cases/{name}.md")
            self.need("eval-cases-missing", name, text, *tokens)

    def test_cases_stay_synthetic(self):
        readme = read("evals/spec-cases/README.md")
        for name, target in case_links(readme).items():
            if name not in ("agent-draft-proposal", "implement-now-yes", "implement-now-mixed-answer",
                            "same-request-handoff", "draft-then-ready", "target-then-draft"):
                continue
            text = read("evals/spec-cases/" + target)
            self.need("eval-case-not-synthetic", name, text, "평가용 가상 시나리오입니다. 실제 사용자 대화나 실행 기록이 아닙니다")
            self.refuse("eval-case-not-synthetic", name, text, "docs/specs/spec-agent-handoff", "](../../docs/", "side-session-notes")


class AC21(Doc):
    def test_versions_raised_and_agree(self):
        market = read_json(".claude-plugin/marketplace.json")
        for name, floor in MAIN_VERSIONS.items():
            _, version = front_matter(f"plugins/{name}/skills/{name}/SKILL.md")
            if version_tuple(version) <= floor:
                raise RequirementViolation("version-not-raised", f"{name} {version} is not above {floor}")
            found = {
                "claude": read_json(f"plugins/{name}/.claude-plugin/plugin.json").get("version"),
                "codex": read_json(f"plugins/{name}/.codex-plugin/plugin.json").get("version"),
                "marketplace": next((p.get("version") for p in market.get("plugins", []) if p.get("name") == name), None),
            }
            if any(v != version for v in found.values()):
                raise RequirementViolation("version-not-raised", f"{name} skill {version} disagrees with {found}")


class AC15(Doc):
    def test_direct_flow_kept(self):
        self.need("direct-flow-changed", "Spec SKILL.md", read(SPEC + "SKILL.md"),
                  "ask the user to choose before writing or editing any file",
                  "That message writes nothing; wait for the answer.",
                  "never wait for the user to agree before saving them",
                  "Set `Status: Ready` once no outcome-changing decision is open.",
                  "A direct invocation never adds a `Proposed-By:` line")


class AC16(Doc):
    def test_separate_request_still_starts(self):
        text = read(SEAL + "SKILL.md")
        keys, _ = front_matter(SEAL + "SKILL.md")
        if keys.get("description") != SEAL_DESCRIPTION:
            raise RequirementViolation("separate-start-changed", "the Seal skill description changed")
        self.need("separate-start-changed", "Seal Authority", section(text, "## Authority"),
                  "Work on a goal only when the user asks in this conversation to implement, finish, or continue it. "
                  "`Status: Ready` in SPEC.md is not such a request.")
        self.need("separate-start-changed", "guides/USAGE.md", read("guides/USAGE.md"),
                  "`/seal:seal docs/specs/<goal>`", "`$seal:seal docs/specs/<goal>`", "`docs/specs/<goal> 구현해줘`")
        self.need("separate-start-changed", "guides/USAGE.en.md", read("guides/USAGE.en.md"),
                  "`/seal:seal docs/specs/<goal>`", "`$seal:seal docs/specs/<goal>`", "`Implement docs/specs/<goal>`")


class AC18(Doc):
    def test_product_boundaries_kept(self):
        skill = read(SPEC + "SKILL.md")
        self.need("product-boundary-changed", "Spec SKILL.md", skill,
                  "Do not implement, edit code, or run commands that change the repository other than writing the goal documents.",
                  "no branches, merges, version changes, pushes, pull requests",
                  "This skill never edits code or runs checks.")
        self.need("product-boundary-changed", "Seal SKILL.md", read(SEAL + "SKILL.md"),
                  "Editing goal documents, or lowering, dropping, or reinterpreting a criterion.")
        for path in sorted((ROOT / "plugins/spec").rglob("*")):
            if path.is_file():
                rel = path.relative_to(ROOT).as_posix()
                text = read(rel)
                # Go's \b is ASCII-only, so a mention followed by Hangul still matches.
                found = [m.group(0) for pattern in SPEC_FORBIDDEN for m in [re.search(pattern, text, re.ASCII)] if m]
                if found:
                    raise RequirementViolation("product-boundary-changed", f"{rel} mentions {found}")


if __name__ == "__main__":
    unittest.main()
