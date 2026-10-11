"""Actual unittest -> review observer -> local Core result consumption."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
GOAL = 'docs/specs/synthetic'
OBSERVER = 'tools/verification-checks/review_regression_gates.py'
DECLARATION = 'tools/verification-checks/review-regression-gates/AC-1.json'
ENV = {k: v for k, v in os.environ.items() if not k.startswith('HA_EVIDENCE_')}
ENV.update(PYTHONDONTWRITEBYTECODE='1', GOTOOLCHAIN='local', GOFLAGS='-mod=readonly',
           GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1')


class ObserverCore(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        temporary = tempfile.TemporaryDirectory(prefix='jaekit-observer-core-')
        cls.addClassCleanup(temporary.cleanup)
        cls.binary = Path(temporary.name) / 'ha'
        result = subprocess.run(['go', 'build', '-mod=readonly', '-o', str(cls.binary), './cmd/ha'],
                                cwd=ROOT, env=ENV, capture_output=True, text=True, timeout=120)
        if result.returncode:
            raise RuntimeError('local Core build failed: ' + result.stderr)

    def run_case(self, body):
        with tempfile.TemporaryDirectory(prefix='jaekit-core-result-') as temporary:
            root = Path(temporary)
            for name in (OBSERVER, DECLARATION):
                dest = root / name
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(ROOT / name, dest)
            tests = root / 'tools/regression-checks/test_map.py'
            tests.parent.mkdir(parents=True)
            tests.write_text('import unittest\nclass ReachableSelection(unittest.TestCase):\n' + body)
            (root / '.gitignore').write_text('docs/\n')
            goal = root / GOAL
            goal.mkdir(parents=True)
            (goal / 'SPEC.md').write_text('# Synthetic\n\nStatus: Ready\nCriteria-Format: nested/1\n\n## 목표\nObserve special unittest outcomes.\n\n## Acceptance Criteria\n- **AC-1** Preserve non-pass facts.\n\n## Open Decisions\n없음\n')
            (goal / 'PLAN.md').write_text(f'''# Synthetic plan
## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `python3 {OBSERVER} AC-1` | {OBSERVER}, {DECLARATION}, tools/regression-checks/test_map.py | T001 |
## 결과 계약
| ID | 선언 경로 |
| --- | --- |
| AC-1 | {DECLARATION} |
## 범위
- 바꿀 수 있는 경로: `tools/**` (synthetic fixture)
- 테스트 경로: (기본값)
- 유지할 동작: none
## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | observe | AC-1 | — | — | 예 | — |
## 예산
- 검증 실행: 10
- 경과 시간: 1시간
- 비용: 미관측
''')
            (goal / 'REVIEW.md').write_text('# Synthetic review\n## 결론\nready\n')
            (goal / 'PROGRESS.md').write_text('# Synthetic progress\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | synthetic fixture |\n')
            for args in (('init', '-q'), ('config', 'user.name', 'Synthetic'),
                         ('config', 'user.email', 'synthetic@example.invalid'), ('add', '.'),
                         ('commit', '-qm', 'synthetic observer input')):
                subprocess.run(['git', *args], cwd=root, env=ENV, check=True, capture_output=True, timeout=30)
            start = subprocess.run([str(self.binary), 'start', GOAL, '--request', 'synthetic'],
                                   cwd=root, env=ENV, capture_output=True, text=True, timeout=30)
            self.assertEqual(start.returncode, 0, start.stdout + start.stderr)
            for baseline in (False, True):
                argv = [str(self.binary), 'check', GOAL, 'AC-1']
                if baseline:
                    argv.append('--baseline')
                result = subprocess.run(argv, cwd=root, env=ENV, capture_output=True, text=True, timeout=90)
                records = [json.loads(line) for line in (goal / 'runs.jsonl').read_text().splitlines()]
                record = records[-1]
                self.assertIn(record['kind'], ('check', 'baseline'), result.stdout + result.stderr)
                if record['result'] != 'error':
                    self.fail('[violation:unittest-outcome-hidden] actual Core accepted special unittest outcome as ' + record['result'])
                self.assertNotEqual(result.returncode, 0)
                self.assertTrue(any(item['status'] == 'error' for item in record['evidence']['targets']))

    def test_special_results_alone_and_mixed_remain_errors_at_core(self):
        for statement in ("self.fail('expected assertion')", "raise RuntimeError('expected exception')", 'pass'):
            for mixed in (False, True):
                with self.subTest(statement=statement, mixed=mixed):
                    body = '    @unittest.expectedFailure\n    def test_special(self): ' + statement + '\n'
                    if mixed:
                        body += "    def test_normal(self): pass\n    def test_violation(self): self.fail('[violation:unreachable-selection-counted] observed')\n"
                    self.run_case(body)


if __name__ == '__main__':
    unittest.main()
