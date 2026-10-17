#!/usr/bin/env python3
"""Observe decision history through the real `ha history` CLI.

Each criterion builds `ha` from this checkout and drives it against synthetic
local repositories (normal checkouts, linked worktrees, diverged run records,
clones). No declaration is read to choose an observed violation: a runnable
CLI without the command is reported as the missing feature, distinct from
build, setup, and process errors. Fixtures are synthetic; nothing reads private
documents. AC-10 additionally observes the current public PR CI through GitHub.
"""
from __future__ import annotations

import argparse
from concurrent.futures import ThreadPoolExecutor
import fcntl
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import stat
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
PROBE = Path(__file__).resolve().parent / 'platform_probe.sh'
STORAGE_BOUNDARY_TESTS = {'TestStoragePinsDirectoryHierarchy', 'TestStorageRejectsReplacedLeaf',
                          'TestNativeStorageBoundsHistoryLines'}
FILE_BOUNDARY_TESTS = {'TestFileEvidenceRejectsReplacedLeaf', 'TestFileEvidenceRejectsReplacedParent', 'TestFileEvidenceRejectsReplacedRoot',
                       'TestFileEvidencePreservesStablePathStates', 'TestFileEvidencePreservesSearchOnlyDirectories',
                       'TestFileEvidenceConfinedWalkRejectsLinks', 'TestFileEvidenceBoundsDirectoryDescriptors',
                       'TestFileEvidenceBoundsGrowth'}


class Violation(Exception):
    def __init__(self, code, detail):
        super().__init__(code)
        self.code, self.detail = code, detail


class Environment(Exception):
    def __init__(self, reason, detail):
        super().__init__(reason)
        self.reason, self.detail = reason, detail


def require(value, code, detail):
    if not value:
        raise Violation(code, str(detail)[:4000])


def run(argv, cwd, env=None, timeout=300, stdin=subprocess.DEVNULL):
    try:
        return subprocess.run([str(x) for x in argv], cwd=cwd, env=env, stdin=stdin,
                              text=True, capture_output=True, timeout=timeout)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Environment('execution_error', str(error)) from error


def build(temp, goos=None, goarch=None, name='ha'):
    binary = temp / name
    env = dict(os.environ, CGO_ENABLED='0')
    if goos:
        env.update(GOOS=goos, GOARCH=goarch)
    result = run(['go', 'build', '-o', binary, './cmd/ha'], ROOT, env)
    if result.returncode:
        raise Environment('compile_error', result.stderr)
    return binary


def probe(binary):
    cap = run([binary, 'capabilities', '--format', 'json'], ROOT)
    if cap.returncode:
        raise Environment('setup_error', 'capabilities failed: ' + cap.stderr)
    try:
        capabilities = json.loads(cap.stdout)
    except ValueError as error:
        raise Environment('setup_error', 'capabilities output invalid') from error
    help_ = run([binary, 'history', '--help'], ROOT)
    features = capabilities.get('features', [])
    if 'unknown command "history"' in help_.stderr and 'decision-history/v1' not in features:
        raise Violation('history-command-unavailable', 'runnable CLI has no history command')
    require('decision-history/v1' in features and
            capabilities.get('formats', {}).get('decision_history') == ['decision-history/v1'],
            'history-capability-missing', capabilities)
    return capabilities


def code_of(result):
    match = re.search(r'ha: ([a-z_]+):', result.stderr)
    return match[1] if match else ''


def unit_observations(package, names):
    """Require every named boundary test to run; a missing test is not a pass."""
    result = run(['go', 'test', '-count=1', '-json', package,
                  '-run', '^(%s)$' % '|'.join(sorted(names))], ROOT)
    events = [json.loads(line) for line in result.stdout.splitlines() if line.startswith('{')]
    failed = {e.get('Test') for e in events if e.get('Action') == 'fail'} - {None}
    passed = {e.get('Test') for e in events if e.get('Action') == 'pass'}
    require(not failed, 'history-unit-assertion', sorted(failed))
    if result.returncode or not set(names) <= passed:
        raise Environment('collection_error', result.stdout[-2000:] + result.stderr)


SPEC = '''# Synthetic

Status: Ready
Criteria-Format: nested/1

## Acceptance Criteria

- **AC-1** The synthetic check remains runnable.
- **AC-2** A second synthetic property holds.

## Open Decisions

None.
'''

PLAN = '''# PLAN

## 조건표

| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | maintain | `true` | — | T001 |
| AC-2 | maintain | `true` | — | T002 |
| EX-1 | maintain | `true` | — | T001 |

## 범위

- 바꿀 수 있는 경로: `src/**`
- 테스트 경로: (기본값)
- 유지할 동작: synthetic

## Task

| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | Synthetic one | AC-1, EX-1 | — | — | 예 | — |
| T002 | Synthetic two | AC-2 | — | — | 예 | — |

## 예산

- 검증 실행: 40
- 경과 시간: 4시간
- 비용: 미관측
'''


class Fixture:
    """A synthetic repository with one started goal and a committed bundle."""

    def __init__(self, binary, root, goal='docs/specs/synthetic', start=True):
        self.binary, self.root, self.goal = binary, root, goal
        self.env = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1', LC_ALL='C',
                        GIT_AUTHOR_NAME='synthetic', GIT_COMMITTER_NAME='synthetic',
                        GIT_AUTHOR_EMAIL='synthetic@example.invalid', GIT_COMMITTER_EMAIL='synthetic@example.invalid')
        for name in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_COMMON_DIR'):
            self.env.pop(name, None)
        g = root / goal
        g.mkdir(parents=True)
        (root / 'src').mkdir(exist_ok=True)
        (root / 'src/value').write_text('stable\n')
        (g / 'SPEC.md').write_text(SPEC)
        (g / 'PLAN.md').write_text(PLAN)
        (g / 'REVIEW.md').write_text('# REVIEW\n\n## 결론\n\nready\n')
        (g / 'PROGRESS.md').write_text('# PROGRESS\n\n## Task 상태\n\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | todo | |\n| T002 | todo | |\n')
        self.git('init', '-q', '-b', 'main')
        self.commit('base')
        if start:
            self.ok('start', goal, '--request', 'Synthetic fixture', '--skill-name', 'synthetic', '--skill-version', '1')
            self.ok('check', goal, 'AC-1')
            self.commit('bundle')

    def git(self, *args, cwd=None):
        r = run(['git', *args], cwd or self.root, self.env)
        if r.returncode:
            raise Environment('setup_error', 'git %s: %s' % (' '.join(args), r.stderr))
        return r.stdout

    def commit(self, message, cwd=None):
        self.git('add', '-A', cwd=cwd)
        self.git('commit', '-q', '--allow-empty', '-m', message, cwd=cwd)

    def cli(self, *args, cwd=None, binary=None, env=None):
        return run([binary or self.binary, *args], cwd or self.root, env or self.env)

    def ok(self, *args, cwd=None):
        r = self.cli(*args, cwd=cwd)
        if r.returncode:
            raise Environment('setup_error', 'ha %s: %s%s' % (' '.join(args), r.stdout, r.stderr))
        return r

    def h(self, *args, cwd=None, env=None):
        return self.cli('history', self.goal, *args, cwd=cwd, env=env)

    def add(self, *args, cwd=None, expect=0, code=None):
        r = self.h('add', *args, cwd=cwd)
        if expect == 0:
            require(r.returncode == 0, 'history-registration-refused', r.stdout + r.stderr)
            try:
                return json.loads(r.stdout)
            except ValueError as error:
                raise Violation('history-receipt-invalid', r.stdout) from error
        require(r.returncode == expect, 'history-invalid-registration-accepted', '%s -> %d %s %s' % (args, r.returncode, r.stdout, r.stderr))
        if code:
            require(code_of(r) == code, 'history-error-cause-conflated', 'want %s: %s' % (code, r.stderr))
        return r

    def view(self, *args, cwd=None):
        r = self.h(*args, '--format', 'json', cwd=cwd)
        require(r.returncode == 0, 'history-query-refused', r.stdout + r.stderr)
        try:
            return json.loads(r.stdout)
        except ValueError as error:
            raise Violation('history-view-invalid', r.stdout) from error

    def md(self, *args, cwd=None):
        r = self.h(*args, cwd=cwd)
        require(r.returncode == 0, 'history-query-refused', r.stdout + r.stderr)
        return r.stdout

    def worktree(self, name, base=None):
        path = self.root.parent / (self.root.name + '-' + name)
        self.git('worktree', 'add', '-q', '-b', name, str(path), *([base] if base else []))
        return path

    def runs(self, cwd=None):
        path = (cwd or self.root) / self.goal / 'runs.jsonl'
        return path.read_bytes() if path.exists() else None

    def start_sha(self, cwd=None):
        return hashlib.sha256(self.runs(cwd).split(b'\n')[0]).hexdigest()

    def common(self, cwd=None):
        out = self.git('rev-parse', '--git-common-dir', cwd=cwd).strip()
        path = Path(out)
        return (path if path.is_absolute() else (cwd or self.root) / path).resolve()

    def history_dir(self, cwd=None):
        return self.common(cwd) / 'ha' / 'history' / self.start_sha(cwd)

    def history_bytes(self, cwd=None):
        p = self.history_dir(cwd) / 'events.jsonl'
        return p.read_bytes() if p.exists() else None

    def tree(self, cwd=None):
        return self.git('status', '--porcelain', '--untracked-files=all', cwd=cwd)

    def snapshot(self, cwd=None):
        files = {}
        base = self.common(cwd) / 'ha'
        if base.is_dir():
            for p in sorted(base.rglob('*')):
                if p.is_file() and not p.is_symlink():
                    files[str(p.relative_to(base))] = p.read_bytes()
        return {'runs': self.runs(cwd), 'tree': self.tree(cwd), 'ha': files}

    def status(self, cwd=None):
        r = self.cli('status', self.goal, '--format', 'json', cwd=cwd)
        try:
            return r.returncode, json.loads(r.stdout)
        except ValueError as error:
            raise Violation('history-status-unreadable', r.stdout + r.stderr) from error


def ids(items):
    return [i['id'] if i['type'] == 'event' else i['id'] + '>' + i['by'] for i in items]


def item(view, event_id):
    for i in view['items']:
        if i['type'] == 'event' and i['id'] == event_id:
            return i
    return None


# --- criteria ---------------------------------------------------------------

