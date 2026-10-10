#!/usr/bin/env python3
"""Observe the public findings CLI against synthetic local repositories.

No declarations are read to choose observed violation identifiers. A runnable
CLI lacking this command is distinct from build, setup, and process errors.
"""
from __future__ import annotations
import argparse
import copy
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]

class Violation(Exception):
    def __init__(self, code, detail):
        self.code, self.detail = code, detail

class Environment(Exception):
    def __init__(self, reason, detail):
        self.reason, self.detail = reason, detail

def require(value, code, detail):
    if not value:
        raise Violation(code, detail)

def run(argv, cwd, env=None):
    try:
        return subprocess.run([str(x) for x in argv], cwd=cwd, env=env,
                              text=True, capture_output=True, timeout=120)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Environment('execution_error', str(error)) from error

def decode(result):
    try:
        return json.loads(result.stdout)
    except ValueError as error:
        raise Violation('findings-invalid-json', result.stdout + result.stderr) from error

class Fixture:
    def __init__(self, binary, root):
        self.binary, self.root = binary, root
        self.env = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1',
                        GIT_AUTHOR_NAME='synthetic', GIT_COMMITTER_NAME='synthetic',
                        GIT_AUTHOR_EMAIL='synthetic@example.invalid', GIT_COMMITTER_EMAIL='synthetic@example.invalid')
        for name in ('GIT_DIR', 'GIT_WORK_TREE', 'GIT_INDEX_FILE', 'GIT_COMMON_DIR'):
            self.env.pop(name, None)
        self.goal = 'docs/specs/synthetic'
        g = root / self.goal
        g.mkdir(parents=True)
        (root/'src').mkdir()
        (root/'src/value').write_text('stable\n')
        (g/'SPEC.md').write_text('# Synthetic\n\nStatus: Ready\nCriteria-Format: nested/1\n\n## Acceptance Criteria\n\n- **AC-1** The synthetic check remains runnable.\n\n## Open Decisions\n\nNone.\n')
        (g/'PLAN.md').write_text('''# PLAN

## 조건표

| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | maintain | `python3 -c "pass"` | — | T001 |

## 범위

- 바꿀 수 있는 경로: `src/**`
- 테스트 경로: (기본값)
- 유지할 동작: synthetic

## Task

| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | Synthetic | AC-1 | — | — | 예 | — |

## 예산

- 검증 실행: 10
- 경과 시간: 4시간
- 비용: 미관측
''')
        (g/'REVIEW.md').write_text('# REVIEW\n\n## 결론\n\nready\n')
        (g/'PROGRESS.md').write_text('# PROGRESS\n\n## Task 상태\n\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | synthetic |\n')
        for argv in (['git','init','-q','-b','main'],['git','add','.'],['git','commit','-qm','synthetic']):
            r=run(argv,root,self.env)
            if r.returncode:
                raise Environment('setup_error',r.stderr)
        for args in (['start',self.goal,'--request','Synthetic fixture','--skill-name','synthetic','--skill-version','1'],['check',self.goal],['done',self.goal,'--format','json']):
            r=self.cli(*args)
            if r.returncode:
                raise Environment('setup_error',r.stdout+r.stderr)
        try:
            self.done=json.loads(r.stdout)['completion_record']
        except (ValueError, KeyError, TypeError) as error:
            raise Environment('setup_error','fixture completion output invalid') from error
        self.original=self.records.read_bytes()
        self.done_ref=self.ref(self.done)

    @property
    def records(self):
        return self.root/self.goal/'runs.jsonl'

    def cli(self,*args,binary=None):
        return run([binary or self.binary,*args],self.root,self.env)

    def lines(self):
        return [json.loads(x) for x in self.records.read_text().splitlines()]

    def ref(self,seq):
        raw=self.records.read_bytes().splitlines()[seq-1]
        return {'seq':seq,'sha256':hashlib.sha256(raw).hexdigest()}

    def request(self,id='one'):
        return dict(id=id,request_id='create-'+id,revision=0,completion=copy.deepcopy(self.done_ref),
                    criteria=['AC-1'],mapping='linked',status='candidate',category='existing_condition',
                    summary='Synthetic boundary finding',source='synthetic-review',evidence=[])

    def mutate(self,data,ok=True):
        # Input is deliberately outside the repository and is never copied to output.
        with tempfile.NamedTemporaryFile(mode='w',suffix='.json') as inp:
            json.dump(data,inp);inp.flush()
            before=self.records.read_bytes()
            r=self.cli('finding',self.goal,'--input',inp.name)
        if ok:
            require(r.returncode==0,'findings-write-rejected',r.stdout+r.stderr)
            return decode(r)
        require(r.returncode!=0,'findings-invalid-write-accepted',str(data))
        require(before==self.records.read_bytes(),'findings-rejected-write-mutated','record changed')
        return r

    def report(self):
        r=self.cli('finding',self.goal,'--format','json')
        require(r.returncode==0,'findings-read-rejected',r.stdout+r.stderr)
        return decode(r)

    def status(self):
        r=self.cli('status',self.goal,'--format','json')
        require(r.returncode==0,'findings-changed-completion',r.stdout+r.stderr)
        return decode(r)

    def update(self,change,status='confirmed'):
        nxt=copy.deepcopy(change)
        nxt.update(request_id='update-'+change['id']+'-'+status,revision=self.report()['findings'][0]['revision'],
                   status=status,reason='Synthetic follow-up',evidence=['synthetic:reproduction'])
        return nxt

