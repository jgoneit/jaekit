"""Shared regression: a broken relation is reported only by the criteria that own it.

Each case changes one relation in a copy of the public documents and runs the
observer for every registered criterion. Owners must report a violation and
every other criterion must still pass. Shared relations list all owners.
"""
from pathlib import Path
import shutil
import tempfile


def copy_inputs(checker, suite, root):
    names = {name for clauses in checker.REQUIREMENTS[suite].values() for name, _ in clauses}
    for name in names:
        source = checker.ROOT / name
        if source.exists():
            destination = root / name
            destination.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(source, destination)


def assert_owned(test, checker, suite, document, cases):
    for old, new, owners in cases:
        with test.subTest(relation=old), tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            copy_inputs(checker, suite, root)
            path = root / document
            text = path.read_text(encoding="utf-8")
            test.assertIn(old, text)
            path.write_text(text.replace(old, new, 1), encoding="utf-8")
            for criterion in checker.REQUIREMENTS[suite]:
                observation, problems = checker.observe(root, suite, criterion)
                violated = observation["status"] == "violation"
                if violated and criterion not in owners:
                    test.fail(f"[violation:unrelated-relation-attributed] {suite} {criterion} reported {old!r}: {problems}")
                if not violated and criterion in owners:
                    test.fail(f"[violation:owned-relation-missed] {suite} {criterion} missed {old!r}")
