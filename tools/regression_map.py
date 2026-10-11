#!/usr/bin/env python3
"""Validate the finite public regression registry; this does not execute tests."""
from __future__ import annotations

import argparse
import ast
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
INVARIANTS = {f'I{n}' for n in range(1, 6)}
_GO_READER = None
# Statements that can end or skip the rest of a verification group.
ENDING = (ast.Return, ast.Raise, ast.Break, ast.Continue, ast.If, ast.While, ast.Try,
          getattr(ast, 'TryStar', ast.Try), ast.Match, ast.Assert)
# Collect the actual discovery command, including its load_tests hooks, without
# running test bodies. Keep class ownership separate from potentially identical
# selector names in other modules and modules loaded under an explicit alias.
COLLECT = r"""import json, sys, unittest
from pathlib import Path
class Loader(unittest.TestLoader):
    def __init__(self):
        super().__init__()
        self.sources = {}
    def loadTestsFromModule(self, module, *, pattern=None):
        for value in vars(module).values():
            if (isinstance(value, type) and issubclass(value, unittest.TestCase)
                    and value.__module__ == module.__name__):
                self.sources[value] = getattr(module, '__file__', None)
        return super().loadTestsFromModule(module, pattern=pattern)
loader = Loader()
selection = json.loads(sys.argv[1])
loader.testNamePatterns = selection['names'] or None
suite = loader.discover(selection['start'], pattern=selection['pattern'],
                        top_level_dir=selection['top'])
def cases(suite):
    for item in suite:
        if isinstance(item, unittest.TestSuite):
            yield from cases(item)
        else:
            cls = type(item)
            module = sys.modules.get(cls.__module__)
            source = loader.sources.get(cls) or getattr(module, '__file__', None)
            method = getattr(item, '_testMethodName', None)
            yield {'file': str(Path(source).resolve()) if source else None,
                   'selector': cls.__qualname__ + '.' + method if method else None}
tests = list(cases(suite))
print(json.dumps({'errors': len(loader.errors), 'tests': tests}))
"""


class Invalid(ValueError):
    pass


def go_reader():
    """Load the fixed sibling reader only when Go source evidence is needed."""
    global _GO_READER
    if _GO_READER is None:
        spec = importlib.util.spec_from_file_location('jaekit_go_selection', Path(__file__).with_name('go_selection.py'))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        _GO_READER = module
    return _GO_READER


def pairs(items):
    result = {}
    for key, value in items:
        if key in result:
            raise Invalid(f'duplicate JSON key: {key}')
        result[key] = value
    return result


def load(root, path=None):
    return json.loads((path or root / 'contracts/regressions.json').read_text(encoding='utf-8'),
                      object_pairs_hook=pairs)


def fields(value, expected, label):
    if not isinstance(value, dict) or set(value) != set(expected.split()):
        raise Invalid(f'{label}: unsupported or missing fields')


def strings(value, label):
    if not isinstance(value, list) or not value or any(not isinstance(x, str) or not x.strip() for x in value):
        raise Invalid(f'{label}: nonempty strings required')
    if len(value) != len(set(value)):
        raise Invalid(f'{label}: duplicate values')


def tracked(root):
    """Tracked files of the Git source checkout at root; history is not read."""
    try:
        top = subprocess.run(['git', 'rev-parse', '--show-toplevel'], cwd=root, capture_output=True, text=True)
        files = subprocess.run(['git', 'ls-files', '-z'], cwd=root, capture_output=True)
    except OSError as error:
        raise Invalid('registry validation requires git and a source checkout') from error
    if top.returncode or files.returncode or Path(top.stdout.strip()).resolve() != Path(root).resolve():
        raise Invalid('registry validation requires a Git source checkout at the repository root')
    return {name for name in files.stdout.decode('utf-8', 'surrogateescape').split('\0') if name}


def source(root, name, public):
    if not isinstance(name, str) or not name or Path(name).is_absolute() or '..' in Path(name).parts:
        raise Invalid(f'unsafe source path: {name!r}')
    parts = Path(name).parts
    if '.git' in parts or parts[0] == 'docs':
        raise Invalid(f'repository metadata or local goal documents are not public sources: {name}')
    path = root / name
    if path.is_symlink() or not path.is_file() or not path.resolve().is_relative_to(root.resolve()):
        raise Invalid(f'missing or nonregular source: {name}')
    if Path(name).as_posix() not in public:
        raise Invalid(f'untracked source is not public evidence: {name}')
    return path


