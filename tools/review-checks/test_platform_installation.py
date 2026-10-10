"""Optional-environment acceptance entry: local shells plus isolated Linux shells.

Requires an existing Docker engine and locally available python:3.12-slim.
Only the disposable container installs zsh; no host configuration is changed.
Ordinary CI runs test_installation.py on its native OS instead.
"""
import json
import subprocess
import unittest

import test_installation
from test_installation import ROOT


class AC8(test_installation.AC8):
    def test_linux_bash_and_zsh_use_the_published_cleanup(self):
        program = """
import json, sys, unittest
sys.path.insert(0, '/repo/tools/review-checks')
from test_installation import AC7, AC8
suite = unittest.TestSuite(unittest.defaultTestLoader.loadTestsFromTestCase(c) for c in (AC7, AC8))
result = unittest.TextTestRunner(verbosity=2).run(suite)
print('LINUX_RESULT=' + json.dumps({'ran': result.testsRun, 'errors': len(result.errors),
    'skips': len(result.skipped), 'failures': len(result.failures)}))
"""
        # Apt output is separate from fixture evidence. An unavailable package
        # manager/container is an environment error, never an assertion result.
        script = ('apt-get update -qq >&2 && '
                  'apt-get install -y --no-install-recommends zsh >/dev/null && '
                  'exec python3 -c "$1"')
        argv = ["docker", "run", "--rm", "--pull=never",
                "--mount", f"type=bind,source={ROOT / 'guides'},target=/repo/guides,readonly",
                "--mount", f"type=bind,source={ROOT / 'tools/review-checks'},target=/repo/tools/review-checks,readonly",
                "-e", "PYTHONDONTWRITEBYTECODE=1", "-e", "DEBIAN_FRONTEND=noninteractive",
                "python:3.12-slim", "sh", "-c", script, "review-fixture", program]
        result = subprocess.run(argv, capture_output=True, text=True, timeout=240)
        print(result.stdout, end="")
        print(result.stderr, end="", file=__import__("sys").stderr)
        lines = [line for line in result.stdout.splitlines() if line.startswith("LINUX_RESULT=")]
        if result.returncode or len(lines) != 1:
            raise RuntimeError("Linux cleanup fixture did not execute successfully")
        summary = json.loads(lines[0].split("=", 1)[1])
        if summary["errors"] or summary["skips"] or summary["ran"] != 5:
            raise RuntimeError("Linux cleanup fixture had an error, skip or missing selection")
        self.assertEqual(summary["failures"], 0, "published cleanup violated its Linux policy")


if __name__ == "__main__":
    unittest.main()
