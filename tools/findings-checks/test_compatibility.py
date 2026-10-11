"""Synthetic capability boundaries; actual executable checks live in check.py."""
import copy
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location('finding_compatibility', ROOT/'plugins/seal/skills/seal/scripts/check_core.py')
HELPER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HELPER)


class FindingCompatibility(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.goal = Path(self.directory.name)
        (self.goal/'SPEC.md').write_text('# Synthetic\n')
        self.cap = dict(schema='ha-capabilities/v1', ha_version='synthetic',
                        default_rules='run-rules/3', supported_rules=sorted(HELPER.KNOWN_RULES),
                        features=sorted(HELPER.START_FEATURES | {'post-completion-findings/v1'}),
                        formats=dict(criteria=['legacy','nested/1'], finding=['run-finding/v1'],
                                     **{k:[v] for k,v in HELPER.FORMATS.items()}))

    def records(self, rules, extension=False):
        entries = [dict(schema='run/v1',kind='start',rules=rules)]
        if extension:
            entries.append(dict(schema='run-finding/v1',kind='finding'))
        (self.goal/'runs.jsonl').write_text(''.join(json.dumps(x)+'\n' for x in entries))

    def test_explicit_operation_all_saved_rules(self):
        for rules in sorted(HELPER.KNOWN_RULES):
            self.records(rules)
            self.assertEqual(HELPER.requirements(self.cap,'finding',str(self.goal)),rules)
            for missing in ('feature','format'):
                cap = copy.deepcopy(self.cap)
                if missing == 'feature':
                    cap['features'].remove('post-completion-findings/v1')
                else:
                    del cap['formats']['finding']
                with self.assertRaises(HELPER.Refusal) as refused:
                    HELPER.requirements(cap,'finding',str(self.goal))
                self.assertEqual(refused.exception.reason,'missing_support')

    def test_resume_cannot_silently_ignore_extensions(self):
        old = copy.deepcopy(self.cap)
        old['features'].remove('post-completion-findings/v1')
        del old['formats']['finding']
        for rules in sorted(HELPER.KNOWN_RULES):
            self.records(rules)
            self.assertEqual(HELPER.requirements(old,'resume',str(self.goal)),rules)
            self.records(rules,True)
            for operation in ('resume','check'):
                with self.assertRaises(HELPER.Refusal):
                    HELPER.requirements(old,operation,str(self.goal))
                self.assertEqual(HELPER.requirements(self.cap,operation,str(self.goal)),rules)

    def test_bad_format_and_unknown_extension(self):
        bad = copy.deepcopy(self.cap)
        bad['formats']['finding']='run-finding/v1'
        with self.assertRaises(HELPER.Refusal):
            HELPER.validate(bad)
        self.records('run-rules/3',True)
        path=self.goal/'runs.jsonl'
        path.write_text(path.read_text().replace('run-finding/v1','run-finding/future'))
        with self.assertRaises(HELPER.Refusal) as refused:
            HELPER.requirements(self.cap,'resume',str(self.goal))
        self.assertEqual(refused.exception.reason,'unsupported_record_schema')


if __name__=='__main__':
    unittest.main()
