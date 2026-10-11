"""Public registry boundaries; fixtures contain public source metadata only."""
import copy
import importlib.util
from pathlib import Path
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


def module():
    spec=importlib.util.spec_from_file_location('regression_map',ROOT/'tools/regression_map.py')
    value=importlib.util.module_from_spec(spec);spec.loader.exec_module(value)
    return value


class Registry(unittest.TestCase):
    def setUp(self):
        self.m=module();self.registry=self.m.load(ROOT)

    def test_finite_invariants_and_explicit_gaps(self):
        report=self.m.validate(ROOT,self.registry)
        self.assertEqual(report['invariants'],['I1','I2','I3','I4','I5'])
        self.assertTrue(report['protected'])
        # Gaps may close after integration; this branch must describe its current gaps.
        self.assertFalse(set(report['protected']) & set(report['gaps']))
        for row in self.registry['routes']:
            if row['status']=='gap':
                self.assertTrue(row['follow_up'])
                self.assertFalse(row['checks'])
            else:
                self.assertTrue(row['checks'])

    def test_missing_unknown_duplicate_and_unbound_claims_are_refused(self):
        mutations=[]
        value=copy.deepcopy(self.registry);value['invariants'].pop();mutations.append(value)
        value=copy.deepcopy(self.registry);value['routes'].append(value['routes'][0]);mutations.append(value)
        value=copy.deepcopy(self.registry);value['unexpected']=True;mutations.append(value)
        protected=next(i for i,r in enumerate(self.registry['routes']) if r['status']=='protected')
        for field,bad in (('checks',[]),('sources',['tools/missing-product.py']),('invariant','I99'),('status','probably protected')):
            value=copy.deepcopy(self.registry);value['routes'][protected][field]=bad;mutations.append(value)
        for value in mutations:
            with self.subTest(value=value),self.assertRaises(self.m.Invalid):
                self.m.validate(ROOT,value)

    def test_definitions_must_be_selected_and_exist(self):
        protected=next(i for i,r in enumerate(self.registry['routes']) if r['status']=='protected')
        for change in ({'selector':'NoSuchTest'}, {'file':'tools/review-checks/test_platform_installation.py','selector':'AC8','group':'go'}, {'group':'imaginary'}):
            value=copy.deepcopy(self.registry)
            value['routes'][protected]['checks'][0].update(change)
            with self.subTest(change=change),self.assertRaises(self.m.Invalid):
                self.m.validate(ROOT,value)

    def test_removal_or_demotion_requires_visible_replacement(self):
        protected=next(r for r in self.registry['routes'] if r['status']=='protected')
        removed=copy.deepcopy(self.registry);removed['routes']=[r for r in removed['routes'] if r['id']!=protected['id']]
        changes=self.m.compare(self.registry,removed)
        self.assertIn(protected['id'],changes['lost_protection'])
        changed=copy.deepcopy(self.registry)
        row=next(r for r in changed['routes'] if r['id']==protected['id'])
        row.update(status='gap',checks=[],follow_up=['https://github.com/jgoneit/jaekit/issues/27'])
        self.assertIn(protected['id'],self.m.compare(self.registry,changed)['lost_protection'])
        narrowed=copy.deepcopy(self.registry)
        row=next(r for r in narrowed['routes'] if r['id']==protected['id']);row['checks']=[]
        self.assertIn(protected['id'],self.m.compare(self.registry,narrowed)['lost_protection'])
        self.assertEqual(self.m.compare(self.registry,copy.deepcopy(self.registry))['lost_protection'],[])

    def test_group_disconnection_is_observed_from_executable_selection(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);(root/'tools').mkdir()
            verify=(ROOT/'tools/verify.py').read_text()
            (root/'tools/verify.py').write_text(verify)
            before=self.m.selected_suites(root)
            self.assertIn(('tools/review-checks','test_observation.py'),before['go'])
            (root/'tools/verify.py').write_text(verify.replace('"test_observation.py", "test_installation.py", "test_producers.py"','"test_installation.py", "test_producers.py"',1))
            after=self.m.selected_suites(root)
            self.assertNotIn(('tools/review-checks','test_observation.py'),after['go'])
            # A mention in a comment is not an executed selection.
            with (root/'tools/verify.py').open('a') as out:
                out.write('\n# "tools/review-checks", "test_observation.py"\n')
            self.assertEqual(after,self.m.selected_suites(root))

    def test_unsupported_control_flow_cannot_claim_discovery(self):
        original=(ROOT/'tools/verify.py').read_text()
        for replacement in ('    return\n', '    if False:\n        pass\n'):
            with tempfile.TemporaryDirectory() as directory:
                root=Path(directory);(root/'tools').mkdir()
                source=original.replace('def verify_docs(env: dict[str, str]):\n','def verify_docs(env: dict[str, str]):\n'+replacement)
                (root/'tools/verify.py').write_text(source)
                with self.assertRaises(self.m.Invalid):self.m.selected_suites(root)

    def test_registry_result_is_deterministic_and_has_no_execution_claim(self):
        a=self.m.validate(ROOT,self.registry);b=self.m.validate(ROOT,self.registry)
        self.assertEqual(a,b)
        self.assertEqual(a['assurance'],'selection-only')
        self.assertNotIn('ci_passed',a)


if __name__=='__main__':
    unittest.main()