def ac1(binary, temp):
    early = Fixture(binary, temp / 'early', start=False)
    before = early.snapshot()
    r = early.add('--kind', '관측', '--summary', 'too early', expect=65, code='not_started')
    require(early.snapshot() == before and not (early.root / early.goal / 'runs.jsonl').exists(),
            'history-started-a-run', 'registration without start changed files')
    f = Fixture(binary, temp / 'main')
    other = f.history_dir().parent / ('f' * 64)
    other.mkdir(parents=True)
    (other / 'events.jsonl').write_bytes(b'unrelated\n')
    before = f.snapshot()
    receipt = f.add('--kind', '원인', '--status', '가설', '--summary', 'first item assumed', '--target', 'AC-1', '--ref', 'run:2', '--request-id', 'one')
    require(receipt.get('schema') == 'decision-receipt/v1' and receipt['id'] == 'D1' and receipt['seq'] == 1 and not receipt['replayed'],
            'history-receipt-invalid', receipt)
    after = f.snapshot()
    require(after['runs'] == before['runs'] and after['tree'] == before['tree'], 'history-changed-run-or-tree', 'run record or working tree changed')
    added = set(after['ha']) - set(before['ha'])
    rel = str(f.history_dir().relative_to(f.common() / 'ha'))
    require(added <= {rel + '/events.jsonl', rel + '/lock', rel + '/last'} and {rel + '/events.jsonl', rel + '/last'} <= added,
            'history-stored-elsewhere', sorted(added))
    require(all(after['ha'][k] == v for k, v in before['ha'].items()), 'history-changed-other-data', 'existing local data changed')
    lines = f.history_bytes().splitlines()
    require(len(lines) == 1 and hashlib.sha256(lines[0]).hexdigest() == receipt['sha256'], 'history-receipt-mismatch', lines)
    view = f.view('show', 'D1')
    e = view['event']
    runs = f.runs().splitlines()
    head = {'seq': len(runs), 'sha256': hashlib.sha256(runs[-1]).hexdigest()}
    require(e['id'] == 'D1' and e['summary'] == 'first item assumed' and e['kind'] == '원인' and e['status'] == '가설'
            and e['targets'] == ['AC-1'] and e['recorded_context']['run_head'] == head
            and e['recorded_context']['commit'] == f.git('rev-parse', 'HEAD').strip()
            and e['evidence'][0]['recorded']['version'] == hashlib.sha256(runs[1]).hexdigest(),
            'history-reread-differs', e)
    listed = f.view('list')
    require(ids(listed['items']) == ['D1'] and listed['items'][0]['summary'] == 'first item assumed', 'history-reread-differs', listed)
    # A structured input file is an alternative path, not a requirement.
    data = temp / 'input.json'
    data.write_text(json.dumps({'request_id': 'two', 'decision': {'kind': '결정', 'summary': 'from a file', 'targets': ['T001'], 'builds_on': ['D1']}}))
    require(f.add('--input', str(data))['id'] == 'D2', 'history-input-path-refused', 'input file')


def ac2(binary, temp):
    f = Fixture(binary, temp / 'main')
    head_sha = f.git('rev-parse', 'HEAD').strip()
    f.add('--kind', '원인', '--status', '가설', '--summary', 'hypothesis', '--reason', 'line one\nline two', '--target', 'AC-1', '--target', 'T001',
          '--ref', 'run:2', '--checked-ref', 'file:src/value', '--ref', 'file:missing/file.md', '--ref', 'commit:' + head_sha[:10],
          '--ref', 'commit:' + '0' * 40, '--ref', 'commit:abcdef1@github.com/example/other', '--ref', 'external:https://example.invalid/review/1',
          '--event-at', '2026-01-01T09:00:00+09:00', '--request-id', 'hypothesis')
    f.add('--kind', '결정', '--summary', 'plan change: drop the parser rewrite', '--target', 'T001', '--checked-ref', 'file:' + f.goal + '/PLAN.md#조건표', '--builds-on', 'D1')
    data = temp / 'external.json'
    data.write_text(json.dumps({'decision': {'kind': '검증', 'status': '통과', 'summary': 'claimed pass', 'targets': ['AC-2'],
                                             'evidence': [{'type': 'external', 'ref': 'https://example.invalid/ci/7', 'version': 'run-7', 'checked': True}]}}))
    f.add('--input', str(data))
    f.add('--kind', '관측', '--summary', 'no claim state', '--supersedes', 'D1')
    e = f.view('show', 'D1')['event']
    for key in ('kind', 'status', 'summary', 'reason', 'targets', 'recorded_at', 'event_at', 'evidence', 'recorded_context', 'superseded_by'):
        require(key in e, 'history-fields-indistinct', 'missing ' + key)
    require(e['event_at'] == '2026-01-01T09:00:00+09:00' and e['recorded_at'] != e['event_at'] and e['recorded_at'].endswith('Z'),
            'history-times-conflated', e)
    ev = {(x['type'], x.get('path') or x.get('commit') or x.get('ref') or str(x.get('seq'))): x for x in e['evidence']}
    run_ref = ev[('run', '2')]
    require(run_ref['seq'] == 2 and run_ref['recorded']['version'] == hashlib.sha256(f.runs().splitlines()[1]).hexdigest()
            and run_ref['recorded']['state'] == 'matched' and not run_ref.get('checked'), 'history-run-reference-conflated', run_ref)
    checked = ev[('file', 'src/value')]
    require(checked['checked'] is True and checked['recorded']['version'].startswith('sha256:') and checked['recorded']['state'] == 'read',
            'history-file-reference-conflated', checked)
    missing = ev[('file', 'missing/file.md')]
    require(not missing.get('checked') and missing['recorded'] == {'version': '', 'state': 'missing'}, 'history-unidentified-version-claimed', missing)
    found = ev[('commit', head_sha[:10])]
    require(found['recorded']['state'] == 'found' and found['recorded']['version'] == head_sha, 'history-commit-reference-conflated', found)
    require(ev[('commit', '0' * 40)]['recorded']['state'] == 'not_found', 'history-commit-reference-conflated', ev)
    require(ev[('commit', 'abcdef1')]['recorded']['state'] == 'other_repository', 'history-commit-reference-conflated', ev)
    require(ev[('external', 'https://example.invalid/review/1')]['recorded'] == {'version': '', 'state': 'not_checked'}, 'history-external-reference-conflated', ev)
    plan = f.view('show', 'D2')['event']['evidence'][0]
    require(plan['section'] == '조건표' and plan['recorded']['state'] == 'read' and plan['recorded']['version'].startswith('sha256:'),
            'history-start-documents-unlinked', plan)
    claimed = f.view('show', 'D3')['event']
    require(claimed['evidence'][0]['version'] == 'run-7' and claimed['evidence'][0]['checked'] is True, 'history-external-version-lost', claimed)
    plain = f.view('show', 'D4')['event']
    require('status' not in plain and 'event_at' not in plain, 'history-claim-state-inferred', plain)
    md = f.md('show', 'D4')
    require('### D4 · 관측\n' in md and 'event time: unknown' in md, 'history-claim-state-inferred', md)
    require(f.md('show', 'D1').count('content not checked') >= 3, 'history-checked-claim-conflated', 'unchecked references')
    # A claimed pass, a reference, or a correction proves nothing to the run state.
    code, state = f.status()
    require(code == 1 and any(c['id'] == 'AC-2' and not c['satisfied'] for c in state['criteria']), 'history-claim-promoted', state)
    before = f.history_bytes()
    for args, err in ((['--kind', 'plan', '--summary', 'x'], 'invalid_input'), (['--kind', '결정', '--status', '확정', '--summary', 'x'], 'invalid_input'),
                      (['--kind', '원인', '--status', '통과', '--summary', 'x'], 'invalid_input'), (['--kind', '관측', '--summary', 'x', '--ref', 'url:x'], 'invalid_input'),
                      (['--kind', '관측', '--summary', 'x', '--ref', 'file:../outside'], 'invalid_input')):
        f.add(*args, expect=64, code=err)
    bad = temp / 'unknown.json'
    bad.write_text(json.dumps({'decision': {'kind': '관측', 'summary': 'x', 'verified': True}}))
    f.add('--input', str(bad), expect=64, code='invalid_input')
    require(f.history_bytes() == before, 'history-rejected-input-stored', 'history changed')
    # Only a path that does not exist is missing. One whose parents cannot be
    # traversed or resolved is unreadable, and the stored record never changes.
    locked = f.root / 'src' / 'locked'
    (locked / 'sub').mkdir(parents=True)
    (locked / 'sub' / 'b.md').write_text('b\n')
    os.symlink('loop', f.root / 'src' / 'loop')
    want = {'src/none/deeper.md': 'missing', 'src/value/under.md': 'missing', 'src/loop/c.md': 'unreadable'}
    if os.geteuid() != 0:
        want['src/locked/sub/b.md'] = 'unreadable'
    os.chmod(locked, 0)
    try:
        r = f.add('--kind', '관측', '--summary', 'paths', *[x for p in want for x in ('--ref', 'file:' + p)])
    finally:
        os.chmod(locked, 0o755)
    states = {e['path']: e['recorded']['state'] for e in f.view('show', r['id'])['event']['evidence']}
    require(states == want, 'history-unknown-path-recorded-missing', states)
    evidence_read_boundaries(binary, temp / 'read-boundaries')
    unit_observations('./cmd/ha', FILE_BOUNDARY_TESTS)
    unit_observations('./internal/history', {'TestFileEvidenceObservationBoundaries'})


