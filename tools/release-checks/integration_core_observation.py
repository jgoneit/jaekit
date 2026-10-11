"""Actual local Core records through collector/consumer; host/API envelopes synthetic."""
import copy
import importlib.util
import argparse
import contextlib
import io
import json
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
RELEASE = ROOT / 'tools/release-checks'
spec = importlib.util.spec_from_file_location('core_observation_fixtures', ROOT / 'tools/review-checks/test_observation.py')
fixtures = importlib.util.module_from_spec(spec)
spec.loader.exec_module(fixtures)
check = fixtures.check
GOAL = 'docs/specs/synthetic'


class CoreImageIsolation(fixtures.Case):
    def test_consumer_query_executes_verified_bytes_despite_original_path_replacement(self):
        original_path = self.root / 'original-core'
        original = (f'#!{sys.executable}\nprint("verified A")\n').encode()
        replacement = (f'#!{sys.executable}\nprint("unverified B")\n').encode()
        original_path.write_bytes(original); original_path.chmod(0o755)
        actual_command = check.command
        executed = []
        def replace_before_query(argv, cwd=check.ROOT, timeout=180):
            original_path.write_bytes(replacement)
            executed.append(Path(argv[0]))
            try:
                return actual_command(argv, cwd, timeout)
            finally:
                original_path.write_bytes(original)
        with mock.patch.object(check, 'command', side_effect=replace_before_query):
            self.assertEqual(check.query_core(original_path.read_bytes(), ['status', 'goal', '--format', 'json']), b'verified A\n')
        self.assertEqual(original_path.read_bytes(), original)
        self.assertNotEqual(executed, [original_path])
        self.assertFalse(executed[0].exists(), 'private image should be cleaned after the read-only query')

    def test_requested_path_change_and_restore_does_not_change_actual_image(self):
        import observe
        core = self.root / 'ha'
        marker = self.root / 'actual-image'
        original = (f'#!{sys.executable}\nfrom pathlib import Path\nPath({str(marker)!r}).write_text("verified A")\n').encode()
        replacement = (f'#!{sys.executable}\nfrom pathlib import Path\nPath({str(marker)!r}).write_text("unverified B")\n').encode()
        core.write_bytes(original); core.chmod(0o755)
        shell = os.environ.get('JAEKIT_TEST_BASH') or shutil.which('bash')
        if not shell:
            raise RuntimeError('required local shell unavailable: bash')
        output = self.root / 'original-execution.json'
        script = '"$JAEKIT_CORE" status goal'
        argv = [str(RELEASE / 'observe.py'), 'run', '--core', str(core), '--shell', shell,
                '--output', str(output), '--invocation', 'controlled-image', '--', script]
        args = argparse.Namespace(core=str(core), shell=shell, output=str(output), invocation='controlled-image',
                                  script=script, project=None, goal=None)
        actual_popen = subprocess.Popen
        def launch(command, **options):
            core.write_bytes(replacement)
            process = actual_popen(command, **options)
            class OriginalRun:
                def wait(self):
                    code = process.wait()
                    core.write_bytes(original)
                    return code
            return OriginalRun()
        error = io.StringIO()
        with mock.patch.object(observe.subprocess, 'Popen', side_effect=launch), mock.patch.object(sys, 'argv', argv), \
             contextlib.redirect_stderr(error):
            self.assertEqual(observe.collect(args), 0)
        self.assertEqual(marker.read_text(), 'verified A')
        self.assertEqual(core.read_bytes(), original)
        report = json.loads(output.read_bytes())
        self.assertTrue(report['identity_unchanged'])
        self.assertNotEqual(report['core']['path'], report['execution_image']['path'])
        self.assertEqual(report['core']['sha256'], report['execution_image']['sha256'])
        self.receipts = {str(output): error.getvalue()}
        artifact = {'path': str(output), 'sha256': check.digest(output.read_bytes())}
        value = self.observed_value('codex', 'spec', [sys.executable, *argv], artifact, core)
        self.checker.trace('codex', value, 'spec')