def ends_group(node):
    """Whether a context body can end or skip the rest of its group."""
    for child in ast.walk(node):
        if isinstance(child, ENDING):
            return True
        if isinstance(child, ast.Call):
            func = child.func
            if isinstance(func, ast.Name) and func.id in ('exit', 'quit'):
                return True
            if (isinstance(func, ast.Attribute) and isinstance(func.value, ast.Name) and
                    (func.value.id, func.attr) in (('sys', 'exit'), ('os', '_exit'), ('os', 'abort'))):
                return True
    return False


def collected(root, selection):
    """Source-owned selectors collected by one actual common discovery command."""
    directory, pattern = selection[:2]
    top, names = selection[2:] if len(selection) == 4 else (None, ())
    for value in (directory, top):
        if value is not None:
            path = root / value
            if not path.is_dir() or not path.resolve().is_relative_to(root.resolve()):
                raise Invalid(f'unsupported unittest discovery directory: {value}')
    configuration = dict(start=directory, pattern=pattern, top=top, names=names)
    label = f'{directory}/{pattern}'
    try:
        result = subprocess.run([sys.executable, '-c', COLLECT, json.dumps(configuration)], cwd=root,
                                env=dict(os.environ, PYTHONDONTWRITEBYTECODE='1'),
                                capture_output=True, text=True, timeout=300)
        data = json.loads(result.stdout.strip().splitlines()[-1]) if result.stdout.strip() else None
    except (OSError, subprocess.TimeoutExpired, ValueError) as error:
        raise Invalid(f'Python regression could not be collected: {label}') from error
    if result.returncode or not isinstance(data, dict) or data.get('errors') or not isinstance(data.get('tests'), list):
        raise Invalid(f'Python regression could not be collected: {label}')
    return data['tests']


def python_discovery(arguments):
    """Parse the supported standard unittest discover inputs, failing closed."""
    class DiscoveryParser(argparse.ArgumentParser):
        def error(self, message):
            raise Invalid(f'unsupported unittest discovery: {message}')

    if any(not isinstance(value, str) for value in arguments):
        raise Invalid('unsupported unittest discovery argument')
    parser = DiscoveryParser(add_help=False, allow_abbrev=False)
    parser.add_argument('-s', '--start-directory', dest='start', default='.')
    parser.add_argument('-p', '--pattern', default='test*.py')
    parser.add_argument('-t', '--top-level-directory', dest='top')
    # Match unittest's positional discovery inputs and option precedence.
    for name in ('start', 'pattern', 'top'):
        parser.add_argument(name, nargs='?', default=argparse.SUPPRESS)
    parser.add_argument('-k', dest='names', action='append', default=[])
    for options in (('-v', '--verbose'), ('-q', '--quiet'), ('-f', '--failfast'),
                    ('-c', '--catch'), ('-b', '--buffer'), ('--locals',)):
        parser.add_argument(*options, action='store_true')
    parsed = parser.parse_args(arguments)
    names = tuple(value if '*' in value else f'*{value}*' for value in parsed.names)
    # Preserve the existing simple selection representation for common callers.
    if parsed.top is None and not names:
        return parsed.start, parsed.pattern
    return parsed.start, parsed.pattern, parsed.top, names


def literal(node, bindings):
    if isinstance(node, ast.Constant):
        return node.value
    if isinstance(node, (ast.Tuple, ast.List)):
        return [literal(item, bindings) for item in node.elts]
    if isinstance(node, ast.Name) and node.id in bindings:
        return bindings[node.id]
    if isinstance(node, ast.Attribute) and isinstance(node.value, ast.Name) and node.value.id == 'sys' and node.attr == 'executable':
        return '<python>'
    if isinstance(node, ast.JoinedStr):
        return ''.join(str(literal(n.value, bindings)) if isinstance(n, ast.FormattedValue)
                       else str(n.value) for n in node.values)
    raise Invalid('unsupported dynamic verification selection')


def unfiltered_go_suite(argv):
    """Only claim the whole suite when explicit options cannot narrow it."""
    arguments = iter(argv[2:])
    for argument in arguments:
        if not isinstance(argument, str):
            raise Invalid('unsupported Go test argument')
        if not argument.startswith('-'):
            continue
        name, separator, value = argument.lstrip('-').partition('=')
        name = name.removeprefix('test.')
        if name in ('args', 'exec') or argument == '--':
            raise Invalid('unsupported Go test execution mode')
        if name in ('run', 'skip', 'list', 'fuzz', 'count'):
            if not separator:
                value = next(arguments, None)
            if not isinstance(value, str):
                raise Invalid('incomplete Go test selection option')
            if name == 'count':
                digits = value[1:] if value[:1] in ('+', '-') else value
                if not digits or any(char not in '0123456789' for char in digits) or int(value) <= 0:
                    raise Invalid('Go test count cannot establish full selection')
            elif value:
                raise Invalid('filtered Go tests are not the full suite')
        if name in ('short', 'c'):
            if not separator or value not in ('0', 'f', 'F', 'FALSE', 'false', 'False'):
                raise Invalid('restricted Go tests are not the full suite')