def evidence_read_boundaries(binary, temp):
    f = Fixture(binary, temp / 'repo')
    bounded = f.root / 'src/bounded'
    with bounded.open('wb') as out:
        out.truncate(64 * 1024 * 1024)
    exact = f.add('--kind', '관측', '--summary', 'exact limit', '--ref', 'file:src/bounded', '--request-id', 'exact')
    recorded = f.view('show', exact['id'])['event']['evidence'][0]['recorded']
    require(recorded['state'] == 'read' and recorded['version'].startswith('sha256:'),
            'history-file-limit-boundary', recorded)
    with bounded.open('ab') as out:
        out.write(b'x')
    oversized = f.add('--kind', '관측', '--summary', 'over limit', '--ref', 'file:src/bounded')
    over = f.view('show', oversized['id'])['event']['evidence'][0]
    require(over['recorded'] == {'version': '', 'state': 'too_large'} and over['now'] == 'unidentified',
            'history-file-limit-boundary', over)
    changed = f.view('show', exact['id'])['event']['evidence'][0]
    require(changed['recorded'] == recorded and changed['now'] == 'unavailable',
            'history-file-observation-rewritten', changed)
    before = f.history_bytes()
    bounded.unlink()
    os.mkfifo(bounded)
    replay = f.add('--kind', '관측', '--summary', 'exact limit', '--ref', 'file:src/bounded', '--request-id', 'exact')
    require(replay['replayed'] and replay['id'] == exact['id'] and f.history_bytes() == before,
            'history-replay-rereads-file', replay)
    for query in ([], ['current'], ['list']):
        require(f.view(*query)['total'] == 2, 'history-summary-rereads-file', query)
    outside = temp / 'outside'
    outside.mkdir()
    (outside / 'value').write_text('outside content')
    os.symlink(outside, f.root / 'src/outside')
    os.symlink(outside / 'value', f.root / 'src/link')
    expected = {'src/bounded': 'not_regular', 'src/link': 'not_regular', 'src/outside/value': 'outside', 'src': 'not_regular'}
    r = f.add('--kind', '관측', '--summary', 'unsafe files', *[x for path in expected for x in ('--ref', 'file:' + path)])
    states = {e['path']: e['recorded'] for e in f.view('show', r['id'])['event']['evidence']}
    require(states == {p: {'version': '', 'state': state} for p, state in expected.items()},
            'history-file-boundary-escaped', states)


def ac3(binary, temp):
    f = Fixture(binary, temp / 'main')
    wt = f.worktree('peer')
    a = f.add('--kind', '원인', '--status', '가설', '--summary', 'hypothesis A', '--target', 'AC-1', '--ref', 'run:2', '--request-id', 'a')
    original = f.history_bytes()
    f.add('--kind', '결정', '--summary', 'decision on A', '--builds-on', 'D1')
    f.add('--kind', '조치', '--status', '예정', '--summary', 'second use of A', '--builds-on', 'D1')
    f.add('--kind', '원인', '--status', '기각', '--summary', 'A rejected', '--supersedes', 'D1')
    require(f.history_bytes().startswith(original), 'history-original-rewritten', 'D1 bytes changed')
    succ = f.view('show', 'D1')
    rel = sorted((s['relation'], s['id']) for s in succ['successors'])
    require(rel == [('builds_on', 'D2'), ('builds_on', 'D3'), ('supersedes', 'D4')], 'history-relation-untraceable', succ['successors'])
    pred = f.view('show', 'D4')['predecessors']
    require([(p['relation'], p['id']) for p in pred] == [('supersedes', 'D1')], 'history-relation-untraceable', pred)
    require(f.view('show', 'D1')['event']['summary'] == 'hypothesis A', 'history-original-rewritten', 'D1 content')
    # A later builds_on of a corrected event is not a conflict.
    f.add('--kind', '관측', '--summary', 'still cites A', '--builds-on', 'D1')
    before = f.history_bytes()
    for args in (['--builds-on', 'D6'], ['--builds-on', 'D99'], ['--supersedes', 'D6'], ['--supersedes', 'D1@D9']):
        f.add('--kind', '결정', '--summary', 'x', *args, expect=65, code='invalid_relation')
    f.add('--kind', '결정', '--summary', 'x', '--builds-on', 'D0', expect=64, code='invalid_input')
    other = Fixture(binary, temp / 'other')
    for _ in range(7):
        other.add('--kind', '관측', '--summary', 'other history')
    f.add('--kind', '결정', '--summary', 'x', '--builds-on', 'D7', expect=65, code='invalid_relation')
    require(f.history_bytes() == before, 'history-invalid-relation-stored', 'history changed')
    # Replay and request conflicts.
    again = f.add('--kind', '원인', '--status', '가설', '--summary', 'hypothesis A', '--target', 'AC-1', '--ref', 'run:2', '--request-id', 'a')
    require(again['replayed'] and again['seq'] == a['seq'] and again['sha256'] == a['sha256'] and f.history_bytes() == before,
            'history-replay-duplicated', again)
    f.add('--kind', '원인', '--summary', 'different', '--request-id', 'a', expect=65, code='request_conflict')
    # Independent registrations from the main checkout and a linked worktree,
    # whose run-record locks differ, all survive intact.
    def register(arg):
        where, n = arg
        return f.h('add', '--kind', '관측', '--summary', 'parallel %s %d' % (where.name, n), cwd=where)
    jobs = [(where, n) for n in range(8) for where in (f.root, wt)]
    with ThreadPoolExecutor(max_workers=16) as pool:
        results = list(pool.map(register, jobs))
    require(all(r.returncode == 0 for r in results), 'history-concurrent-registration-lost', [r.stderr for r in results if r.returncode])
    events = [json.loads(line) for line in f.history_bytes().splitlines()]
    require(len(events) == 5 + 16 and [e['seq'] for e in events] == list(range(1, 22)), 'history-concurrent-registration-lost', len(events))
    summaries = {e['decision']['summary'] for e in events}
    require(all('parallel %s %d' % (w.name, n) in summaries for w, n in jobs), 'history-concurrent-registration-lost', 'missing summaries')
    require(f.view('list', '--limit', '200')['total'] == 21, 'history-concurrent-registration-lost', 'list')
    # Conflicting corrections of one judgment: one is stored, the other surfaces.
    def correct(where):
        return f.h('add', '--kind', '원인', '--summary', 'correction from ' + where.name, '--supersedes', 'D2', cwd=where)
    with ThreadPoolExecutor(max_workers=2) as pool:
        results = list(pool.map(correct, (f.root, wt)))
    require(sorted(r.returncode for r in results) == [0, 65] and any(code_of(r) == 'correction_conflict' for r in results),
            'history-correction-overwritten', [(r.returncode, r.stderr) for r in results])


def diverged(binary, temp):
    """Main and a peer worktree share seq 1-2, then each records its own seq 3."""
    f = Fixture(binary, temp / 'main')
    wt = f.worktree('peer')
    (wt / 'src/peer').write_text('peer\n')
    f.commit('peer change', cwd=wt)
    f.ok('check', f.goal, 'AC-1')
    f.ok('check', f.goal, 'AC-1', cwd=wt)
    require(f.runs().splitlines()[2] != f.runs(wt).splitlines()[2], 'history-lineage-fixture', 'seq 3 did not diverge')
    return f, wt


def ac4(binary, temp):
    f = Fixture(binary, temp / 'main')
    f.add('--kind', '원인', '--status', '가설', '--summary', 'PDF first item assumed', '--target', 'T001')
    f.add('--kind', '관측', '--summary', 'empty input reproduces', '--target', 'AC-1')
    f.add('--kind', '결정', '--summary', 'return zero', '--target', 'T001', '--builds-on', 'D1', '--builds-on', 'D2')
    f.add('--kind', '원인', '--status', '확정', '--summary', 'OCR path is the cause', '--target', 'T001', '--supersedes', 'D1')
    view = f.view('current')
    require(ids(view['items']) == ['D2', 'D3', 'D4'], 'history-current-is-latest-event', view['items'])
    d3 = item(view, 'D3')
    require(d3['current'] and d3['grounds_corrected'] == [{'id': 'D1', 'by': ['D4']}], 'history-grounds-correction-hidden', d3)
    require(ids(f.view('current', '--keyword', 'PDF')['items']) == [] and 'D1' not in ids(f.view('current', '--target', 'T001')['items']),
            'history-filter-resurrected', 'D1')
    md = f.md('current')
    require('linked grounds D1 corrected by D4' in md and '- D1 ·' not in md, 'history-grounds-correction-hidden', md)
    for word in ('unresolved', 'not done', 'unfinished', 'stale judgment', 'verified'):
        require(word not in md.lower(), 'history-absence-judged', word)
    # Head advance alone keeps the lineage; the recorded snapshot stays.
    before = f.view('show', 'D2')['event']['recorded_context']
    f.ok('check', f.goal, 'AC-1')
    after = f.view('show', 'D2')['event']
    require(after['lineage'] == 'same' and after['current'] and after['recorded_context'] == before, 'history-head-advance-reclassified', after)
    # Uncommitted code differs from the same commit.
    (f.root / 'src/value').write_text('dirty\n')
    f.add('--kind', '관측', '--summary', 'seen on a dirty tree', '--target', 'AC-1')
    dirty = item(f.view('current'), 'D5')
    require(dirty['context']['code'] == 'uncommitted' and f.view('show', 'D5')['event']['recorded_context']['tree_clean'] is False,
            'history-uncommitted-context-merged', dirty)
    (f.root / 'src/value').write_text('stable\n')
    # Diverged run chains.
    g, wt = diverged(binary, temp / 'lineage')
    g.add('--kind', '결정', '--summary', 'shared judgment', '--target', 'T001')
    g.add('--kind', '결정', '--summary', 'peer correction', '--supersedes', 'D1', cwd=wt)
    g.add('--kind', '관측', '--summary', 'peer only', cwd=wt)
    g.ok('check', g.goal, 'AC-1', cwd=wt)
    g.add('--kind', '관측', '--summary', 'peer ahead', cwd=wt)
    main = g.view('current')
    require(ids(main['items']) == ['D1'], 'history-lineage-merged', main['items'])
    d1 = item(main, 'D1')
    require(d1['other_corrections'] == ['D2'] and not d1.get('superseded_by'), 'history-other-lineage-applied', d1)
    others = {o['id']: o['lineage'] for o in main['other_lineage']['ids']}
    require(others == {'D2': 'different', 'D3': 'different', 'D4': 'unverifiable'}, 'history-lineage-merged', others)
    peer = g.view('current', cwd=wt)
    require(ids(peer['items']) == ['D2', 'D3', 'D4'], 'history-lineage-merged', peer['items'])
    # A broken run chain cannot support current judgments.
    runs = g.root / g.goal / 'runs.jsonl'
    data = runs.read_bytes()
    runs.write_bytes(data.replace(b'"seq":2', b'"seq":9', 1))
    broken = g.view('current')
    require(broken['run_chain'] == 'broken' and broken['items'] == [] and broken['other_lineage']['count'] == 4
            and all(o['lineage'] == 'unverifiable' for o in broken['other_lineage']['ids']), 'history-broken-chain-trusted', broken)
    runs.write_bytes(data)
    neutral_grounds(binary, temp / 'neutral')
    unit_observations('./internal/history', {'TestGroundsCorrectionsPreserveEarlierSameAndLaterEvents',
                                           'TestGroundsCorrectionsRespectSelectedLineage',
                                           'TestGroundsCorrectionPagesKeepOriginalBoundary'})