def observe(binary,n,root,legacy):
    f=Fixture(binary,root)
    item=f.request()
    if n==1:
        f.mutate(item)
        view=f.report()['findings'][0]
        require(view['change']['completion']==f.done_ref and view['change']['criteria']==['AC-1'],'findings-anchor-lost',str(view))
        require(view['commit']==f.lines()[f.done-1]['commit'],'findings-code-lost',str(view))
        bad=f.request('bad');bad['completion']['sha256']='0'*64;f.mutate(bad,False)
        bad=f.request('other');bad['criteria']=['AC-99'];f.mutate(bad,False)
    elif n==2:
        f.mutate(item);new=f.update(item);new['event_at']='2000-01-01T00:00:00Z';f.mutate(new)
        view=f.report()['findings'][0]
        require(len(view['history'])==2 and view['history'][0]['change']['status']=='candidate','findings-history-lost',str(view))
        require(view['change']['event_at']!=view['history'][-1]['at'],'findings-times-conflated',str(view))
        bad=f.update(new,'dismissed');bad['reason']='';f.mutate(bad,False)
    elif n==3:
        f.mutate(item);f.mutate(f.update(item))
        s=f.status();md=f.cli('status',f.goal).stdout
        require(s['completion_record']==f.done and s['findings']['confirmed']==1,'findings-current-risk-hidden',str(s))
        require('unresolved' in md and 'confirmed' in md and 'not_requested' in md,'findings-human-risk-hidden',md)
    elif n==4:
        f.mutate(item);other=f.request('two');f.mutate(other)
        new=f.update(item,'duplicate');new['duplicate_of']='two';f.mutate(new)
        two=copy.deepcopy(other);two.update(request_id='dup-two',revision=f.report()['findings'][1]['revision'],status='duplicate',duplicate_of='one',reason='duplicate',evidence=['synthetic:review']);f.mutate(two,False)
        resolved=f.update(new,'resolved');resolved.pop('duplicate_of',None);resolved['resolution_refs']=['synthetic:fix1','synthetic:fix2'];f.mutate(resolved)
        contradicted=f.update(resolved,'confirmed');contradicted['request_id']='contradiction';contradicted.pop('resolution_refs');f.mutate(contradicted)
        require(len(f.report()['findings'][0]['history'])==4,'findings-correction-lost','history')
    elif n==5:
        before=f.status();f.mutate(item);after=f.status()
        require(before['budget']==after['budget'] and before['completion_record']==after['completion_record'],'findings-reopened-without-request','state changed')
        r=f.cli('note',f.goal,'reopen','--quote','Synthetic explicit rework');require(r.returncode==0,'findings-reopen-rejected',r.stderr)
        seq=f.lines()[-1]['seq'];new=f.update(item);new['rework']=f.ref(seq);f.mutate(new)
        require(f.report()['findings'][0]['rework_status']=='requested','findings-rework-link-lost','rework')
        f.cli('done',f.goal)
        require(f.report()['findings'][0]['rework_status']=='completed','findings-rework-result-lost','done')
    elif n==6:
        receipt=f.mutate(item);before=f.records.read_bytes();again=f.mutate(item)
        require(before==f.records.read_bytes() and receipt['seq']==again['seq'],'findings-retry-duplicated','request id')
        first=f.update(item);stale=copy.deepcopy(first);stale.update(request_id='stale',status='dismissed')
        with tempfile.TemporaryDirectory() as inputs:
            paths=[]
            for i,data in enumerate((first,stale)):
                path=Path(inputs)/str(i);path.write_text(json.dumps(data));paths.append(path)
            with ThreadPoolExecutor(max_workers=2) as pool:
                results=list(pool.map(lambda path:f.cli('finding',f.goal,'--input',path),paths))
            require(sorted(r.returncode for r in results)==[0,65],'findings-concurrent-conflict-lost',str(results))
            paths=[]
            for name in ('two','three'):
                path=Path(inputs)/name;path.write_text(json.dumps(f.request(name)));paths.append(path)
            with ThreadPoolExecutor(max_workers=2) as pool:
                results=list(pool.map(lambda path:f.cli('finding',f.goal,'--input',path),paths))
            require(all(r.returncode==0 for r in results) and len(f.report()['findings'])==3,'findings-concurrent-registration-lost',str(results))
        collision=f.request();collision['summary']='different';f.mutate(collision,False)
        f.records.write_bytes(f.records.read_bytes()[:-5])
        r=f.cli('finding',f.goal,'--format','json');require(r.returncode!=0,'findings-corruption-hidden',r.stdout)
        f.mutate(f.request('after-corruption'),False)
    elif n==7:
        f.mutate(item);other=f.request('two');other.update(mapping='unmapped',criteria=[],category='unknown');f.mutate(other)
        new=f.update(item,'dismissed');f.mutate(new)
        report=f.report();require(report['candidate']==1 and report['confirmed']==0 and report['unresolved']==1,'findings-counts-conflated',str(report))
        require(report['findings'][1]['change']['mapping']=='unmapped','findings-mapping-lost',str(report))
    elif n==8:
        f.mutate(item)
        require(f.records.read_bytes().startswith(f.original),'findings-history-rewritten','prefix changed')
        last=f.lines()[-1]
        require(last['schema']=='run-finding/v1','findings-legacy-silent-ignore','new event uses legacy schema')
        if legacy:
            before=f.records.read_bytes()
            for args in (['status',f.goal],['done',f.goal],['note',f.goal,'input','--quote','synthetic'],['check',f.goal]):
                r=f.cli(*args,binary=legacy)
                require(r.returncode!=0 and ('schema' in r.stdout+r.stderr or 'record' in r.stdout+r.stderr),'findings-legacy-accepted',r.stdout+r.stderr)
                require(before==f.records.read_bytes(),'findings-legacy-mutated','old binary changed data')
    elif n==9:
        f.mutate(item);a=f.cli('finding',f.goal,'--format','json');b=f.cli('finding',f.goal,'--format','json')
        require(a.stdout==b.stdout,'findings-nondeterministic','query changed')
        require(f.status()['findings']==decode(a),'findings-status-diverged','standalone versus status')
        require(f.records.read_bytes().startswith(f.original),'findings-save-roundtrip-lost','original')
    elif n==10:
        before=f.status();item['event_at']='2099-01-01T00:00:00Z';f.mutate(item)
        # Only this synthetic fixture's clock is changed; no real run records are edited.
        last=f.lines()[-1];last['at']='2099-01-01T00:00:00Z'
        raw=f.records.read_bytes().splitlines();raw[-1]=json.dumps(last,separators=(',', ':')).encode();f.records.write_bytes(b'\n'.join(raw)+b'\n')
        after=f.status();require(before['budget']==after['budget'] and before['usage']==after['usage'],'findings-budget-interference','budget or usage')
        f.cli('note',f.goal,'reopen','--quote','Synthetic rework')
        before=decode(f.cli('status',f.goal,'--format','json'));new=f.update(item);f.mutate(new)
        after=decode(f.cli('status',f.goal,'--format','json'))
        require(before['budget']==after['budget'],'findings-reopened-budget-interference','reopened budget')
    elif n==11:
        item['document']={'ref':'synthetic:document','recorded_at':'2020-01-01T00:00:00Z','event_at':''}
        f.mutate(item);v=f.report()['findings'][0]
        require(v['change']['status']=='candidate' and v['rework_status']=='not_requested','findings-document-promoted','document became fact')
        bad=f.request('fake');bad['document']=item['document'];bad['status']='confirmed';f.mutate(bad,False)
        bad=f.update(item);bad['evidence']=[];f.mutate(bad,False)
    else:
        raise Environment('setup_error','unknown criterion')