def selected_suites(root):
    """Read the supported unconditional run/literal-loop subset, without eval.

    Control flow that can hide a selected call is refused. Build-only with
    blocks are outside this selection parser; they are exercised by verify.py.
    A with body that selects tests, or can end or skip the rest of the group,
    is refused because later selections might not be reached.
    """
    tree = ast.parse((root / 'tools/verify.py').read_text())
    result = {'go': set(), 'docs': set()}

    def statements(nodes, bindings, output):
        for node in nodes:
            if isinstance(node, (ast.If, ast.Return, ast.While, ast.Try, ast.Match, ast.Break, ast.Continue)):
                raise Invalid('unsupported verification control flow')
            if isinstance(node, ast.With):
                constants = {n.value for n in ast.walk(node) if isinstance(n, ast.Constant) and isinstance(n.value, str)}
                if 'test' in constants or 'discover' in constants or 'unittest' in constants:
                    raise Invalid('test selection inside unsupported context manager')
                if ends_group(node):
                    raise Invalid('unsupported control flow inside context manager')
                continue
            if isinstance(node, ast.For):
                values = literal(node.iter, bindings)
                for value in values:
                    child = dict(bindings)
                    names = [node.target.id] if isinstance(node.target, ast.Name) else [n.id for n in node.target.elts]
                    parts = [value] if isinstance(node.target, ast.Name) else value
                    if len(names) != len(parts):
                        raise Invalid('unsupported loop binding')
                    child.update(zip(names, parts))
                    statements(node.body, child, output)
                if node.orelse:
                    raise Invalid('unsupported loop else')
            elif isinstance(node, ast.Expr) and isinstance(node.value, ast.Call):
                call = node.value
                if not isinstance(call.func, ast.Name) or call.func.id != 'run':
                    raise Invalid('unsupported verification call')
                argv = literal(call.args[1], bindings)
                if argv[:2] == ['go', 'test'] and './...' in argv:
                    unfiltered_go_suite(argv)
                    output.add(('<go>', './...'))
                if argv[1:4] == ['-m', 'unittest', 'discover']:
                    output.add(python_discovery(argv[4:]))
            elif not isinstance(node, ast.Pass):
                raise Invalid('unsupported verification statement')

    for group in result:
        functions = [n for n in tree.body if isinstance(n, ast.FunctionDef) and n.name == 'verify_' + group]
        if len(functions) != 1 or functions[0].decorator_list:
            raise Invalid('verification group missing, repeated, or decorated')
        statements(functions[0].body, {}, result[group])
    return result


def check_definition(root, check, selected, public, collections):
    fields(check, 'file selector group', 'check')
    file, selector, group = (check[k] for k in ('file', 'selector', 'group'))
    path = source(root, file, public)
    if group not in selected or not isinstance(selector, str):
        raise Invalid('unknown group or selector')
    if file.endswith('_test.go'):
        if group != 'go' or ('<go>', './...') not in selected[group]:
            raise Invalid(f'Go regression not selected: {file}')
        reader = go_reader()
        try:
            reader.validate_test(path, selector, root=root)
        except reader.SelectionError as error:
            raise Invalid(f'Go regression not selected: {file}:{selector}: {error}') from error
    elif file.endswith('.py'):
        parts = selector.split('.')
        if len(parts) not in (1, 2) or not all(re.fullmatch(r'[A-Za-z_]\w*', p) for p in parts):
            raise Invalid(f'unsupported Python selector: {selector}')
        for selection in sorted(selected[group], key=repr):
            if selection[0] == '<go>':
                continue
            if selection not in collections:
                collections[selection] = collected(root, selection)
            if any(test['file'] == str(path.resolve()) and isinstance(test['selector'], str)
                   and (test['selector'] == selector or
                        (len(parts) == 1 and test['selector'].startswith(selector + '.')))
                   for test in collections[selection]):
                break
        else:
            raise Invalid(f'Python test is not collected by the common loader: {file}:{selector}')
    else:
        raise Invalid(f'unsupported test type: {file}')


