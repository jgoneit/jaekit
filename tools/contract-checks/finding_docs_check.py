#!/usr/bin/env python3
"""Observe document-only post-completion discovery links, not Core state."""
import check as checker
P=checker.SEAL
checker.REQUIREMENTS['finding-docs']={
 'AC-1':[(P+'assets/templates/PROGRESS.md',('## 완료 뒤 발견','completion reference','target code','condition absent or unresolved','event time','recording time')),
         ('contracts/bundle.md',('발견 식별자','당시 완료 참조','조건 미포함·연결 미정','사건 시각','기록 시각'))],
 'AC-2':[(P+'SKILL.md',('historical completion','unresolved discoveries','rework request and progress','Do not overwrite')),
         ('contracts/bundle.md',('당시 완료·현재 발견·재작업','한곳의 상세','현재 결함이 없는 것처럼'))],
 'AC-3':[(P+'references/bundle.md',('candidate, confirmed, duplicate, dismissed, and resolved','new requirement from an existing-condition violation','correction target','not a unique confirmed-defect count')),
         ('contracts/bundle.md',('후보·확인·중복·기각·해결','정정 대상','고유한 확인 결함 수'))],
 'AC-4':[(P+'SKILL.md',('does not authorize rework','explicit user request','does not change the execution budget')),
         ('contracts/role-card.md',('발견을 적는 것만으로','재작업 요청','실행 예산'))],
 'AC-5':[(P+'references/bundle.md',('Seal 0.1.12','missing means unrecorded','Do not migrate','raw logs remain')),
         ('contracts/bundle.md',('Seal 0.1.12','미기록','원문 로그','구조화 조회'))],
 'AC-6':[(P+'SKILL.md',('document format','machine-verified finding state')),
         ('examples/verification-findings.md',('가상 형식 예시','실제 실행 기록이 아니다','## 당시 완료','## 발견 원본','## 후속 판단','## 현재 요약')),
         ('guides/OPERATIONS.md',('완료 뒤 발견','당시 완료','구조화 조회'))],
}
base_observe=checker.observe

