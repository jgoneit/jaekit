#!/usr/bin/env python3
"""Goal evidence for the public invariant registry and actual selected checks."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT=Path(__file__).resolve().parents[2]

class Violation(Exception):
    def __init__(self,code,detail):self.code,self.detail=code,detail

class Environment(Exception):
    def __init__(self,reason,detail):self.reason,self.detail=reason,detail

def run(argv):
    env={k:v for k,v in os.environ.items() if not k.startswith('HA_EVIDENCE_')}
    env['PYTHONDONTWRITEBYTECODE']='1'
    try:
        return subprocess.run(argv,cwd=ROOT,env=env,capture_output=True,text=True,timeout=1800)
    except (OSError,subprocess.TimeoutExpired) as error:
        raise Environment('execution_error',str(error)) from error

def tests(argv, kind):
    result=run(argv);output=result.stdout+result.stderr
    print('selected:',repr(argv),flush=True)
    # Setup, build, missing selection, errors and skipped cases cannot establish
    # an intended product violation. Keep their actual output in the raw log.
    if kind=='go':
        active=set();executed=set();finished=set();packages=set();passed_packages=set()
        for line in result.stdout.splitlines():
            try:event=json.loads(line)
            except ValueError as error:raise Environment('execution_error',output) from error
            if not isinstance(event,dict):raise Environment('execution_error',output)
            if set(event)-{'Time','Action','Package','Test','Elapsed','Output','OutputType','FailedBuild'}:
                raise Environment('execution_error',output)
            action=event.get('Action');package=event.get('Package');test=event.get('Test')
            if package in passed_packages or (action=='output' and not isinstance(event.get('Output'),str)):
                raise Environment('execution_error',output)
            if not isinstance(package,str) or not package or action not in ('start','run','pause','cont','output','pass'):
                raise Environment('execution_error',output)
            if action=='start':
                if test or package in packages:raise Environment('execution_error',output)
                packages.add(package)
            elif action=='run':
                key=(package,test)
                if package not in packages or not isinstance(test,str) or not test or key in executed:
                    raise Environment('execution_error',output)
                active.add(key);executed.add(key)
            elif action=='pass':
                if test:
                    key=(package,test)
                    if key not in active:raise Environment('execution_error',output)
                    active.remove(key);finished.add(key)
                else:
                    if package not in packages or package in passed_packages or any(key[0]==package for key in active):
                        raise Environment('execution_error',output)
                    passed_packages.add(package)
            elif action in ('pause','cont') and (package,test) not in active:
                raise Environment('execution_error',output)
        if not executed or active or executed!=finished or packages!=passed_packages:
            raise Environment('execution_error',output)
    elif kind=='verify':
        expected='PASS: '+(argv[-1] if argv[-1] in ('docs','go') else 'all')+' verification'
        if expected not in result.stdout:raise Environment('execution_error',output)
    if result.returncode:raise Environment('execution_error',output)

def python_tests(path, selector=''):
    with tempfile.TemporaryDirectory(prefix='regression-result-') as directory:
        report=Path(directory)/'result.json'
        result=run([sys.executable,str(ROOT/'tools/regression-checks/run_unittest.py'),
                    '--file',str(path),'--selector',selector,'--report',str(report)])
        print('selected Python:',path,selector,flush=True)
        if not report.is_file():raise Environment('execution_error',result.stdout+result.stderr)
        try:data=json.loads(report.read_text())
        except ValueError as error:raise Environment('execution_error','invalid unittest report') from error
        if (data.get('schema')!='regression-unittest/v1' or data.get('tests_run',0)<=0 or
            any(data.get(k,1)!=0 for k in ('failures','errors','skipped','expected_failures','unexpected_successes')) or result.returncode):
            raise Environment('execution_error',result.stdout+result.stderr)

def observe(n):
    registry=ROOT/'contracts/regressions.json'
    if not registry.exists():
        raise Violation('regression-map-missing','tracked checkout has no finite invariant/consumer registry')
    command=run([sys.executable,'tools/regression_map.py','--format','json'])
    if command.returncode:
        if command.returncode==1:
            raise Violation('regression-map-invalid',command.stderr)
        raise Environment('execution_error',command.stderr)
    try:report=json.loads(command.stdout)
    except ValueError as error:raise Environment('execution_error','invalid registry result JSON') from error
    if report.get('assurance')!='selection-only':
        raise Violation('regression-map-overclaims','static registry must not assert actual tests or remote CI passed')
    if n in (1,3,4):
        python_tests('tools/regression-checks/test_map.py')
        python_tests('tools/regression-checks/test_checker.py')
    if n==2:
        # The registry carries explicit finite selection. Run each unique
        # protected selector, including its positive and adverse fixtures.
        data=json.loads(registry.read_text());selected=set()
        for route in data['routes']:
            if route['status']!='protected':continue
            for check in route['checks']:
                selected.add((check['file'],check['selector']))
        for path,selector in sorted(selected):
            if path.endswith('.go'):
                package='./'+str(Path(path).parent)
                tests(['go','test','-json','-count=1',package,'-run','^'+selector+'$'],'go')
            else:
                python_tests(path,selector)
    if n in (3,4):
        tests([sys.executable,'tools/verify.py','docs'],'verify')
    if n==5:
        python_tests('tools/public-checks/test_tree.py')
        python_tests('tools/gap-checks/test_history_results.py','AC4')
        python_tests('tools/gap-checks/test_history_results.py','AC5')
    if n==6:
        tests([sys.executable,'tools/verify.py'],'verify')

def main():
    parser=argparse.ArgumentParser();parser.add_argument('--criterion',type=int,choices=range(1,7),required=True);args=parser.parse_args()
    result=dict(target='regression-contract',attempt=1,status='pass')
    try:observe(args.criterion)
    except Violation as error:
        result.update(status='violation',violation=error.code);print(error.detail,file=sys.stderr)
    except Environment as error:
        result.update(status='error',reason=error.reason);print(error.detail,file=sys.stderr)
    except Exception as error:
        result.update(status='error',reason='execution_error');print(repr(error),file=sys.stderr)
    if 'HA_EVIDENCE_PATH' in os.environ:
        report=dict(schema='check-result/v1',invocation=os.environ['HA_EVIDENCE_INVOCATION'],declaration_digest=os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],producer='jaekit-regression-map',producer_version='1',attempts_complete=True,observations=[result])
        Path(os.environ['HA_EVIDENCE_PATH']).write_text(json.dumps(report)+'\n')
    print('regression contract',args.criterion,result['status'])
    return 0 if result['status']=='pass' else 1

if __name__=='__main__':raise SystemExit(main())