def validate(root, data):
    fields(data, 'schema invariants routes', 'registry')
    if data['schema'] != 'regression-map/v1':
        raise Invalid('unsupported registry schema')
    selected = selected_suites(root)
    public, collections = tracked(root), {}
    invariants = set()
    for row in data['invariants']:
        fields(row, 'id basis property', 'invariant')
        if row['id'] in invariants or row['id'] not in INVARIANTS:
            raise Invalid('unknown or duplicate invariant')
        invariants.add(row['id'])
        strings(row['basis'], 'basis')
        for basis in row['basis']:
            source(root, basis, public)
        if not isinstance(row['property'], str) or not row['property'].strip():
            raise Invalid('missing invariant property')
    if invariants != INVARIANTS:
        raise Invalid('finite invariant scope must be I1 through I5')
    ids, covered, protected, gaps = set(), set(), [], []
    for route in data['routes']:
        fields(route, 'id invariant status sources property examples families boundary checks follow_up', 'route')
        identifier = route['id']
        if not isinstance(identifier, str) or not re.fullmatch(r'[a-z0-9-]+', identifier) or identifier in ids:
            raise Invalid('missing or duplicate route ID')
        ids.add(identifier)
        if route['invariant'] not in INVARIANTS:
            raise Invalid('unknown route invariant')
        covered.add(route['invariant'])
        for key in ('sources', 'examples', 'families'):
            strings(route[key], key)
        for name in route['sources']:
            source(root, name, public)
        for key in ('property', 'boundary'):
            if not isinstance(route[key], str) or not route[key].strip():
                raise Invalid(f'missing route {key}')
        if route['status'] == 'protected':
            if not isinstance(route['checks'], list) or not route['checks']:
                raise Invalid('protection requires selected checks')
            for check in route['checks']:
                check_definition(root, check, selected, public, collections)
            protected.append(identifier)
        elif route['status'] == 'gap':
            strings(route['follow_up'], 'gap followup')
            if route['checks']:
                raise Invalid('gap cannot count checks as protected')
            gaps.append(identifier)
        else:
            raise Invalid('unsupported protection status')
        if not isinstance(route['follow_up'], list) or any(not re.fullmatch(r'https://github\.com/jgoneit/jaekit/issues/[0-9]+', x) for x in route['follow_up']):
            raise Invalid('followups must be public issue references')
    if covered != INVARIANTS:
        raise Invalid('every invariant requires explicit supported routes')
    report = dict(schema='regression-map/v1', assurance='selection-only', invariants=sorted(invariants),
                  protected=sorted(protected), gaps=sorted(gaps))
    if any(check['file'].endswith('_test.go') for route in data['routes'] for check in route['checks']):
        report['go_selection'] = dict(go_reader().CONTEXT)
    return report


def compare(before, after):
    """Surface removal, demotion, scope edits and narrower checks for review.

    Invariant property or basis edits, additions and removals are reported
    with every route under that invariant; they are not judged automatically.
    """
    current = {r['id']: r for r in after['routes']}
    lost = []
    for old in before['routes']:
        if old['status'] != 'protected':
            continue
        new = current.get(old['id'])
        if new is None or new['status'] != 'protected':
            lost.append(old['id'])
            continue
        if (any(old[k] != new[k] for k in ('invariant', 'sources', 'property', 'families', 'boundary')) or
            any(check not in new['checks'] for check in old['checks'])):
            lost.append(old['id'])
    old, new = ({row['id']: row for row in data['invariants']} for data in (before, after))
    changed = sorted(key for key in set(old) | set(new) if old.get(key) != new.get(key))
    return {'lost_protection': sorted(lost),
            'added_routes': sorted(set(current) - {r['id'] for r in before['routes']}),
            'changed_invariants': changed,
            'affected_routes': sorted({r['id'] for r in before['routes'] + after['routes'] if r['invariant'] in changed})}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--format', choices=('json', 'md'), default='md')
    parser.add_argument('--previous', type=Path, help='explicit public registry snapshot to compare; no Git history lookup')
    args = parser.parse_args()
    try:
        current = load(ROOT)
        report = validate(ROOT, current)
        if args.previous:
            previous = load(ROOT, args.previous)
            # Old source paths may have moved. Comparison does not claim that
            # the old registry is valid for the current tree.
            report['comparison'] = compare(previous, current)
    except (Invalid, OSError, ValueError, TypeError, KeyError, SyntaxError) as error:
        print(f'regression map invalid: {error}', file=sys.stderr)
        return 1
    if args.format == 'json':
        print(json.dumps(report, sort_keys=True))
    else:
        print('Regression selection only; tests and remote CI have not been executed by this command.')
        for key in ('protected', 'gaps', 'comparison'):
            if key in report:
                print(f'{key}: {json.dumps(report[key], sort_keys=True)}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
