#!/usr/bin/env python3
"""Run selected public Python tests and report the runner's observed result."""
import argparse
import importlib.util
import json
from pathlib import Path
import unittest


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--file',required=True)
    parser.add_argument('--selector',default='')
    parser.add_argument('--report',required=True)
    args=parser.parse_args()
    spec=importlib.util.spec_from_file_location('selected_regression',args.file)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    loader=unittest.TestLoader()
    suite=loader.loadTestsFromName(args.selector,module) if args.selector else loader.loadTestsFromModule(module)
    result=unittest.TextTestRunner(verbosity=2).run(suite)
    data=dict(schema='regression-unittest/v1',tests_run=result.testsRun,
              failures=len(result.failures),errors=len(result.errors),skipped=len(result.skipped),
              expected_failures=len(result.expectedFailures),unexpected_successes=len(result.unexpectedSuccesses))
    Path(args.report).write_text(json.dumps(data)+'\n')
    return 0 if result.wasSuccessful() else 1


if __name__=='__main__':raise SystemExit(main())