def main():
    p=argparse.ArgumentParser();p.add_argument('--criterion',default='all');p.add_argument('--binary');p.add_argument('--legacy');a=p.parse_args()
    result={'target':'finding-contract','attempt':1,'status':'pass'}
    try:
        with tempfile.TemporaryDirectory(prefix='jaekit-findings-') as tmp:
            temp=Path(tmp);binary=Path(a.binary).resolve() if a.binary else temp/'ha'
            if not a.binary:
                build=run(['go','build','-o',binary,'./cmd/ha'],ROOT)
                if build.returncode:
                    raise Environment('compile_error',build.stderr)
            cap=run([binary,'capabilities','--format','json'],ROOT)
            if cap.returncode:
                raise Environment('setup_error','normal CLI capabilities failed: '+cap.stderr)
            capabilities=decode(cap)
            probe=run([binary,'finding','--help'],ROOT)
            if 'unknown command "finding"' in probe.stderr and 'post-completion-findings/v1' not in capabilities.get('features',[]):
                raise Violation('findings-command-unavailable','runnable CLI has no finding command')
            criteria=range(1,12) if a.criterion=='all' else [int(a.criterion)]
            for n in criteria:
                root=temp/str(n);root.mkdir();observe(binary,n,root,a.legacy)
            print('Observed findings properties:',a.criterion)
    except Violation as error:
        result.update(status='violation',violation=error.code);print(error.code+': '+error.detail,file=sys.stderr)
    except Environment as error:
        result.update(status='error',reason=error.reason);print(error.detail,file=sys.stderr)
    except Exception as error:
        result.update(status='error',reason='execution_error');print(repr(error),file=sys.stderr)
    if 'HA_EVIDENCE_PATH' in os.environ:
        report={'schema':'check-result/v1','invocation':os.environ['HA_EVIDENCE_INVOCATION'],
                'declaration_digest':os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],
                'producer':'jaekit-findings','producer_version':'1','attempts_complete':True,'observations':[result]}
        Path(os.environ['HA_EVIDENCE_PATH']).write_text(json.dumps(report)+'\n')
    return 0 if result['status']=='pass' else 1

if __name__=='__main__':
    raise SystemExit(main())
