"""Internal selected-check worker; stdout is an internal result, not acceptance.

The public check.py entry verifies public source identity and creates the source
copy before calling this helper. Calling this worker alone is not that gate.
"""
import sys

# Load the named source before ordinary imports. A sibling json package or a
# check package must not execute before the checked reader establishes imports.
_check_path = (__file__.rpartition("/")[0] + "/" if "/" in __file__ else "") + "check.py"
check = type(sys)("check")
check.__file__ = _check_path
sys.modules["check"] = check
with open(_check_path, "rb") as _source:
    exec(compile(_source.read(), _check_path, "exec"), check.__dict__)

import json
from contextlib import ExitStack
from pathlib import Path

WORKER_BYTES = Path(__file__).read_bytes()


def main():
    try:
        request = json.load(sys.stdin)
        criterion = request['criterion']
        if criterion not in [f'AC-{number}' for number in range(1, 12)]:
            raise ValueError('unsupported selection')
        checker = check.Checker(request['evidence'])
        loaded = dict(check.LOADED_SOURCE, **{str(Path(__file__).resolve()): WORKER_BYTES})
        with ExitStack() as stack:
            stack.enter_context(check.source_identity.verified(check.ROOT, request['tree'], loaded))
            if int(criterion[3:]) >= 7:
                checker.product_root = Path(request['product_root'])
                stack.enter_context(check.source_identity.verified(checker.product_root, request['product_tree'], {}))
            getattr(checker, 'check_' + criterion[3:])()
        result = {'status': 'pass'}
    except (check.Failure, check.source_identity.SourceMismatch) as error:
        result = {'status': 'fail', 'reason': str(error)}
    except (check.Unavailable, check.source_identity.SourceUnavailable, OSError, ValueError, KeyError, TypeError) as error:
        result = {'status': 'unverified', 'reason': str(error) if isinstance(error, check.Unavailable) else 'evidence is missing or malformed'}
    print(json.dumps(result))
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
