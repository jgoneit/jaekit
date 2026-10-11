"""Explicit project/goal and original record-prefix bindings for observations."""
import hashlib
import json
import os
import re
from pathlib import Path, PurePosixPath
import subprocess


def digest(data):
    return hashlib.sha256(data).hexdigest()


def context(project, goal):
    if not isinstance(project, str) or not Path(project).is_absolute() or not isinstance(goal, str) or not goal:
        raise ValueError('absolute project and relative goal must be explicit')
    root = Path(project).resolve(strict=True)
    relative = PurePosixPath(goal)
    if not root.is_dir() or relative.is_absolute() or not relative.parts or '..' in relative.parts:
        raise ValueError('project and relative goal must be explicit')
    target = (root / goal).resolve(strict=True)
    if not target.is_dir() or not target.is_relative_to(root):
        raise ValueError('goal is outside its project')
    env = {key: value for key, value in os.environ.items() if not key.startswith('GIT_')}
    env['GIT_OPTIONAL_LOCKS'] = '0'
    result = subprocess.run(['git', '-C', str(root), 'rev-parse', '--show-toplevel'],
                            env=env, capture_output=True, timeout=30)
    if result.returncode or Path(os.fsdecode(result.stdout).strip()).resolve() != root:
        raise ValueError('project is not the selected Git root')
    return {'project': str(root), 'goal': target.relative_to(root).as_posix(),
            'records': str(target / 'runs.jsonl')}


def prefix(path):
    try:
        raw = Path(path).read_bytes()
    except FileNotFoundError:
        return {'exists': False, 'size': 0, 'sha256': digest(b''), 'seq': 0, 'head': None}
    lines = raw.splitlines()
    if raw and not raw.endswith(b'\n'):
        raise ValueError('record prefix is incomplete')
    return {'exists': True, 'size': len(raw), 'sha256': digest(raw), 'seq': len(lines),
            'head': digest(lines[-1]) if lines else None}


def observed_prefix(path):
    try:
        return prefix(path)
    except (OSError, ValueError):
        return {'unavailable': True}


def match_prefix(value, raw):
    if not isinstance(value, dict) or set(value) != {'exists', 'size', 'sha256', 'seq', 'head'}:
        raise ValueError('original record prefix was not observed')
    size = value['size']
    if type(size) is not int or size < 0 or size > len(raw) or type(value['exists']) is not bool:
        raise ValueError('original record prefix length is invalid')
    part = raw[:size]
    lines = part.splitlines()
    if ((size and not part.endswith(b'\n')) or value['sha256'] != digest(part)
            or value['seq'] != len(lines) or value['head'] != (digest(lines[-1]) if lines else None)
            or (not value['exists'] and size)):
        raise ValueError('observed record prefix differs from the original goal records')
    return len(lines)


def records(raw):
    if not raw or not raw.endswith(b'\n'):
        raise ValueError('original goal record chain is incomplete')
    output, previous = [], None
    for number, line in enumerate(raw.splitlines(), 1):
        value = json.loads(line)
        if not isinstance(value, dict) or value.get('schema') != 'run/v1' or value.get('seq') != number or value.get('prev') != previous:
            raise ValueError('original goal record chain is invalid')
        output.append(value)
        previous = digest(line)
    return output


def invocation(arguments, cwd, expected):
    if not arguments or not isinstance(cwd, str) or Path(cwd).resolve() != Path(expected['project']):
        raise ValueError('actual Core cwd differs from the selected project')
    operation = arguments[0]
    if operation in {'--version', '--help', 'help', 'capabilities'}:
        return operation, [], False
    value_flags = {
        'start': {'request', 'skill', 'skill-name', 'skill-version', 'host-name', 'host-version', 'model'},
        'note': {'quote', 'cause'}, 'status': {'format'}, 'done': {'format'}, 'log': {'tail'},
        'estimate': {'format'}, 'budget': {'runs', 'from', 'window', 'revision', 'quote', 'context', 'reason', 'interpretation'},
        'check': set(), 'lint': set(),
    }
    if operation not in value_flags:
        raise ValueError('unsupported Core operation provenance')
    positions, baseline, index = [], False, 1
    while index < len(arguments):
        word = arguments[index]
        index += 1
        if word == '--':
            positions.extend(arguments[index:])
            break
        if not word.startswith('--'):
            positions.append(word)
            continue
        name, equal, value = word[2:].partition('=')
        if name == 'baseline' and operation in {'check', 'estimate'} and not equal:
            baseline = True
        elif name in value_flags[operation]:
            if not equal:
                if index == len(arguments):
                    raise ValueError('Core option value is missing')
                index += 1
        else:
            raise ValueError('unsupported Core option provenance')
    if not positions:
        raise ValueError('actual Core goal argument is missing')
    goal = positions[0]
    actual = Path(goal) if Path(goal).is_absolute() else Path(cwd) / goal
    if actual.resolve() != Path(expected['project']) / expected['goal']:
        raise ValueError('actual Core invocation names a different goal')
    return operation, positions[1:], baseline


def check_targets(names, observed):
    if not names:
        # The actual Core chooses required default criteria. Every appended
        # record remains bound, while current Core status checks completeness.
        targets = [row.get('target') for row in observed]
        return all(isinstance(target, dict) and set(target) == {'criterion'}
                   and isinstance(target['criterion'], str) and re.fullmatch(r'(?:AC|EX)-[0-9]+', target['criterion'])
                   for target in targets) and len({target['criterion'] for target in targets}) == len(targets)
    cursor = 0
    for name in names:
        if re.fullmatch(r'T[0-9]+', name):
            count = 0
            while cursor < len(observed) and observed[cursor].get('target') == {'task': name, 'index': count + 1}:
                count += 1
                cursor += 1
            if not count:
                return False
        elif re.fullmatch(r'(?:AC|EX)-[0-9]+', name):
            if cursor == len(observed) or observed[cursor].get('target') != {'criterion': name}:
                return False
            cursor += 1
        else:
            return False
    return cursor == len(observed)


def record_delta(start, end, raw, chain, expected):
    operation, names, baseline = invocation(start['argv'][1:], start.get('cwd', ''), expected)
    if end.get('cwd') != start.get('cwd'):
        raise ValueError('Core completion cwd differs')
    left, right = match_prefix(start.get('records_before'), raw), match_prefix(end.get('records_after'), raw)
    if right < left:
        raise ValueError('original record history went backwards')
    if operation not in {'start', 'check', 'done'}:
        return
    if right <= left or (operation != 'check' and right != left + 1):
        raise ValueError('required Core call did not append the original record sequence')
    observed = chain[left:right]
    kind = ('baseline' if baseline else 'check') if operation == 'check' else operation
    if any(record.get('kind') != kind for record in observed):
        raise ValueError('required Core call appended a different record kind')
    if operation == 'start' and (left != 0 or end.get('returncode') != 0):
        raise ValueError('observed start did not create this goal history')
    if operation == 'check':
        if not check_targets(names, observed):
            raise ValueError('Core check and original record targets differ')
        expected_return = 0 if all(record.get('result') in {'pass', 'fail_as_expected'} for record in observed) else 1
        if end.get('returncode') != expected_return:
            raise ValueError('Core check exit and recorded results differ')
    if operation == 'done' and (end.get('returncode') != 0 or observed[0].get('status') != 'complete' or right != len(chain)):
        raise ValueError('observed completion is not the current original completion record')
    if operation == 'done' and any(item.get('kind') == 'done' for item in chain[:left]):
        raise ValueError('previous completion cannot supply a fresh observed goal flow')
