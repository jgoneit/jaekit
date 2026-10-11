"""Adapter protocol tests; these synthetic producers are not product evidence."""
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('integration_results', ROOT / 'tools/verification-checks/integration_results.py')
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class IntegrationResultsTests(unittest.TestCase):
    def invoke(self, observations, *, code=1, edit='', no_report=False, declaration_edit=None):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            declaration = dict(schema='check-declaration/v1', producer='synthetic', producer_version='1',
                               producer_paths=['probe.py'], targets=[dict(id='x', violation='expected')])
            if declaration_edit:
                declaration_edit(declaration)
            (root / 'declaration.json').write_text(json.dumps(declaration))
            source = '''import json, os
from pathlib import Path
r=dict(schema='check-result/v1',producer='synthetic',producer_version='1',
 invocation=os.environ['HA_EVIDENCE_INVOCATION'],declaration_digest=os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],
 attempts_complete=True, observations=OBSERVATIONS)
EDIT
WRITE
raise SystemExit(CODE)
'''.replace('OBSERVATIONS', repr(observations)).replace('EDIT', edit).replace('CODE', str(code)).replace('WRITE', '' if no_report else "Path(os.environ['HA_EVIDENCE_PATH']).write_text(json.dumps(r))")
            (root / 'probe.py').write_text(source)
            return module.invoke(dict(id='probe', declaration='declaration.json', argv=[sys.executable, 'probe.py']), root=root)

    def test_actual_violation_and_mixed_attempts_are_not_replaced_by_expected_id(self):
        facts = [dict(target='x',attempt=1,status='violation',violation='actually-observed'),
                 dict(target='x',attempt=2,status='skip',reason='skipped'),
                 dict(target='x',attempt=3,status='error',reason='import_error')]
        result = self.invoke(facts)
        self.assertEqual(result[:-1], [dict(item,target='probe.x') for item in facts])
        self.assertEqual(result[-1]['status'],'pass')
        self.assertNotIn('expected', json.dumps(result))

    def test_exit_conflict_preserves_actual_facts_and_adds_execution_error(self):
        facts = [dict(target='x',attempt=1,status='violation',violation='actual')]
        result = self.invoke(facts, code=0)
        self.assertEqual(result[0]['violation'],'actual')
        self.assertEqual(result[-1]['status'],'error')
        passed = self.invoke([dict(target='x',attempt=1,status='pass')],code=0)
        self.assertTrue(all(item['status']=='pass' for item in passed))

    def test_valid_retry_and_skip_exit_zero_retain_facts_without_environment_error(self):
        retry = [dict(target='x',attempt=1,status='violation',violation='actual'),
                 dict(target='x',attempt=2,status='pass')]
        skipped = [dict(target='x',attempt=1,status='skip',reason='skipped')]
        for facts in (retry, skipped):
            for code in (0, 1):
                with self.subTest(facts=facts, code=code):
                    result = self.invoke(facts, code=code)
                    self.assertEqual(result[:-1], [dict(item,target='probe.x') for item in facts])
                    self.assertEqual(result[-1]['status'], 'pass')
                    self.assertFalse(any(item['status']=='error' for item in result))

    def test_invalid_child_declarations_cannot_establish_expected_failure(self):
        facts = [dict(target='x',attempt=1,status='violation',violation='expected')]
        edits = [lambda d: d.update(schema='unsupported'), lambda d: d.update(producer=3),
                 lambda d: d.update(extra=True), lambda d: d['targets'].append(d['targets'][0]),
                 lambda d: d['producer_paths'].append('probe.py'),
                 lambda d: d['targets'][0].update(extra='ignored'),
                 lambda d: d.update(producer_paths=['../outside'])]
        for edit in edits:
            with self.subTest(edit=edit):
                result = self.invoke(facts, declaration_edit=edit)
                self.assertTrue(all(item['status']=='error' for item in result))
                self.assertTrue(all('violation' not in item for item in result))

    def test_missing_misbound_incomplete_and_malformed_reports_are_not_positive_evidence(self):
        facts = [dict(target='x',attempt=1,status='pass')]
        for edit in ["r['invocation']='wrong'", "r['attempts_complete']=False", "r['observations']=[]",
                     "r['observations'][0]['attempt']=True", "r['observations'][0]['attempt']=2",
                     "r['observations'][0]['target']='elsewhere'", "r['observations'][0]['target']=[]",
                     "r['unknown']=True"]:
            with self.subTest(edit=edit):
                result = self.invoke(facts, code=0, edit=edit)
                self.assertTrue(all(item['status']=='error' for item in result))
                self.assertTrue(all('violation' not in item for item in result))
        self.assertTrue(all(item['status']=='error' for item in self.invoke(facts,no_report=True)))


if __name__ == '__main__':
    unittest.main()
