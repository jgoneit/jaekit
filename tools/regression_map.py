#!/usr/bin/env python3
"""Validate the finite public regression registry; this does not execute tests."""
from __future__ import annotations

import argparse
import ast
import fnmatch
import json
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[1]
INVARIANTS = {f'I{n}' for n in range(1, 6)}


class Invalid(ValueError):
    pass


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


def source(root, name):
    if not isinstance(name, str) or not name or Path(name).is_absolute() or '..' in Path(name).parts:
        raise Invalid(f'unsafe source path: {name!r}')
    path = root / name
    if path.is_symlink() or not path.is_file() or not path.resolve().is_relative_to(root.resolve()):
        raise Invalid(f'missing or nonregular source: {name}')
    return path


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


def selected_suites(root):
    """Read the supported unconditional run/literal-loop subset, without eval.

    Control flow that can hide a selected call is refused. Build-only with
    blocks are outside this selection parser; they are exercised by verify.py.
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
                    output.add(('<go>', './...'))
                if argv[1:4] == ['-m', 'unittest', 'discover']:
                    try:
                        output.add((argv[argv.index('-s') + 1], argv[argv.index('-p') + 1]))
                    except (ValueError, IndexError) as error:
                        raise Invalid('incomplete unittest discovery') from error
            elif not isinstance(node, ast.Pass):
                raise Invalid('unsupported verification statement')

    for group in result:
        functions = [n for n in tree.body if isinstance(n, ast.FunctionDef) and n.name == 'verify_' + group]
        if len(functions) != 1 or functions[0].decorator_list:
            raise Invalid('verification group missing, repeated, or decorated')
        statements(functions[0].body, {}, result[group])
    return result


def check_definition(root, check, selected):
    fields(check, 'file selector group', 'check')
    file, selector, group = (check[k] for k in ('file', 'selector', 'group'))
    path = source(root, file)
    if group not in selected or not isinstance(selector, str):
        raise Invalid('unknown group or selector')
    if file.endswith('_test.go'):
        if group != 'go' or ('<go>', './...') not in selected[group]:
            raise Invalid(f'Go regression not selected: {file}')
        if not re.fullmatch(r'Test[A-Za-z0-9_]+', selector) or not re.search(r'^func ' + re.escape(selector) + r'\(t \*testing\.T\)', path.read_text(), re.M):
            raise Invalid(f'Go test definition missing: {file}:{selector}')
    elif file.endswith('.py'):
        if not any(str(path.relative_to(root).parent) == directory and fnmatch.fnmatch(path.name, pattern)
                   for directory, pattern in selected[group]):
            raise Invalid(f'Python regression not selected: {file}')
        parts = selector.split('.')
        if len(parts) not in (1, 2) or not all(re.fullmatch(r'[A-Za-z_]\w*', p) for p in parts):
            raise Invalid(f'unsupported Python selector: {selector}')
        classes = [n for n in ast.parse(path.read_text()).body if isinstance(n, ast.ClassDef) and n.name == parts[0]]
        if len(classes) != 1:
            raise Invalid(f'Python class missing: {file}:{selector}')
        names = [n.name for n in classes[0].body if isinstance(n, ast.FunctionDef) and n.name.startswith('test_')]
        if not names or (len(parts) == 2 and parts[1] not in names):
            raise Invalid(f'Python test missing: {file}:{selector}')
    else:
        raise Invalid(f'unsupported test type: {file}')


def validate(root, data):
    fields(data, 'schema invariants routes', 'registry')
    if data['schema'] != 'regression-map/v1':
        raise Invalid('unsupported registry schema')
    selected = selected_suites(root)
    invariants = set()
    for row in data['invariants']:
        fields(row, 'id basis property', 'invariant')
        if row['id'] in invariants or row['id'] not in INVARIANTS:
            raise Invalid('unknown or duplicate invariant')
        invariants.add(row['id'])
        strings(row['basis'], 'basis')
        for basis in row['basis']:
            source(root, basis)
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
            source(root, name)
        for key in ('property', 'boundary'):
            if not isinstance(route[key], str) or not route[key].strip():
                raise Invalid(f'missing route {key}')
        if route['status'] == 'protected':
            if not isinstance(route['checks'], list) or not route['checks']:
                raise Invalid('protection requires selected checks')
            for check in route['checks']:
                check_definition(root, check, selected)
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
    return dict(schema='regression-map/v1', assurance='selection-only', invariants=sorted(invariants),
                protected=sorted(protected), gaps=sorted(gaps))


def compare(before, after):
    """Surface removal, demotion, scope edits and narrower checks for review."""
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
    return {'lost_protection': sorted(lost),
            'added_routes': sorted(set(current) - {r['id'] for r in before['routes']})}


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