def observe(root,suite,criterion):
 observation,problems=base_observe(root,suite,criterion)
 file='examples/verification-findings.md'
 completed=checker.rows(root,file,'## 당시 완료')
 expected_completion={'ID':'C1','목표':'example-goal','코드':'code-v1','완료 기록':'seq 18; synthetic-head','판정':'complete','근거':'R0','시각':'가상 2026-01-01T09:00:00+09:00'}
 if completed!=[expected_completion]:problems.append('historical completion changed or absent')
 originals=checker.rows(root,file,'## 발견 원본')
 expected={
  'F1':('C1','code-v1','AC-1','기존 조건 위반','후보','E1','미확인','미상'),
  'F2':('C1','code-v1','AC-1','기존 조건 위반','후보','E1b','미확인','미상'),
  'F3':('C1','code-v1','AC-2','기존 조건 위반','후보','E0','미확인','미상'),
  'F4':('C1','code-v1','조건 미포함','새 요구','후보','U2','미확인','미상'),
  'F5':('C1','code-v1','AC-2','기존 조건 위반','확인','E5','해당 버전의 합성 재현','미상'),
 }
 columns=('대상 완료','대상 코드','조건','종류','최초 상태','출처','재현 범위','사건 시각')
 if len(originals)!=5 or {r.get('ID') for r in originals}!=set(expected):problems.append('original finding identity missing or ambiguous')
 for row in originals:
  if tuple(row.get(c) for c in columns)!=expected.get(row.get('ID')) or row.get('기록 시각')!='가상 2026-01-02T09:00:00+09:00' or row.get('시각 출처')!='가상 기록 시계; 사건 시각 미상':problems.append('original target/condition/source/time provenance changed')
 judgments=checker.rows(root,file,'## 후속 판단')
 expected_judgments={
  'J1':('F1','후보','확인','F1 원본','없음','E2','없음','현재 수정 없음'),
  'J2':('F2','후보','중복','F2 원본','F1','E2','없음','동일 재현 조건'),
  'J3':('F3','후보','기각','F3 원본','없음','E0','없음','공개 계약의 허용 동작'),
  'J4':('F4','후보','확인','F4 원본','없음','U2','없음','새 요구 확인; 결함 아님'),
  'J5':('F1','확인','해결','J1','없음','E3','code-v2; check-v2','수정 범위 재확인; 다른 환경 미관측'),
 }
 cols=('발견','이전 상태','새 상태','정정 대상','기준 발견','근거','수정·재확인','판단 이유·한계')
 if len(judgments)!=5 or {r.get('ID') for r in judgments}!=set(expected_judgments):problems.append('append-only judgment history missing')
 states={r.get('ID'):r.get('최초 상태') for r in originals}
 for row in judgments:
  if tuple(row.get(c) for c in cols)!=expected_judgments.get(row.get('ID')):problems.append('judgment target/grounds/history/repair link incorrect')
  key=row.get('발견')
  if states.get(key)!=row.get('이전 상태'):problems.append('judgment does not continue the recorded prior state')
  states[key]=row.get('새 상태')
 requests=checker.rows(root,file,'## 재작업 연결')
 expected_requests=[{'발견':'F1','요청':'U1','원문 출처':'가상 사용자 요청 U1','진행':'완료','후속 코드':'code-v2','근거':'E3'}, {'발견':'F5','요청':'없음','원문 출처':'없음','진행':'미요청','후속 코드':'없음','근거':'없음'}]
 if requests!=expected_requests:problems.append('rework status or authority was invented or lost')
 summary=checker.rows(root,file,'## 현재 요약')
 expected_summary=[{'당시 완료':'C1 보존','미해결 기존 위반':'F5','새 요구':'F4','중복':'F2 → F1','기각':'F3','해결':'F1 → E3','재작업':'F1 완료(U1); F5 미요청'}]
 if summary!=expected_summary or states.get('F1')!='해결' or states.get('F5')!='확인':problems.append('current summary contradicts preserved history')
 evidence=checker.rows(root,file,'## 근거 연결')
 expected_evidence={
  'R0':('code-v1','AC-1, AC-2','C1','당시 검사'),
  'E1':('code-v1','AC-1','F1','미재현 제보'),
  'E1b':('code-v1','AC-1','F2','미재현 제보'),
  'E2':('code-v1','AC-1','F1, F2','해당 입력 재현'),
  'E0':('code-v1','AC-2','F3','공개 계약'),
  'U2':('미정','조건 미포함','F4','새 요구 원문'),
  'U1':('code-v1','AC-1','F1','명시적 재작업 요청'),
  'E3':('code-v2','AC-1','F1','check-v2 재확인'),
  'E5':('code-v1','AC-2','F5','해당 입력 재현'),
 }
 if len(evidence)!=len(expected_evidence) or {r.get('ID') for r in evidence}!=set(expected_evidence):problems.append('inspectable synthetic evidence reference absent')
 for row in evidence:
  if tuple(row.get(c) for c in ('코드','조건','대상','범위'))!=expected_evidence.get(row.get('ID')):problems.append('evidence version/condition/finding/reproduction scope inconsistent')
 path=root/file
 if path.exists():
  text=path.read_text()
  for clause in ('실제 실행 기록이 아니다','F4는 새 요구로 확인되었으며 기존 조건 위반 수에 넣지 않는다','F5는 code-v1의 미해결 발견이며 code-v2에서 재현했는지는 미관측','현재 결함이 없는 것처럼 설명하지 않는다','문서 표의 상태는 기계가 검증한 최신 상태가 아니다','가상 사용자 요청 U1: “F1을 수정해 주세요.”'):
   if clause not in text:problems.append('finding example lost its evidence/authority/scope boundary')
 if problems:observation.update(status='violation',violation=f'{suite}.{criterion}.missing-contract')
 return observation,problems
checker.observe=observe
if __name__=='__main__':raise SystemExit(checker.main())