class ActualCore(fixtures.Case):
    @classmethod
    def setUpClass(cls):
        temporary = tempfile.TemporaryDirectory(prefix='jaekit-observed-core-')
        cls.addClassCleanup(temporary.cleanup)
        cls.binary = Path(temporary.name) / 'ha'
        result = subprocess.run(['go', 'build', '-mod=readonly', '-ldflags', '-X main.version=0.1.3',
                                 '-o', str(cls.binary), './cmd/ha'], cwd=ROOT, capture_output=True, timeout=120)
        if result.returncode:
            raise RuntimeError('local Core build failed: ' + result.stderr.decode())

    def command(self, argv, cwd, **kwargs):
        result = subprocess.run(argv, cwd=cwd, capture_output=True, timeout=120,
                                env=dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1'), **kwargs)
        self.assertEqual(result.returncode, 0, result.stdout.decode() + result.stderr.decode())
        return result

    def flow(self, shell='bash', mode='normal'):
        project = self.root / ('project-' + shell + '-' + mode)
        (project / GOAL).mkdir(parents=True)
        (project / 'tools').mkdir()
        (project / 'checks').mkdir()
        (project / 'sample').mkdir()
        shutil.copyfile(ROOT / 'tools/check-result-reference.py', project / 'tools/check-result-reference.py')
        for name in ('declaration.json', 'reference.json'):
            shutil.copyfile(ROOT / 'examples/check-result' / name, project / 'checks' / name)
        (project / 'sample/input.txt').write_text('not ready\n')
        (project / '.gitignore').write_text('docs/\n')
        (project / GOAL / 'SPEC.md').write_text('# Synthetic\n\nStatus: Ready\nCriteria-Format: nested/1\n\n## 목표\nA greeting.\n\n## Acceptance Criteria\n- **AC-1** sample input says hello.\n\n## Open Decisions\n없음\n')
        (project / GOAL / 'PLAN.md').write_text('''# Synthetic plan
## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `python3 tools/check-result-reference.py checks/declaration.json checks/reference.json` | checks/declaration.json, checks/reference.json, tools/check-result-reference.py | T001 |
## 결과 계약
| ID | 선언 경로 |
| --- | --- |
| AC-1 | checks/declaration.json |
## 범위
- 바꿀 수 있는 경로: `sample/**` (SPEC AC-1)
- 테스트 경로: (기본값)
- 유지할 동작: 없음
## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | greeting | AC-1 | — | — | 예 | — |
## 예산
- 검증 실행: 50
- 경과 시간: 4시간
- 비용: 미관측
''')
        (project / GOAL / 'REVIEW.md').write_text('# Synthetic review\n\n## 결론\nready\n')
        (project / GOAL / 'PROGRESS.md').write_text('# Synthetic progress\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | observed fixture |\n')
        if mode in ('multiple', 'default'):
            spec = project / GOAL / 'SPEC.md'
            spec.write_text(spec.read_text().replace('## Open Decisions', '- **AC-2** the same greeting remains observable through a second declared criterion.\n\n## Open Decisions'))
            plan = project / GOAL / 'PLAN.md'
            text = plan.read_text()
            criterion = next(line for line in text.splitlines() if line.startswith('| AC-1 | change |'))
            text = text.replace(criterion, criterion + '\n' + criterion.replace('AC-1', 'AC-2'))
            text = text.replace('| AC-1 | checks/declaration.json |', '| AC-1 | checks/declaration.json |\n| AC-2 | checks/declaration.json |')
            text = text.replace('| greeting | AC-1 |', '| greeting | AC-1, AC-2 |')
            plan.write_text(text)
        for args in (('init', '-q'), ('config', 'user.name', 'Synthetic'),
                     ('config', 'user.email', 'synthetic@example.invalid'), ('add', '.'), ('commit', '-qm', 'synthetic base')):
            self.command(['git', *args], project)
        baseline = {'normal': f'{GOAL} --baseline AC-1', 'trailing': f'{GOAL} AC-1 --baseline',
                    'leading': f'--baseline {GOAL} AC-1', 'multiple': f'{GOAL} AC-1 AC-2 --baseline',
                    'default': f'{GOAL} --baseline'}[mode]
        first = self.collect(project, shell, f'"$JAEKIT_CORE" --version; "$JAEKIT_CORE" start {GOAL} --request synthetic --host-name Synthetic --model {fixtures.base.MODEL}; "$JAEKIT_CORE" check {baseline}')
        (project / 'sample/input.txt').write_text('hello\n')
        self.command(['git', 'add', '.'], project)
        self.command(['git', 'commit', '-qm', 'synthetic implementation'], project)
        current = '' if mode == 'default' else 'AC-1 AC-2' if mode == 'multiple' else 'AC-1'
        second = self.collect(project, shell, f'"$JAEKIT_CORE" check {GOAL} {current}; "$JAEKIT_CORE" done {GOAL}')
        return project, [first, second]

    def collect(self, project, shell, script):
        shell_path = os.environ.get('JAEKIT_TEST_' + shell.upper()) or shutil.which(shell)
        if not shell_path:
            raise RuntimeError('required local shell unavailable: ' + shell)
        self.count += 1
        output = self.root / f'actual-core-{self.count}.json'
        argv = [sys.executable, str(RELEASE / 'observe.py'), 'run', '--core', str(self.binary), '--shell', shell_path,
                '--output', str(output), '--invocation', f'actual-{self.count}', '--project', str(project), '--goal', GOAL, '--', script]
        result = self.command(argv, project)
        return argv, {'path': str(output), 'sha256': check.digest(output.read_bytes())}, result.stderr.decode()

    def envelope(self, host, project, reports):
        commands = [shlex.join(argv) for argv, _, _ in reports]
        value = self.codex_commands('seal', commands) if host == 'codex' else self.bound_claude('seal', commands)
        value.update(project=str(project), goal=GOAL, core={'path': str(self.binary), 'sha256': check.digest(self.binary.read_bytes())})
        value['seal']['executions'] = []
        location = value['seal']['session_trace'] if host == 'codex' else value['session_trace']
        events = self.read_events(location)
        captured = self.read_events(value['seal']['stdout'])
        for index, (_, report, receipt) in enumerate(reports):
            call = f'tool-codex-{index}' if host == 'codex' else f'tool-2-{index}'
            value['seal']['executions'].append({'tool_call_id': call, 'report': report})
            if host == 'codex':
                events.append({'type': 'response_item', 'payload': {'type': 'function_call_output', 'call_id': call, 'output': receipt}})
            else:
                event = {'type': 'user', 'uuid': f'result-2-{index}', 'sessionId': fixtures.base.SESSION,
                    'promptId': 'prompt-2', 'parentUuid': 'reply-2', 'message': {'content': [
                        {'type': 'tool_result', 'tool_use_id': call, 'content': receipt}]}}
                events.append(event)
                captured.insert(-1, event)
        if host == 'codex':
            value['seal']['session_trace'] = self.lines(events)
        else:
            value['session_trace'] = self.lines(events)
            value['seal']['stdout'] = self.lines(captured)
        return value

    def public_core(self):
        # Remote archive authentication is a synthetic boundary here. All
        # collector/Core/status subprocesses and original records remain real.
        return mock.patch.object(check, 'archive_files', return_value={'ha': self.binary.read_bytes()})

    def changed_report(self, original, change):
        argv, artifact, _ = original
        report = json.loads(Path(artifact['path']).read_bytes())
        change(report)
        previous = None
        for sequence, event in enumerate(report['events'], 1):
            event.update(seq=sequence, prev=previous)
            previous = check.digest(json.dumps(event, sort_keys=True, separators=(',', ':')).encode())
        self.count += 1
        path = self.root / f'altered-observation-{self.count}.json'
        path.write_text(json.dumps(report, sort_keys=True) + '\n')
        encoded = path.read_bytes()
        receipt = {'schema': 'jaekit-execution-receipt/v1', 'invocation': report['invocation'],
                   'run_id': report['run_id'], 'sha256': check.digest(encoded)}
        return argv, {'path': str(path), 'sha256': check.digest(encoded)}, 'JAEKIT_EXECUTION_RECEIPT ' + json.dumps(receipt) + '\n'

    def test_execution_identity_cwd_goal_and_record_prefix_are_bound_at_final_consumer(self):
        project, reports = self.flow()
        def first_start(report):
            return next(event for event in report['events'] if event['kind'] == 'core_start' and event['argv'][1] == 'start')
        mutations = {
            'unreadable-child-id': lambda r: next(event for event in r['events'] if event['kind'] == 'core_exit').update(call=[]),
            'same-basename-core': lambda r: first_start(r).update(executable={'path': str(self.root / 'other/ha'), 'sha256': r['core']['sha256']}),
            'wrong-cwd': lambda r: first_start(r).update(cwd=str(self.root)),
            'wrong-goal': lambda r: first_start(r)['argv'].__setitem__(2, 'docs/specs/other'),
            'wrong-records': lambda r: first_start(r)['records_before'].update(sha256='0' * 64),
            'other-project-context': lambda r: r['context'].update(project=str(self.root)),
        }
        for reason, mutate in mutations.items():
            altered = [self.changed_report(reports[0], mutate), reports[1]]
            for host in ('codex', 'claude'):
                with self.subTest(reason=reason, host=host):
                    with self.assertRaises(check.Unavailable):
                        self.checker.trace(host, self.envelope(host, project, altered), 'seal')
        for host in ('codex', 'claude'):
            with self.assertRaisesRegex(check.Failure, 'does not observe ha done'):
                self.checker.trace(host, self.envelope(host, project, reports[:1]), 'seal')
            with mock.patch.object(check, 'archive_files', return_value={'ha': b'different public Core'}), \
                 mock.patch.object(self.checker, 'download', return_value=b'synthetic archive'):
                with self.assertRaisesRegex(check.Failure, 'public native asset'):
                    self.checker.trace(host, self.envelope(host, project, reports), 'seal')

    def test_actual_supported_shell_core_receipts_and_original_records_are_accepted(self):
        for shell in ('bash', 'zsh'):
            with self.subTest(shell=shell):
                project, reports = self.flow(shell)
                before = (project / GOAL / 'runs.jsonl').read_bytes()
                for host in ('codex', 'claude'):
                    value = self.envelope(host, project, reports)
                    with self.public_core(), mock.patch.object(self.checker, 'download', return_value=b'synthetic authenticated archive'):
                        self.checker.trace(host, value, 'seal')
                    self.assertEqual((project / GOAL / 'runs.jsonl').read_bytes(), before, 'consumer changed original goal records')

    def test_valid_baseline_flag_positions_and_multiple_default_criteria(self):
        for mode in ('trailing', 'leading', 'multiple', 'default'):
            with self.subTest(mode=mode):
                project, reports = self.flow(mode=mode)
                for host in ('codex', 'claude'):
                    with self.public_core(), mock.patch.object(self.checker, 'download', return_value=b'synthetic archive'):
                        self.checker.trace(host, self.envelope(host, project, reports), 'seal')

    def test_other_goal_old_completion_and_static_flow_cannot_supply_current_execution(self):
        project, reports = self.flow()
        for host in ('codex', 'claude'):
            value = self.envelope(host, project, reports)
            (project / 'docs/specs/other').mkdir(exist_ok=True)
            other = copy.deepcopy(value)
            other['goal'] = 'docs/specs/other'
            with self.assertRaises((check.Unavailable, check.Failure)):
                self.checker.trace(host, other, 'seal')
            old = self.collect(project, 'bash', f'"$JAEKIT_CORE" done {GOAL}')
            replay = self.envelope(host, project, [reports[0], old])
            with self.assertRaisesRegex(check.Unavailable, 'previous completion|did not append'):
                self.checker.trace(host, replay, 'seal')
            static = self.codex_commands('seal', fixtures.base.SEAL_COMMANDS) if host == 'codex' else self.bound_claude('seal', fixtures.base.SEAL_COMMANDS)
            with self.assertRaisesRegex(check.Unavailable, 'static text'):
                self.checker.trace(host, static, 'seal')


if __name__ == '__main__':
    unittest.main()