def neutral_grounds(binary, temp):
    f = Fixture(binary, temp / 'repo')
    f.add('--kind', '관측', '--summary', 'original')
    f.add('--kind', '관측', '--summary', 'earlier correction', '--supersedes', 'D1')
    f.add('--kind', '결정', '--summary', 'keep me', '--target', 'T001', '--builds-on', 'D1', '--request-id', 'uses-old')
    f.add('--kind', '관측', '--summary', 'uses and corrects', '--builds-on', 'D1', '--supersedes', 'D1@D2')
    f.add('--kind', '관측', '--summary', 'later correction', '--supersedes', 'D1@D4')
    expected = [{'id': 'D1', 'by': ['D2', 'D4', 'D5']}]
    before = f.snapshot()
    queries = ([], ['current'], ['list'], ['current', '--target', 'T001', '--keyword', 'keep'],
               ['show', 'D3'], ['show', '--request-id', 'uses-old'], ['show', 'D4'])
    for query in queries:
        view = f.view(*query)
        event = view['event'] if query and query[0] == 'show' else item(view, 'D3')
        require(event['current'] and event['grounds_corrected'] == expected, 'history-correction-information-lost', event)
        md = f.md(*query)
        require('linked grounds D1 corrected by D2, D4, D5' in md and 'later corrected' not in md,
                'history-correction-time-misreported', md)
    require(f.snapshot() == before, 'history-query-wrote', 'neutral display changed storage')


def ac5(binary, temp):
    f = Fixture(binary, temp / 'main')
    long_reason = 'detail ' * 200
    f.add('--kind', '원인', '--status', '가설', '--summary', 'PDF parser assumption', '--reason', long_reason, '--target', 'T001', '--target', 'AC-2')
    for n in range(4):
        f.add('--kind', '관측', '--summary', 'observation %d' % n, '--target', 'AC-1', '--builds-on', 'D1')
    listing = f.view('list')
    for i in listing['items']:
        require(not {'reason', 'evidence', 'builds_on', 'supersedes', 'recorded_context'} & set(i), 'history-list-not-brief', i)
        require({'id', 'kind', 'summary', 'lineage'} <= set(i), 'history-list-incomplete', i)
    require(long_reason.strip() not in f.md('list'), 'history-list-not-brief', 'reason in list')
    detail = f.view('show', 'D1', '--limit', '2')
    require(detail['event']['reason'] == long_reason and [s['id'] for s in detail['successors']] == ['D2', 'D3'] and detail['truncated'] and detail['total'] == 4,
            'history-detail-unbounded', detail)
    rest = f.view('show', 'D1', '--limit', '2', '--page', detail['next'])
    require([s['id'] for s in rest['successors']] == ['D4', 'D5'] and not rest['truncated'], 'history-page-gap', rest)
    require(all('summary' in s and 'reason' not in s for s in detail['successors']), 'history-detail-unbounded', detail['successors'])
    # Filters and stable order.
    require(ids(f.view('current', '--target', 'AC-1')['items']) == ['D2', 'D3', 'D4', 'D5'], 'history-filter-wrong', 'target')
    require(ids(f.view('current', '--keyword', 'pdf')['items']) == ['D1'], 'history-filter-wrong', 'keyword')
    a, b = f.h('current', '--format', 'json'), f.h('current', '--format', 'json')
    require(a.stdout == b.stdout and f.md('current') == f.md('current'), 'history-query-unstable', 'repeat')
    # Changes after a cursor announce replacements outside the filters.
    cursor = f.view('current', '--target', 'AC-2')['cursor']
    f.add('--kind', '원인', '--status', '확정', '--summary', 'OCR path', '--target', 'T001', '--supersedes', 'D1')
    changes = f.view('current', '--target', 'AC-2', '--since', cursor)
    require(ids(changes['items']) == ['D1>D6'], 'history-replacement-unannounced', changes['items'])
    require(f.view('current', '--target', 'AC-2')['items'] == [], 'history-filter-resurrected', 'plain current')
    require('replacement: D1 → D6' in f.md('current', '--target', 'AC-2', '--since', cursor), 'history-replacement-unannounced', 'markdown')
    listed = f.view('list', '--since', cursor)
    require(ids(listed['items']) == ['D6', 'D1>D6'], 'history-changes-wrong', listed['items'])
    # Pages keep their boundary across a later correction.
    first = f.view('current', '--limit', '2')
    f.add('--kind', '관측', '--summary', 'replaces observation 2', '--supersedes', 'D4')
    second = f.view('current', '--limit', '2', '--page', first['next'])
    third = f.view('current', '--limit', '2', '--page', second['next'])
    require(ids(first['items']) + ids(second['items']) + ids(third['items']) == ['D2', 'D3', 'D4', 'D5', 'D6'] and third['next'] is None,
            'history-page-gap', [first['items'], second['items'], third['items']])
    for args, err in ((['current', '--keyword', 'x', '--page', first['next']], 'cursor_mismatch'),
                      (['current', '--page', first['next'].replace(first['next'].split('.')[1], 'f' * 16)], 'cursor_invalid'),
                      (['current', '--since', 'c1.bad'], 'cursor_invalid')):
        r = f.h(*args)
        require(r.returncode == 65 and code_of(r) == err, 'history-cursor-accepted', (args, r.returncode, r.stderr))
    g, wt = diverged(binary, temp / 'lineage')
    g.add('--kind', '관측', '--summary', 'one')
    g.add('--kind', '관측', '--summary', 'two')
    page = g.view('current', '--limit', '1')
    r = g.h('current', '--limit', '1', '--page', page['next'], cwd=wt)
    require(r.returncode == 65 and code_of(r) == 'cursor_stale', 'history-cursor-accepted', r.stderr)
    # Markdown and JSON carry the same items; queries write nothing.
    md = f.md('list')
    for i in f.view('list')['items']:
        require('- %s · ' % i['id'] in md and i['summary'] in md, 'history-formats-diverge', i['id'])
    # Executor-given versions that share a prefix stay distinct in Markdown.
    data = temp / 'external.json'
    data.write_text(json.dumps({'decision': {'kind': '검증', 'summary': 'external builds', 'evidence': [
        {'type': 'external', 'ref': 'https://example.invalid/ci/1', 'version': 'release-2026-10-14-rc1'},
        {'type': 'external', 'ref': 'https://example.invalid/ci/2', 'version': 'release-2026-10-14-rc2'}]}}))
    ext = f.add('--input', str(data))['id']
    versions = [e['recorded']['version'] for e in f.view('show', ext)['event']['evidence']]
    shown = f.md('show', ext)
    require(versions == ['release-2026-10-14-rc1', 'release-2026-10-14-rc2'] and all('version %s ' % v in shown for v in versions),
            'history-formats-diverge', shown)
    # A correction's acknowledged last correction shows in Markdown as in JSON.
    a = Fixture(binary, temp / 'after')
    a.add('--kind', '원인', '--summary', 'first')
    a.add('--kind', '원인', '--summary', 'fix', '--supersedes', 'D1')
    a.add('--kind', '원인', '--summary', 'second fix', '--supersedes', 'D1@D2')
    pred = a.view('show', 'D3')['predecessors']
    md3, md1 = a.md('show', 'D3'), a.md('show', 'D1')
    require([(p['relation'], p['id'], p.get('after')) for p in pred] == [('supersedes', 'D1', 'D2')]
            and '- supersedes D1 · 원인 · first (lineage same, after D2)' in md3
            and '- supersedes D3 · 원인 · second fix (lineage same, current, after D2)' in md1, 'history-formats-diverge', md3 + md1)
    before = f.snapshot()
    for args in (['current'], ['list', '--since', cursor], ['show', 'D1'], ['current', '--format', 'json']):
        f.h(*args)
    require(f.snapshot() == before, 'history-query-wrote', 'files changed')
    absent = Fixture(binary, temp / 'absent')
    before = absent.snapshot()
    view = absent.view('current')
    require(view['history'] == 'absent' and absent.snapshot() == before and not absent.history_dir().exists(), 'history-query-wrote', view)


