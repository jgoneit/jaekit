"""Runner failures cannot masquerade as observed baseline requirements."""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

spec=importlib.util.spec_from_file_location('regression_check',Path(__file__).with_name('check.py'))
check=importlib.util.module_from_spec(spec);spec.loader.exec_module(check)


class Runner(unittest.TestCase):
    def selected(self, source):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'test_sample.py';path.write_text(source)
            check.python_tests(path)

    def test_actual_zero_and_skipped_unittest_runs_are_unverified(self):
        for source in ("import unittest", "import unittest\n@unittest.skip('synthetic unavailable')\nclass Sample(unittest.TestCase):\n def test_one(self):pass"):
            with self.subTest(source=source), self.assertRaises(check.Environment):
                self.selected(source)

    def test_actual_assertion_failure_is_not_mislabeled_as_map_missing(self):
        source="import unittest\nclass Sample(unittest.TestCase):\n def test_one(self):self.fail('unrelated assertion')"
        with self.assertRaises(check.Environment):
            self.selected(source)

    def test_printed_summary_cannot_replace_an_executed_test(self):
        for source in ("print('Ran 12 tests in 0.1s\\nOK')", "import unittest\nclass Sample(unittest.TestCase):\n def test_one(self):self.fail('main never runs this test')\nprint('Ran 12 tests in 0.1s\\nOK')"):
            with self.subTest(source=source), self.assertRaises(check.Environment):
                self.selected(source)

    def test_go_zero_or_skipped_events_do_not_count(self):
        for events in ([{'Action':'pass','Package':'fixture'}], [{'Action':'run','Test':'TestSample'},{'Action':'skip','Test':'TestSample'}]):
            result=subprocess.CompletedProcess([],0,'\n'.join(map(json.dumps,events)),'')
            with patch.object(check,'run',return_value=result), self.assertRaises(check.Environment):
                check.tests(['go','test'],'go')

    def test_go_requires_matching_test_and_package_completion(self):
        start={'Action':'start','Package':'fixture'}
        run={'Action':'run','Package':'fixture','Test':'TestSample'}
        passed={'Action':'pass','Package':'fixture','Test':'TestSample'}
        package={'Action':'pass','Package':'fixture'}
        incomplete=([start,run], [start,run,passed], [start,run,package],
                    [start,run,dict(passed,Test='TestOther'),package],
                    [start,run,passed,package,{'Action':'unknown','Package':'fixture'}])
        for events in incomplete:
            result=subprocess.CompletedProcess([],0,'\n'.join(map(json.dumps,events)),'')
            with self.subTest(events=events),patch.object(check,'run',return_value=result),self.assertRaises(check.Environment):
                check.tests(['go','test'],'go')
        result=subprocess.CompletedProcess([],0,'\n'.join(map(json.dumps,[start,run,{'Action':'output','Package':'fixture','Test':'TestSample','Output':'synthetic frame','OutputType':'frame'},passed,package])),'')
        with patch.object(check,'run',return_value=result):check.tests(['go','test'],'go')

    def test_successful_test_is_observed_and_parent_evidence_is_not_forwarded(self):
        import os
        source="import unittest,os\nclass Sample(unittest.TestCase):\n def test_one(self):self.assertFalse(any(k.startswith('HA_EVIDENCE_') for k in os.environ))"
        with patch.dict(os.environ,{'HA_EVIDENCE_PATH':'synthetic-parent-artifact'}):
            self.selected(source)


if __name__=='__main__':unittest.main()
