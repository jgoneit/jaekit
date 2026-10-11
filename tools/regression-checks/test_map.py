"""Public registry boundaries; fixtures contain public source metadata only."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
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


SAMPLE_VERIFY = """import sys


def run(label, argv, env):
    pass


def verify_go(env):
    run("Go tests", ["go", "test", "-mod=readonly", "./..."], env)


def verify_docs(env):
    run("Sample regressions", [sys.executable, "-m", "unittest", "discover", "-s", "tools/sample-checks",
                               "-p", "test_*.py"], env)
"""
SAMPLE_TESTS = """import unittest


class Base(unittest.TestCase):
    pass


class Indirect(Base):
    def test_one(self):
        pass


class Mixin:
    def test_two(self):
        pass


class Combined(Mixin, unittest.TestCase):
    pass


class Claimed:
    def test_one(self):
        pass
"""
CHECK = {'file': 'tools/sample-checks/test_sample.py', 'selector': 'Indirect', 'group': 'docs'}


def git(root, *args):
    env = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1', GIT_AUTHOR_NAME='Synthetic',
               GIT_AUTHOR_EMAIL='synthetic@example.invalid', GIT_COMMITTER_NAME='Synthetic',
               GIT_COMMITTER_EMAIL='synthetic@example.invalid')
    return subprocess.run(['git', *args], cwd=root, env=env, check=True, capture_output=True, text=True).stdout


def sample_root(directory):
    """A public Git tree with two commits, a minimal verifier and sample regressions."""
    root = Path(directory) / 'public'
    for name, text in (('tools/verify.py', SAMPLE_VERIFY), ('tools/sample.py', '# public sample source\n'),
                       ('tools/sample-checks/test_sample.py', SAMPLE_TESTS)):
        (root / name).parent.mkdir(parents=True, exist_ok=True)
        (root / name).write_text(text)
    git(root, 'init', '-q')
    git(root, 'add', '.')
    git(root, 'commit', '-qm', 'synthetic public tree')
    (root / 'tools/sample.py').write_text('# public sample source, second revision\n')
    git(root, 'commit', '-qam', 'synthetic second revision')
    return root


def sample_registry(check=CHECK):
    def route(identifier, invariant, status, checks):
        return {'id': identifier, 'invariant': invariant, 'status': status, 'sources': ['tools/sample.py'],
                'property': 'synthetic property', 'examples': ['synthetic example'], 'families': ['synthetic family'],
                'boundary': 'synthetic boundary', 'checks': checks,
                'follow_up': ['https://github.com/jgoneit/jaekit/issues/27'] if status == 'gap' else []}
    routes = [route(f'i{n}-gap', f'I{n}', 'gap', []) for n in range(1, 6)]
    routes.append(route('sample-protected', 'I1', 'protected', [dict(check)]))
    return {'schema': 'regression-map/v1',
            'invariants': [{'id': f'I{n}', 'basis': ['tools/sample.py'], 'property': 'synthetic invariant'} for n in range(1, 6)],
            'routes': routes}


class ReachableSelection(unittest.TestCase):
    """A selection after a context body that can end the group is not protection."""

    def setUp(self):
        self.m = module()

    def test_context_body_that_can_end_the_group_hides_later_selection(self):
        original = (ROOT / 'tools/verify.py').read_text()
        marker = '    run("Go tests", ["go", "test", "-mod=readonly", "./..."], env)\n'
        self.assertIn(marker, original)
        for body in ('        return\n', '        raise SystemExit(0)\n', '        sys.exit(0)\n',
                     '        if env:\n            return\n'):
            with self.subTest(body=body), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                (root / 'tools').mkdir()
                (root / 'tools/verify.py').write_text(original.replace(marker, '    with open(__file__) as handle:\n' + body + marker, 1))
                try:
                    selected = self.m.selected_suites(root)
                except self.m.Invalid:
                    continue
                if ('<go>', './...') in selected['go']:
                    self.fail(f'[violation:unreachable-selection-counted] a call after {body.strip()!r} in a context body was selected')
        try:
            selected = self.m.selected_suites(ROOT)
        except self.m.Invalid as error:
            self.fail(f'[violation:unreachable-selection-counted] the supported build context was refused: {error}')
        if ('<go>', './...') not in selected['go']:
            self.fail('[violation:unreachable-selection-counted] the supported Go selection was lost')


class DiscoverableSelectors(unittest.TestCase):
    """A registered Python selector names tests the common loader collects; it is not executed."""

    def setUp(self):
        self.m = module()

    def test_selectors_must_be_collected_by_the_common_loader(self):
        with tempfile.TemporaryDirectory() as directory:
            root = sample_root(directory)
            for selector, discoverable in (('Indirect', True), ('Indirect.test_one', True), ('Combined', True),
                                           ('Combined.test_two', True), ('Claimed', False), ('Claimed.test_one', False),
                                           ('Mixin', False), ('Base', False)):
                with self.subTest(selector=selector):
                    try:
                        self.m.validate(root, sample_registry(dict(CHECK, selector=selector)))
                    except self.m.Invalid as error:
                        if discoverable:
                            self.fail(f'[violation:selector-not-discoverable] {selector} is collected but was refused: {error}')
                    else:
                        if not discoverable:
                            self.fail(f'[violation:selector-not-discoverable] {selector} names no collected test but was accepted')
            (root / 'tools/sample-checks/test_sample.py').write_text(SAMPLE_TESTS + 'import missing_sample_dependency\n')
            git(root, 'commit', '-qam', 'synthetic import failure')
            try:
                self.m.validate(root, sample_registry())
            except self.m.Invalid:
                pass
            else:
                self.fail('[violation:selector-not-discoverable] a module that cannot be collected still counted')


class PublicSources(unittest.TestCase):
    """Sources, bases and checks are tracked public files; full and shallow clones agree."""

    def setUp(self):
        self.m = module()

    def test_sources_and_checks_must_be_tracked_public_files(self):
        with tempfile.TemporaryDirectory() as directory:
            root = sample_root(directory)
            (root / 'tools/untracked.py').write_text('# local only\n')
            (root / 'docs/specs/private').mkdir(parents=True)
            (root / 'docs/specs/private/SPEC.md').write_text('private goal\n')
            for name in ('.git/config', '.git/HEAD', 'tools/untracked.py', 'docs/specs/private/SPEC.md'):
                for place in ('route source', 'invariant basis'):
                    with self.subTest(name=name, place=place):
                        data = sample_registry()
                        if place == 'route source':
                            data['routes'][-1]['sources'] = [name]
                        else:
                            data['invariants'][0]['basis'] = [name]
                        try:
                            self.m.validate(root, data)
                        except self.m.Invalid:
                            pass
                        else:
                            self.fail(f'[violation:private-source-accepted] {place} {name} counted as public evidence')
            (root / 'tools/sample-checks/test_local.py').write_text(SAMPLE_TESTS)
            try:
                self.m.validate(root, sample_registry(dict(CHECK, file='tools/sample-checks/test_local.py')))
            except self.m.Invalid:
                pass
            else:
                self.fail('[violation:private-source-accepted] an untracked test file counted as a protected check')
            expected = self.m.validate(root, sample_registry())
            for options in ((), ('--depth', '1')):
                clone = Path(directory) / ('clone' + ''.join(options).replace('-', ''))
                git(Path(directory), 'clone', '-q', *options, root.resolve().as_uri(), str(clone))
                with self.subTest(clone=options):
                    try:
                        actual = self.m.validate(clone, sample_registry())
                    except self.m.Invalid as error:
                        self.fail(f'[violation:private-source-accepted] a public clone {options} could not validate: {error}')
                    self.assertEqual(actual, expected)


class InvariantScope(unittest.TestCase):
    """Comparing explicit registries shows top-level invariant edits and their routes."""

    def setUp(self):
        self.m = module()
        self.registry = self.m.load(ROOT)

    def test_invariant_basis_or_property_changes_are_reported_with_their_routes(self):
        first = self.registry['invariants'][0]['id']
        routes = sorted(r['id'] for r in self.registry['routes'] if r['invariant'] == first)
        for field, value in (('property', 'a narrower synthetic property'), ('basis', ['README.md'])):
            with self.subTest(field=field):
                changed = copy.deepcopy(self.registry)
                changed['invariants'][0][field] = value
                result = self.m.compare(self.registry, changed)
                if result.get('changed_invariants') != [first] or result.get('affected_routes') != routes:
                    self.fail(f'[violation:invariant-scope-hidden] an invariant {field} edit was not reported: {result}')
        removed = copy.deepcopy(self.registry)
        removed['invariants'] = removed['invariants'][1:]
        if first not in self.m.compare(self.registry, removed).get('changed_invariants', []):
            self.fail('[violation:invariant-scope-hidden] a removed invariant was not reported')
        unchanged = self.m.compare(self.registry, copy.deepcopy(self.registry))
        if unchanged.get('changed_invariants') != [] or unchanged.get('affected_routes') != []:
            self.fail(f'[violation:invariant-scope-hidden] an unchanged registry reported changes: {unchanged}')


class PackagedGuidance(unittest.TestCase):
    """The shipped contract scopes registry commands to a source checkout."""

    def test_registry_commands_are_scoped_to_a_source_checkout(self):
        text = (ROOT / 'contracts/regressions.md').read_text()
        spec = importlib.util.spec_from_file_location('packaged_release', ROOT / 'tools/release/check_archives.py')
        release = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(release)
        packaged = {name for name in release.REQUIRED if name.startswith('tools/')}
        commands = set(re.findall(r'python3 (tools/[A-Za-z0-9_./-]+)', text))
        self.assertTrue(commands, 'the contract names its commands')
        if commands - packaged and not ('source checkout' in text and 'release archive' in text):
            self.fail(f'[violation:archive-guidance-misleading] the shipped contract names unpackaged tools '
                      f'without a source-checkout scope: {sorted(commands - packaged)}')


if __name__=='__main__':
    unittest.main()