def ac6(binary, temp):
    unsupported_history_rules(binary, temp / 'unsupported-rules')
    f = Fixture(binary, temp / 'main')
    f.add('--kind', '관측', '--summary', 'first')
    before = f.history_bytes()
    cases = [
        (['--kind', '관측', '--summary', 'x\ny'], 64, 'invalid_input'),
        (['--kind', '관측', '--summary', 'x' * 1025], 64, 'limit_exceeded'),
        (['--kind', '관측', '--summary', 'x', '--reason', 'r' * (16 * 1024 + 1)], 64, 'limit_exceeded'),
        (['--kind', '관측', '--summary', 'x', '--target', 'AC-9'], 65, 'invalid_reference'),
        (['--kind', '관측', '--summary', 'x', '--ref', 'run:99'], 65, 'invalid_reference'),
        (['--kind', '관측', '--summary', 'x', '--builds-on', 'D5'], 65, 'invalid_relation'),
        (['--kind', '관측', '--summary', 'x', '--request-id', '../escape'], 64, 'invalid_input'),
        (['--kind', '관측', '--summary', 'x', '--request-id', ''], 64, 'invalid_input'),
    ]
    for args, exit_code, err in cases:
        f.add(*args, expect=exit_code, code=err)
    for empty_request in ('', None):
        empty_input = temp / 'empty-request.json'
        empty_input.write_text(json.dumps({'request_id': empty_request,
                                          'decision': {'kind': '관측', 'summary': 'empty id'}}))
        f.add('--input', str(empty_input), expect=64, code='invalid_input')
    for args in (['show', 'D0'], ['show', 'D1\nforged'], ['show', '--request-id='],
                 ['show', '--request-id', '../escape'], ['show', '--request-id', 'bad\nforged']):
        result = f.h(*args)
        require(result.returncode == 64 and code_of(result) == 'invalid_input'
                and len(result.stderr.splitlines()) == 1,
                'history-invalid-identifier-accepted', (args, result.returncode, result.stderr))
    result = f.h('show', 'D999')
    require(result.returncode == 65 and code_of(result) == 'unknown_event',
            'history-storage-states-conflated', result.stderr)
    require(f.history_bytes() == before, 'history-rejected-input-stored', 'query changed history')
    f.add('--kind', '관측', '--summary', 'fix', '--supersedes', 'D1')
    before = f.history_bytes()
    f.add('--kind', '관측', '--summary', 'late', '--supersedes', 'D1', expect=65, code='correction_conflict')
    require(f.history_bytes() == before, 'history-rejected-input-stored', 'history changed')
    # Stored states are distinct and never an empty success.
    events = f.history_dir() / 'events.jsonl'
    good = events.read_bytes()
    states = {}
    for name, data, mode in (('corrupt', good[:-4], 0o644), ('edited', good.replace(b'first', b'fiRst'), 0o644),
                             ('missing-schema', good.replace(b'"schema":"decision-history/v1",', b'', 1), 0o644),
                             ('null-schema', good.replace(b'"schema":"decision-history/v1"', b'"schema":null', 1), 0o644),
                             ('empty-schema', good.replace(b'"schema":"decision-history/v1"', b'"schema":""', 1), 0o644),
                             ('duplicate-schema', good.replace(b'"schema":"decision-history/v1"', b'"schema":null,"schema":"decision-history/v9"', 1), 0o644),
                             ('cased-schema', good.replace(b'"schema":"decision-history/v1"', b'"Schema":"decision-history/v9"', 1), 0o644),
                             ('unsupported', good + b'{"schema":"decision-history/v9"}\n', 0o644), ('unreadable', good, 0o000)):
        events.write_bytes(data)
        os.chmod(events, mode)
        q, a = f.h('current'), f.h('add', '--kind', '관측', '--summary', 'after ' + name)
        os.chmod(events, 0o644)
        require(events.read_bytes() == data, 'history-damage-repaired', name)
        states[name] = (q.returncode, code_of(q), a.returncode, code_of(a))
    events.write_bytes(good)
    want = {'corrupt': (65, 'history_corrupt', 65, 'history_corrupt'), 'edited': (65, 'history_corrupt', 65, 'history_corrupt'),
            'unsupported': (66, 'history_unsupported', 66, 'history_unsupported'), 'unreadable': (70, 'history_unreadable', 70, 'history_unreadable')}
    for name in ('missing-schema', 'null-schema', 'empty-schema', 'duplicate-schema', 'cased-schema'):
        want[name] = (65, 'history_corrupt', 65, 'history_corrupt')
    if os.geteuid() == 0:
        want.pop('unreadable')
        states.pop('unreadable')
    require(states == want, 'history-storage-states-conflated', states)
    absent = Fixture(binary, temp / 'absent')
    md = absent.md('current')
    require('history: absent' in md and 'does not show that no event was ever recorded' in md, 'history-absence-overclaimed', md)
    # A stored request can be confirmed by its id after an uncertain write.
    receipt = f.add('--kind', '검증', '--status', '통과', '--summary', 'confirm me', '--request-id', 'confirm-1')
    shown = f.view('show', '--request-id', 'confirm-1')
    require(shown['event']['id'] == receipt['id'], 'history-request-unconfirmable', shown)
    r = f.h('show', '--request-id', 'never-sent')
    require(r.returncode == 65 and code_of(r) == 'unknown_event', 'history-request-unconfirmable', r.stderr)
    # Losing trailing events is damage: nothing reads as a shorter history, and
    # a lost event's ID and request are not stored again with other content.
    t = Fixture(binary, temp / 'tail')
    for n in range(3):
        t.add('--kind', '관측', '--summary', 'event %d' % n, '--request-id', 'tail-%d' % n)
    hist = t.history_dir()
    full = (hist / 'events.jsonl').read_bytes()
    last = (hist / 'last').read_bytes() if (hist / 'last').exists() else None
    lines = full.splitlines(keepends=True)

    def restore():
        (hist / 'events.jsonl').write_bytes(full)
        if last is None:
            (hist / 'last').unlink(missing_ok=True)
        else:
            (hist / 'last').write_bytes(last)
    for name, change in (('last line', lambda: (hist / 'events.jsonl').write_bytes(b''.join(lines[:2]))),
                         ('all lines', lambda: (hist / 'events.jsonl').write_bytes(b'')),
                         ('events file', lambda: (hist / 'events.jsonl').unlink()),
                         ('last record', lambda: (hist / 'last').unlink(missing_ok=True))):
        restore()
        change()
        q = t.h('list', '--format', 'json')
        a = t.h('add', '--kind', '결정', '--summary', 'different content', '--request-id', 'tail-2')
        require((q.returncode, code_of(q), a.returncode, code_of(a)) == (65, 'history_corrupt', 65, 'history_corrupt'),
                'history-tail-loss-hidden', '%s: %s %s' % (name, q.stdout + q.stderr, a.stdout + a.stderr))
    restore()
    require(t.view('list')['total'] == 3, 'history-tail-loss-hidden', 'restored history')
    # Reads refuse a symlink anywhere on the storage path, as writes do.
    for level in ('ha', 'history', 'start'):
        k = Fixture(binary, temp / ('read-link-' + level))
        k.add('--kind', '관측', '--summary', 'real')
        target = {'ha': k.common() / 'ha', 'history': k.common() / 'ha' / 'history', 'start': k.history_dir()}[level]
        moved = temp / ('moved-' + level)
        target.rename(moved)
        os.symlink(moved, target)
        q = k.h('list', '--format', 'json')
        require(q.returncode == 70 and code_of(q) == 'history_unreadable', 'history-read-through-link', '%s: %s' % (level, q.stdout + q.stderr))
    # A query waits for a registration in progress instead of reading its line.
    w = Fixture(binary, temp / 'wait')
    w.add('--kind', '관측', '--summary', 'first')
    with open(w.history_dir() / 'lock', 'r+') as held:
        fcntl.flock(held, fcntl.LOCK_EX)
        proc = subprocess.Popen([str(binary), 'history', w.goal, 'list', '--format', 'json'], cwd=w.root, env=w.env,
                                stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        try:
            proc.wait(timeout=1)
            waited = False
        except subprocess.TimeoutExpired:
            waited = True
        fcntl.flock(held, fcntl.LOCK_UN)
    out, err = proc.communicate(timeout=60)
    require(waited and proc.returncode == 0 and json.loads(out or '{}').get('total') == 1, 'history-read-during-write', err)
    # Large registrations from a linked worktree never read as damage elsewhere.
    c = Fixture(binary, temp / 'concurrent')
    peer = c.worktree('peer')
    c.add('--kind', '관측', '--summary', 'seed')
    refs = [x for n in range(40) for x in ('--ref', 'external:https://example.invalid/%d/%s' % (n, 'y' * 400))]
    def writer():
        for n in range(15):
            c.h('add', '--kind', '관측', '--summary', 'large %d' % n, '--reason', 'x' * (16 * 1024), *refs, cwd=peer)
    with ThreadPoolExecutor(max_workers=1) as pool:
        job = pool.submit(writer)
        seen = set()
        while not job.done():
            q = c.h('list', '--limit', '1', '--format', 'json')
            seen.add((q.returncode, code_of(q)))
        job.result()
    require(seen <= {(0, '')} and c.view('list')['total'] == 16, 'history-read-during-write', seen)
    # A symlinked storage path is refused through the CLI too.
    s = Fixture(binary, temp / 'symlink')
    outside = temp / 'outside'
    outside.mkdir()
    (s.common() / 'ha').mkdir(exist_ok=True)
    os.symlink(outside, s.common() / 'ha' / 'history')
    r = s.h('add', '--kind', '관측', '--summary', 'x')
    require(r.returncode == 70 and code_of(r) == 'history_unreadable' and not any(outside.iterdir()), 'history-wrote-through-link', r.stderr)
    # Fault-injected writes and lock timing are observed at the store boundary.
    # A stored line whose last-event record cannot be written is reported, not
    # acknowledged as a plain success; the same request then replays.
    if os.geteuid() != 0:
        u = Fixture(binary, temp / 'unanchored')
        u.add('--kind', '관측', '--summary', 'first')
        os.chmod(u.history_dir(), 0o555)
        try:
            r = u.h('add', '--kind', '관측', '--summary', 'second', '--request-id', 'second-1')
        finally:
            os.chmod(u.history_dir(), 0o755)
        shown = u.view('show', '--request-id', 'second-1')['event']['id']
        again = u.add('--kind', '관측', '--summary', 'second', '--request-id', 'second-1')
        require(r.returncode == 70 and code_of(r) == 'write_uncertain' and 'D2 was stored' in r.stderr and 'second-1' in r.stderr
                and shown == 'D2' and again['replayed'] and again['id'] == 'D2', 'history-unanchored-success', (r.returncode, r.stderr, again))
        events = u.history_dir() / 'events.jsonl'
        events.write_bytes(events.read_bytes().splitlines(keepends=True)[0])
        q = u.h('list', '--format', 'json')
        require(q.returncode == 65 and code_of(q) == 'history_corrupt', 'history-tail-loss-hidden', q.stdout + q.stderr)
    units = {'TestUncertainWriteCanBeConfirmedByRequest', 'TestDamageAndUnsupportedAreDistinct', 'TestSymlinkedStorageIsRefused', 'TestStrictInput',
             'TestTrailingEventLossIsDamage', 'TestInterruptedRecordCatchesUp', 'TestReadsRefuseSymlinkedStorage', 'TestReadsWaitForRegistration',
             'TestFirstRegistrationReadWaits', 'TestUnanchoredRegistrationIsReported'} | STORAGE_BOUNDARY_TESTS
    result = run(['go', 'test', '-count=1', '-json', './internal/history', '-run', '^(%s)$' % '|'.join(sorted(units))], ROOT)
    passed = {json.loads(l).get('Test') for l in result.stdout.splitlines() if l.startswith('{') and json.loads(l).get('Action') == 'pass'}
    failed = {json.loads(l).get('Test') for l in result.stdout.splitlines() if l.startswith('{') and json.loads(l).get('Action') == 'fail'} - {None}
    require(not failed, 'history-unit-assertion', failed)
    if result.returncode or not units <= passed:
        raise Environment('collection_error', result.stdout[-2000:] + result.stderr)


def unsupported_history_rules(binary, temp):
    queries = ([], ['current'], ['list'], ['show', 'D1'], ['show', '--request-id', 'old'],
               ['add', '--kind', '관측', '--summary', 'new', '--request-id', 'new'],
               ['add', '--kind', '관측', '--summary', 'old', '--request-id', 'old'])
    for populated in (False, True):
        f = Fixture(binary, temp / str(populated))
        if populated:
            f.add('--kind', '관측', '--summary', 'old', '--request-id', 'old')
        original = f.history_dir()
        path = f.root / f.goal / 'runs.jsonl'
        rows = [json.loads(line) for line in path.read_bytes().splitlines()]
        rows[0]['rules'] = 'run-rules/999'
        encoded = []
        for row in rows:
            row['prev'] = hashlib.sha256(encoded[-1]).hexdigest() if encoded else None
            encoded.append(json.dumps(row, separators=(',', ':'), ensure_ascii=False).encode())
        path.write_bytes(b'\n'.join(encoded) + b'\n')
        if populated:
            # Make the unsupported start's storage present as well. It must
            # still be rejected before reading or changing any history data.
            shutil.copytree(original, f.history_dir())
        before = f.snapshot()
        for query in queries:
            result = f.h(*query)
            require(result.returncode == 66 and code_of(result) == 'history_unsupported',
                    'history-unsupported-rules-unidentified', (query, result.returncode, result.stderr))
            require(f.snapshot() == before, 'history-unsupported-rules-mutated', query)
            require(f.history_dir().exists() == populated, 'history-unsupported-storage-created', query)


def ac7(binary, temp):
    f = Fixture(binary, temp / 'main')
    progress = f.root / f.goal / 'PROGRESS.md'
    progress.write_text(progress.read_text().replace('| todo |', '| done |'))
    f.commit('tasks done')
    f.ok('check', f.goal, 'AC-2')
    f.ok('check', f.goal, 'EX-1')
    queries = (['status', f.goal, '--format', 'json'], ['status', f.goal], ['estimate', f.goal, '--format', 'json'], ['finding', f.goal], ['lint', f.goal])
    def observe():
        return [(r.returncode, r.stdout, r.stderr) for r in (f.cli(*q) for q in queries)]
    base, runs = observe(), f.runs()
    f.add('--kind', '검증', '--status', '통과', '--summary', 'claimed', '--target', 'AC-2')
    f.add('--kind', '결정', '--summary', 'complete now', '--target', 'AC-1', '--builds-on', 'D1')
    f.add('--kind', '원인', '--summary', 'correction', '--supersedes', 'D1')
    require(observe() == base and f.runs() == runs, 'history-changed-execution-semantics', 'after events')
    events = f.history_dir() / 'events.jsonl'
    good = events.read_bytes()
    for data in (good[:-3], good + b'{"schema":"decision-history/v9"}\n'):
        events.write_bytes(data)
        require(observe() == base, 'history-changed-execution-semantics', 'damaged history')
    events.write_bytes(good)
    shutil.rmtree(f.history_dir())
    (f.common() / 'ha' / 'history').rmdir()
    (f.common() / 'ha' / 'history').write_text('not a directory\n')
    require(observe() == base, 'history-changed-execution-semantics', 'unreachable history')
    # Execution, completion, budget, findings, and blocks keep their meaning.
    r = f.cli('check', f.goal)
    require(r.returncode == 0, 'history-changed-execution-semantics', r.stdout + r.stderr)
    done = f.cli('done', f.goal, '--format', 'json')
    require(done.returncode == 0 and json.loads(done.stdout)['completion_record'], 'history-changed-execution-semantics', done.stdout)
    after = f.runs()
    require(after.startswith(runs) and all(json.loads(l)['schema'] == 'run/v1' for l in after.splitlines()), 'history-entered-run-record', 'runs')
    b = Fixture(binary, temp / 'blocked')
    b.ok('note', b.goal, 'block', '--cause', 'environment', '--quote', 'Synthetic block')
    b.add('--kind', '재개', '--summary', 'the block is gone')
    b.add('--kind', '막힘', '--summary', 'resolved by narration')
    code, state = b.status()
    require(code == 3 and state['status'] == 'blocked', 'history-cleared-a-block', state['status'])
    c = Fixture(binary, temp / 'budget')
    _, before = c.status()
    for n in range(5):
        c.add('--kind', '관측', '--summary', 'count %d' % n)
    _, now = c.status()
    require(before['budget'] == now['budget'] and before['usage'] == now['usage'], 'history-counted-as-runs', (before['budget'], now['budget']))
    cap = json.loads(f.cli('capabilities', '--format', 'json').stdout)
    require(cap['features'][:6] == ['estimate/v1', 'dirty-preflight/v1', 'baseline-safe-copy/v1', 'structured-change-results/v1',
                                     'budget-change/v1', 'post-completion-findings/v1'] and cap['default_rules'] == 'run-rules/3'
            and cap['supported_rules'] == ['run-rules/1', 'run-rules/2', 'run-rules/3'], 'history-changed-capabilities', cap)


def text(path):
    return (ROOT / path).read_text(encoding='utf-8')


def section(source, heading):
    start = source.find(heading + '\n')
    require(start >= 0, 'history-progress-roles-missing', 'missing ' + heading)
    level = len(heading) - len(heading.lstrip('#'))
    rest = source[start + len(heading) + 1:]
    match = re.search(r'(?m)^#{1,%d} ' % level, rest)
    return rest[:match.start()] if match else rest


def documents(n):
    """Seal documents describe the PROGRESS roles beside decision history."""
    if n == 8:
        need = {
            ('contracts/bundle.md', '### 5.3 판단 이력과 함께 쓰는 PROGRESS'): [
                'Seal 0.1.13 이상', '`## 현재`', '이력 없이도 이해되게', '보조 참조', '`## 타임라인`', '짧은 색인', '모든 사건을 복제하지 않는다',
                '`## 계획 변경`, `## 막힘`', '시각·사건 ID', '저장에 성공한 사건만', 'Markdown에 설명을 남긴다', '사건 ID 하나로만',
                '시작할 때의 PROGRESS 형식', '소급 생략', '변환하지 않는다', '새 clone', '일부 보완', '자동 생성하지 않는다', '문장 수나 고정 작성 순서'],
            ('plugins/seal/skills/seal/references/bundle.md', '### PROGRESS with decision history'): [
                'seal 0.1.13 or later', '`## 현재`', 'understandable without the history', 'auxiliary references', '`## 타임라인`', 'short index',
                'does not copy every event', '`## 계획 변경`, `## 막힘`', 'time and event ID', 'registration succeeded', 'leave the explanation in Markdown',
                'event ID alone', 'keeps the PROGRESS form', 'retroactively', 'convert past timelines', 'new clone', 'partly make up', 'No sentence counts or fixed writing order'],
            ('plugins/seal/skills/seal/SKILL.md', '## Decision history'): [
                'optional', 'seal 0.1.13 or later', '`## 현재` still states', 'auxiliary references', 'short index', 'point by time and event ID',
                'registration succeeded', 'write the explanation in Markdown', 'ID alone', 'Goals started earlier keep their PROGRESS form', 'new clone'],
            ('plugins/seal/skills/seal/assets/templates/PROGRESS.md', '## 타임라인'): [
                'seal 0.1.13 or later', 'short index line with the event ID', 'still stands on its own', 'auxiliary references',
                'point by time and event ID', 'registration succeeded', 'write the entry as above'],
        }
        for (path, heading), words in need.items():
            body = section(text(path), heading)
            for word in words:
                require(word in body, 'history-progress-roles-missing', '%s %s lacks %r' % (path, heading, word))
        seal = re.search(r'^\s*version: "(\d+)\.(\d+)\.(\d+)"', text('plugins/seal/skills/seal/SKILL.md'), re.M)
        require(seal is not None and tuple(map(int, seal.groups())) >= (0, 1, 13),
                'history-progress-roles-missing', 'Seal version boundary: 0.1.13 or later')
        result = run(['go', 'test', '-count=1', './plugins', '-run', 'Progress|Timeline|SealSkill|Compatibility'], ROOT)
        if result.returncode:
            if '--- FAIL' in result.stdout:
                raise Violation('history-existing-progress-rules-broken', result.stdout[-3000:])
            raise Environment('execution_error', result.stdout + result.stderr)


def ac9(binary, temp):
    f = Fixture(binary, temp / 'main')
    wt = f.worktree('peer')
    f.add('--kind', '결정', '--summary', 'from main')
    f.add('--kind', '결정', '--summary', 'from peer', cwd=wt)
    require(f.common(wt) == f.common() and ids(f.view('list')['items']) == ['D1', 'D2'] == ids(f.view('list', cwd=wt)['items']),
            'history-common-directory-not-shared', 'main and linked worktree')
    # Another started goal in the same repository keeps its own history.
    g2 = Fixture.__new__(Fixture)
    g2.__dict__.update(f.__dict__)
    g2.goal = 'docs/specs/second'
    shutil.copytree(f.root / f.goal, f.root / g2.goal, ignore=shutil.ignore_patterns('runs.jsonl'))
    f.commit('second goal')
    g2.ok('start', g2.goal, '--request', 'Second fixture', '--skill-name', 'synthetic', '--skill-version', '1')
    require(g2.view('current')['history'] == 'absent', 'history-isolation-broken', 'second goal saw the first history')
    g2.add('--kind', '관측', '--summary', 'second goal')
    require(ids(f.view('list')['items']) == ['D1', 'D2'] and g2.history_dir() != f.history_dir(), 'history-isolation-broken', 'mixed')
    # Names with spaces or line breaks never steer the storage path.
    for n, name in enumerate(('docs/specs/with space', 'docs/specs/line\nbreak')):
        odd = Fixture(binary, temp / ('odd%d' % n), goal=name)
        odd.add('--kind', '관측', '--summary', 'odd name')
        stored = sorted(p.relative_to(odd.common() / 'ha' / 'history') for p in (odd.common() / 'ha' / 'history').rglob('*') if p.is_file())
        require([str(p) for p in stored] == [odd.start_sha() + '/events.jsonl', odd.start_sha() + '/last', odd.start_sha() + '/lock'], 'history-path-steered', stored)
    # Worktree removal keeps the history; a new clone does not carry it.
    f.git('worktree', 'remove', '--force', str(wt))
    require(ids(f.view('list')['items']) == ['D1', 'D2'], 'history-removed-with-worktree', 'after worktree removal')
    clone = temp / 'clone'
    f.git('clone', '-q', str(f.root), str(clone))
    view = Fixture.__new__(Fixture)
    view.__dict__.update(f.__dict__)
    view.root = clone
    cloned = view.view('current')
    require(cloned['history'] == 'absent' and 'does not show that no event was ever recorded' in view.md('current'), 'history-clone-overclaimed', cloned)
    # A moved goal finds its history by start record and shows the move.
    f.git('mv', f.goal, 'docs/specs/moved')
    f.commit('move goal')
    moved = Fixture.__new__(Fixture)
    moved.__dict__.update(f.__dict__)
    moved.goal = 'docs/specs/moved'
    item_ = item(moved.view('list'), 'D1')
    require(item_ and item_['context']['goal'] == 'moved' and item_['context']['spec'] == 'changed', 'history-move-accepted-silently', item_)
    # Missing commits are unconfirmed, never fetched.
    receipt = moved.add('--kind', '관측', '--summary', 'unknown commit', '--ref', 'commit:' + '1' * 40)
    ev = moved.view('show', receipt['id'])['event']['evidence'][0]
    require(ev['recorded']['state'] == 'not_found', 'history-commit-fetched', ev)
    # The documented raw-log cleanup keeps the shared history.
    guide = text('guides/INSTALL.md') + text('guides/INSTALL.en.md')
    commands = set(re.findall(r'(?m)^gitdir="\$\(git rev-parse --absolute-git-dir\)" && rm -rf "\$gitdir/ha[^"]*"$', guide))
    require(commands == {'gitdir="$(git rev-parse --absolute-git-dir)" && rm -rf "$gitdir/ha/runs"'}, 'history-cleanup-removes-history', commands)
    logs = f.common() / 'ha' / 'runs'
    require(logs.exists(), 'history-lineage-fixture', 'no raw logs to clean')
    r = run(['sh', '-c', commands.pop()], moved.root, f.env)
    require(r.returncode == 0 and not logs.exists() and ids(moved.view('list')['items'])[:2] == ['D1', 'D2'], 'history-cleanup-removes-history', r.stderr)
    for path, words in (('guides/INSTALL.md', ['linked worktree를 제거해도 이 이력은 남지만', '새 clone', '자동으로 복구되지 않습니다', '판단 이력, 다른 작업 트리의 출력']),
                        ('guides/INSTALL.en.md', ['Removing a linked worktree leaves this history', 'new clone', 'no automatic recovery', 'Decision history, the output of other working trees']),
                        ('contracts/decision-history.md', ['보존·복원을 보증하지 않는다', '새 clone', 'fetch하지 않는다', '바뀐 경로·내용의 유효성을 자동으로 인정하지 않는다'])):
        for word in words:
            require(word in text(path), 'history-preservation-undocumented', '%s lacks %r' % (path, word))


def normalize(transcript):
    """Keep the platform-independent meaning of each probe step."""
    steps = {}
    for block in transcript.split('STEP ')[1:]:
        head, _, body = block.partition('\n')
        name, code = head.split()
        try:
            value = json.loads(body)
        except ValueError:
            match = re.search(r'ha: ([a-z_]+):', body)
            value = match[1] if match else body.strip()[:80]
        if isinstance(value, dict):
            if value.get('schema') == 'decision-receipt/v1':
                value = {k: value[k] for k in ('id', 'request_id', 'seq', 'replayed')}
            elif value.get('schema') == 'decision-history-view/v1':
                value = {'history': value['history'], 'items': [(i['type'], i['id'], i.get('kind'), i.get('status'), i['summary'], i.get('current'),
                                                                 json.dumps(i.get('grounds_corrected')), i.get('lineage')) for i in value['items']],
                         'event': value.get('event') and (value['event']['id'], value['event']['summary']),
                         'predecessors': [(p['relation'], p['id']) for p in value.get('predecessors') or []],
                         'truncated': value['truncated'], 'total': value['total']}
            elif 'status' in value:
                value = (value['status'], value.get('reasons'))
        steps[name] = (int(code), value)
    return steps


def ac10(binary, temp, capabilities):
    require(capabilities['formats'].get('decision_history') == ['decision-history/v1'] and 'decision-history/v1' in capabilities['features'],
            'history-capability-missing', capabilities)
    binaries = {}
    for target in ('darwin/arm64', 'darwin/amd64', 'linux/arm64', 'linux/amd64', 'windows/amd64'):
        goos, goarch = target.split('/')
        binaries[target] = build(temp, goos, goarch, 'ha-%s-%s' % (goos, goarch))
    native = run(['sh', str(PROBE), binary, temp / 'native'], ROOT)
    if native.returncode:
        raise Environment('execution_error', native.stdout + native.stderr)
    reference = normalize(native.stdout)
    require(reference.get('current', (1,))[0] == 0 and len(reference) == 22, 'history-probe-incomplete', reference)
    require(reference['absent'][1].get('history') == 'absent', 'history-platform-semantics', reference['absent'])
    expect = {'query-before-start': 65, 'absent': 0, 'not-started': 65, 'replay': 0, 'request-conflict': 65, 'bad-status': 64, 'bad-relation': 65,
              'bad-target': 65, 'stale': 65, 'reject': 0, 'tail-removed': 65, 'tail-add': 65, 'read-link': 70, 'restored': 0}
    for name, code in expect.items():
        require(reference[name][0] == code, 'history-platform-semantics', (name, reference[name]))
    require(reference['tail-removed'][1] == reference['tail-add'][1] == 'history_corrupt' and reference['read-link'][1] == 'history_unreadable',
            'history-platform-semantics', reference)
    observed = ['native']
    if shutil.which('docker') is None:
        raise Environment('dependency_missing', 'docker is required to run the Linux binaries')
    for arch in ('arm64', 'amd64'):
        out = temp / ('linux-' + arch)
        out.mkdir()
        shutil.copy(binaries['linux/' + arch], out / 'ha')
        shutil.copy(PROBE, out / 'probe.sh')
        r = run(['docker', 'run', '--rm', '--pull', 'never', '--network', 'none', '--platform', 'linux/' + arch,
                 '-v', '%s:/out:ro' % out, '--entrypoint', 'sh', 'jaekit-check-git:' + arch, '/out/probe.sh', '/out/ha', '/tmp/probe'], ROOT, timeout=600)
        if r.returncode or 'STEP' not in r.stdout:
            raise Environment('dependency_missing' if 'No such image' in r.stderr else 'execution_error', r.stdout + r.stderr)
        require(normalize(r.stdout) == reference, 'history-platform-semantics', 'linux/%s differs from native' % arch)
        # The replacement and growth boundaries rely on Unix open flags.
        # Execute on both Linux arches as a non-root user so permission
        # compatibility tests cannot silently bypass directory access rules.
        env = dict(os.environ, CGO_ENABLED='0', GOOS='linux', GOARCH=arch)
        compiled = run(['go', 'test', '-c', '-o', out / 'evidence.test', './cmd/ha'], ROOT, env)
        if compiled.returncode:
            raise Environment('compile_error', compiled.stdout + compiled.stderr)
        boundary = run(['docker', 'run', '--rm', '--pull', 'never', '--network', 'none', '--user', '1000:1000', '--platform', 'linux/' + arch,
                        '-v', '%s:/out:ro' % out, '--entrypoint', '/out/evidence.test', 'jaekit-check-git:' + arch,
                        '-test.v', '-test.run=^(%s)$' % '|'.join(sorted(FILE_BOUNDARY_TESTS))], ROOT, timeout=600)
        require(boundary.returncode == 0 and all('--- PASS: ' + name + ' ' in boundary.stdout for name in FILE_BOUNDARY_TESTS),
                'history-platform-file-boundary', boundary.stdout + boundary.stderr)
        compiled = run(['go', 'test', '-c', '-o', out / 'storage.test', './internal/history'], ROOT, env)
        if compiled.returncode:
            raise Environment('compile_error', compiled.stdout + compiled.stderr)
        storage = run(['docker', 'run', '--rm', '--pull', 'never', '--network', 'none', '--user', '1000:1000', '--platform', 'linux/' + arch,
                       '-v', '%s:/out:ro' % out, '--entrypoint', '/out/storage.test', 'jaekit-check-git:' + arch,
                       '-test.v', '-test.run=^(%s)$' % '|'.join(sorted(STORAGE_BOUNDARY_TESTS))], ROOT, timeout=600)
        require(storage.returncode == 0 and all('--- PASS: ' + name + ' ' in storage.stdout for name in STORAGE_BOUNDARY_TESTS),
                'history-platform-storage-boundary', storage.stdout + storage.stderr)
        observed.append('linux/' + arch)
    print('observed platforms:', ', '.join(observed), '; darwin/amd64 built only', file=sys.stderr)
    # An executable without the feature is never taken for one that has it.
    f = Fixture(binary, temp / 'older')
    older = dict(capabilities, features=[x for x in capabilities['features'] if x != 'decision-history/v1'],
                 formats={k: v for k, v in capabilities['formats'].items() if k != 'decision_history'})
    (temp / 'older.json').write_text(json.dumps(older))
    fake = temp / 'older-ha'
    fake.write_text('#!/bin/sh\ncat "%s"\n' % (temp / 'older.json'))
    fake.chmod(0o755)
    helper = ROOT / 'plugins/seal/skills/seal/scripts/check_core.py'
    for ha, compatible in ((binary, True), (fake, False)):
        r = run([sys.executable, helper, '--ha', ha, '--operation', 'history', '--goal', f.root / f.goal], f.root, f.env)
        report = json.loads(r.stdout or '{}')
        require(report.get('compatible') is compatible, 'history-support-misreported', r.stdout + r.stderr)
    require(report['reason'] == 'missing_support' and report['missing'] == ['features:decision-history/v1', 'formats:decision_history:decision-history/v1'],
            'history-support-misreported', report)
    # This synthetic capability/goal pair exercises only helper admission:
    # history does not inherit the optional findings support requirement.
    history_only = dict(capabilities, features=['decision-history/v1'],
                        formats={k: v for k, v in capabilities['formats'].items() if k != 'finding'})
    (temp / 'history-only.json').write_text(json.dumps(history_only))
    history_ha = temp / 'history-only-ha'
    history_ha.write_text('#!/bin/sh\ncat "%s"\n' % (temp / 'history-only.json'))
    history_ha.chmod(0o755)
    helper_goal = temp / 'helper-goal-with-findings'
    helper_goal.mkdir()
    (helper_goal / 'SPEC.md').write_text(SPEC)
    (helper_goal / 'runs.jsonl').write_bytes(f.runs() + b'{"schema":"run-finding/v1"}\n')
    r = run([sys.executable, helper, '--ha', history_ha, '--operation', 'history', '--goal', helper_goal], ROOT)
    report = json.loads(r.stdout or '{}')
    require(r.returncode == 0 and report.get('compatible') is True,
            'history-support-misreported', r.stdout + r.stderr)
    # Require actual Windows execution; cross-compilation is only a build gate.
    import ci_observer
    try:
        ci_observer.observe()
    except ci_observer.ci.Violation as error:
        raise Violation('history-platform-semantics', str(error)) from error
    except ci_observer.ci.ObservationError as error:
        raise Environment('execution_error', str(error)) from error
    for path in ('contracts/decision-history.md', 'contracts/core-capabilities.md',
                 'plugins/seal/skills/seal/SKILL.md', 'plugins/seal/skills/seal/references/ha.md',
                 'guides/WINDOWS.md', 'guides/WINDOWS.en.md'):
        require('Windows' in text(path), 'history-windows-support-undocumented', path)


def ac11(binary, temp):
    f = Fixture(binary, temp / 'main')
    sandbox = shutil.which('sandbox-exec') if sys.platform == 'darwin' else None
    def offline(*args):
        argv = [binary, 'history', f.goal, *args]
        if sandbox:
            argv = [sandbox, '-p', '(version 1)(allow default)(deny network*)', *argv]
        return run(argv, f.root, dict(f.env, HTTP_PROXY='http://127.0.0.1:9', HTTPS_PROXY='http://127.0.0.1:9', ALL_PROXY='http://127.0.0.1:9'))
    # Any kind order, no relations, folded events, and no prompts on a closed stdin.
    for kind, extra in (('재개', []), ('검증', ['--status', '실패']), ('결정', []), ('관측', []), ('조치', ['--status', '반영']), ('막힘', []), ('원인', [])):
        r = offline('add', '--kind', kind, '--summary', 'free order ' + kind, *extra)
        require(r.returncode == 0, 'history-procedure-imposed', r.stderr)
    r = offline('add', '--kind', '조치', '--status', '반영', '--summary', 'observed, fixed and verified in one event')
    require(r.returncode == 0, 'history-procedure-imposed', r.stderr)
    for args in (['current'], ['list'], ['show', 'D1']):
        r = offline(*args)
        require(r.returncode == 0, 'history-needs-service', r.stderr)
    require(f.view('current')['total'] == 8, 'history-procedure-imposed', 'all events stay current')
    skill = section(text('plugins/seal/skills/seal/SKILL.md'), '## Decision history')
    for word in ('Decision history is optional', 'when you choose', 'neither is required', 'kinds are not steps', 'needs no approval', 'Small events may be one event'):
        require(word in skill, 'history-procedure-imposed', 'SKILL lacks %r' % word)
    for path in ('plugins/seal/skills/seal/SKILL.md', 'plugins/seal/skills/seal/references/ha.md', 'contracts/decision-history.md'):
        body = text(path)
        for forbidden in ('must record', 'record every', 'always record', '반드시 기록', '모든 작업에 사건'):
            require(forbidden not in body, 'history-procedure-imposed', '%s says %r' % (path, forbidden))
    contract = text('contracts/decision-history.md')
    for word in ('모델·네트워크·계정·상시 프로세스·검색 DB를 쓰지 않는다', '새 사람 승인 절차나 고정 작업 절차를 만들지 않는다', '실행자가 정한다'):
        require(word in contract, 'history-procedure-imposed', 'contract lacks %r' % word)


def ac12(binary, temp, capabilities):
    contract = text('contracts/decision-history.md')
    example = section(contract, '## 11. 합성 예시')
    blocks = re.findall(r'```(\w+)\n(.*?)```', example, re.S)
    commands = [line for lang, body in blocks if lang == 'bash' for line in body.splitlines() if line.strip()]
    excerpts = [body for lang, body in blocks if lang == 'text']
    require(len(commands) == 6 and len(excerpts) == 2, 'history-example-unusable', blocks)
    f = Fixture(binary, temp / 'example', goal='docs/specs/empty-input')
    (f.root / 'src/count.go').write_text('package count\n')
    f.commit('count source')
    outputs = []
    for line in commands:
        argv = shlex.split(line)
        require(argv[0] == 'ha', 'history-example-unusable', line)
        r = f.cli(*argv[1:])
        require(r.returncode == 0, 'history-example-diverges', '%s -> %s%s' % (line, r.stdout, r.stderr))
        outputs.append(r.stdout)
    require([json.loads(o)['id'] for o in outputs[:4]] == ['D1', 'D2', 'D3', 'D4'], 'history-example-diverges', outputs[:4])
    for excerpt, output in zip(excerpts, outputs[4:]):
        for line in excerpt.splitlines():
            require(line.strip() and line in output, 'history-example-diverges', 'missing %r in\n%s' % (line, output))
    # Every public description names the same optional feature and limits.
    docs = {
        'contracts/decision-history.md': ['decision-history/v1', '선택 기능', 'correction_conflict', 'write_uncertain', 'local', 'Agent Wiki', '새 clone', '§5.3',
                                          '/last', '쓰지도 읽지도 않는다', '등록 잠금이 있으면 기다리며', '끝에서부터 지운 온전한 줄',
                                          '`last`와 이력을 함께 지우거나 이전 상태로 되돌린 경우는 드러나지 않는다', '실행자가 준 식별 값은 줄이지 않아',
                                          '`missing`(경로가 없음)', '첫 등록의 저장 디렉터리가 아직 없으면 등록 전의 부재를 반환할 수 있다', '첫 등록 잠금이 생기면 잠금 아래에서 다시 읽어', '두 플래그를 섞어 준 순서는 보존하지 않는다',
                                          '사건의 순서는 `seq`가 정한다', '`last`가 한 사건 뒤처질 수 있다', '`after D<n>`',
                                          '제어 문자가 든 경로는 JSON으로 읽는다', '사건은 저장했지만 `last`를 갱신하지 못함'],
        'contracts/core-capabilities.md': ['decision-history/v1', 'formats.decision_history', 'v0.1.4', '--operation history'],
        'contracts/bundle.md': ['decision-history.md', 'Seal 0.1.13'],
        'plugins/seal/skills/seal/SKILL.md': ['decision-history/v1', 'ha history', 'Events are claims', 'Goals started earlier keep their PROGRESS form'],
        'plugins/seal/skills/seal/references/ha.md': ['decision-history/v1', 'formats.decision_history', 'correction_conflict', 'write_uncertain', 'executor claims', '--operation', 'history',
                                                      'Removed trailing events count as damage', 'stored before its last-event record could be updated'],
        'plugins/seal/skills/seal/references/bundle.md': ['decision history', 'event ID'],
        'guides/USAGE.md': ['decision-history/v1', '판단 이력', '새 clone', 'v0.1.4'],
        'guides/USAGE.en.md': ['decision-history/v1', 'decision history', 'new clone', 'v0.1.4'],
        'guides/INSTALL.md': ['판단 이력', 'ha/history/'],
        'guides/INSTALL.en.md': ['decision history', 'ha/history/'],
    }
    for path, words in docs.items():
        body = text(path)
        for word in words:
            require(word in body, 'history-docs-diverge', '%s lacks %r' % (path, word))
    require('decision-history/v1' in capabilities['features'] and capabilities['formats']['decision_history'] == ['decision-history/v1'],
            'history-docs-diverge', 'capabilities')
    helper = run([sys.executable, ROOT / 'plugins/seal/skills/seal/scripts/check_core.py', '--ha', binary, '--operation', 'history', '--goal', f.root / f.goal], f.root, f.env)
    require(helper.returncode == 0 and json.loads(helper.stdout)['compatible'], 'history-docs-diverge', helper.stdout + helper.stderr)
    for path in docs:
        body = text(path)
        require('/Users/' not in body and '/home/' not in body, 'history-private-example', path)
    for claim in ('Agent Wiki를 구현', 'implements an Agent Wiki', '자동 전송한다', 'improves future models'):
        require(claim not in contract, 'history-overclaimed', claim)


def observe(binary, n, temp, capabilities):
    handlers = {1: ac1, 2: ac2, 3: ac3, 4: ac4, 5: ac5, 6: ac6, 7: ac7, 9: ac9, 11: ac11}
    if n in handlers:
        handlers[n](binary, temp)
    elif n == 10:
        ac10(binary, temp, capabilities)
    elif n == 12:
        ac12(binary, temp, capabilities)
    else:
        raise Environment('setup_error', 'unknown criterion %d' % n)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--criterion', required=True, type=int)
    args = parser.parse_args()
    result = {'target': f'AC-{args.criterion}', 'attempt': 1, 'status': 'pass'}
    try:
        with tempfile.TemporaryDirectory(prefix='jaekit-history-') as tmp:
            temp = Path(tmp).resolve()
            try:
                if args.criterion == 8:
                    documents(args.criterion)
                else:
                    binary = build(temp)
                    capabilities = probe(binary)
                    observe(binary, args.criterion, temp, capabilities)
            finally:
                for p in temp.rglob('*'):
                    if not p.is_symlink() and p.exists() and not os.access(p, os.W_OK | os.R_OK):
                        os.chmod(p, stat.S_IRUSR | stat.S_IWUSR | stat.S_IXUSR)
            print('Observed decision history properties: AC-%d' % args.criterion)
    except Violation as error:
        result.update(status='violation', violation=error.code)
        print(error.code + ': ' + error.detail, file=sys.stderr)
    except Environment as error:
        result.update(status='error', reason=error.reason)
        print(error.detail, file=sys.stderr)
    except Exception as error:  # noqa: BLE001 - every unexpected failure is an error
        result.update(status='error', reason='execution_error')
        print(repr(error), file=sys.stderr)
    if 'HA_EVIDENCE_PATH' in os.environ:
        report = {'schema': 'check-result/v1', 'invocation': os.environ['HA_EVIDENCE_INVOCATION'],
                  'declaration_digest': os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],
                  'producer': 'jaekit-decision-history', 'producer_version': '1',
                  'attempts_complete': True, 'observations': [result]}
        Path(os.environ['HA_EVIDENCE_PATH']).write_text(json.dumps(report) + '\n')
    return 0 if result['status'] == 'pass' else 1


if __name__ == '__main__':
    raise SystemExit(main())
