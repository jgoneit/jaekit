#!/usr/bin/env python3
"""Observe explicit assurance policy contracts and linked synthetic cases."""
import check as checker
import re
P=checker.SEAL
checker.REQUIREMENTS['design']={
 'AC-1':[(P+'SKILL.md',('selected assurance policy','required conditions and source','Risk alone does not mandate','may not downgrade')),
         ('contracts/bundle.md',('보증 정책의 출처','위험도만으로','잘못된 성공의 영향','조건 그룹'))],
 'AC-2':[(P+'references/bundle.md',('same property, input scope, and environment','A reachable real system alone','structured original evidence','fake')),
         ('contracts/bundle.md',('같은 성질·입력 범위·환경','실제 시스템이 존재한다는 이유만으로','구조화된 원본'))],
 'AC-3':[(P+'SKILL.md',('initial contract, check, and expected-result grounds','implementation and integration code','not a different model or certified identity')),
         (P+'assets/templates/REVIEW.md',('review role','provided context','code and check versions','not performed'))],
 'AC-4':[(P+'references/bundle.md',('candidate from reproduced counterexample','reporting risk alone','scope-changing proposal')),
         ('contracts/bundle.md',('보고의 위험 문구만 추가','재현·기각·수정','원래 조건 위반'))],
 'AC-5':[(P+'references/bundle.md',('general property','unobserved, environment error, skip, and observed violation','not successful evidence or an intended baseline violation','product allow/deny policy')),
         ('contracts/bundle.md',('일반 성질','판정 불가','미관측·환경 오류·skip·실제 위반'))],
 'AC-6':[(P+'SKILL.md',('keep completion pending','cause, affected conditions, missing evidence, and resolution','independent useful work','optional unobserved comparison')),
         (P+'assets/templates/PROGRESS.md',('required assurance gaps','affected conditions','completion pending')),
         ('contracts/role-card.md',('필수 보증이 빠진 조건','완료 보류','권한·예산'))],
 'AC-7':[(P+'references/bundle.md',('goal, checks, code, grounds, environment, or assurance policy changes','preserve the reviewed versions','do not reuse')),
         ('contracts/bundle.md',('검토 대상 버전','새 적용 범위','소급'))],
 'AC-9':[(P+'assets/templates/REVIEW.md',('Assurance policy','counterexample','required or optional','policy source')),
         ('examples/verification-design.md',('가상 형식 예시','실제 실행 기록이 아니다','## 보증 연결','## 검토 내역','## 반례 내역')),
         ('guides/OPERATIONS.md',('명시한 보증 정책','선택 비교','완료 보류'))],
}
base_observe=checker.observe

def observe(root,suite,criterion):
 observation,problems=base_observe(root,suite,criterion)
 cases=checker.rows(root,'examples/verification-design.md','## 보증 연결')
 expected={
  'counterexample-fixed':('AC-1','high','goal-policy-A','required:performed','required:observed','reproduced:fixed','complete','code-v2','check-v2','R2','E2'),
  'false-positive':('AC-1','high','goal-policy-A','required:performed','required:observed','rejected:contract-permits','complete','code-v2','check-v2','R2','E2'),
  'review-unavailable':('AC-1','high','goal-policy-A','required:unavailable','required:observed','none','pending','code-v1','check-v1','none','G1'),
  'real-unavailable':('AC-2','high','goal-policy-A','optional:not-performed','required:unavailable','none','pending','code-v1','check-v1','none','G2'),
  'optional-comparison':('AC-3','high','project-policy-B','optional:not-performed','optional:unobserved','none','complete','code-v1','check-v1','none','E3'),
  'low-risk':('AC-4','low','goal-policy-C','not-required','not-required','none','complete','code-v1','check-v1','none','E4'),
 }
 columns=('조건','위험','정책 출처','독립 검토','실제 비교','반례 결과','완료','코드','검사','검토','근거')
 if len(cases)!=len(expected) or {row.get('사례') for row in cases}!=set(expected): problems.append('assurance cases absent or identity ambiguous')
 for row in cases:
  if tuple(row.get(column) for column in columns)!=expected.get(row.get('사례')): problems.append('requiredness/observation/completion links changed')
 reviews=checker.rows(root,'examples/verification-design.md','## 검토 내역')
 if len(reviews)!=2 or {r.get('ID') for r in reviews}!={'R1','R2'}: problems.append('separate versioned reviews absent')
 for row in reviews:
  version='v1' if row.get('ID')=='R1' else 'v2'
  if row.get('코드')!='code-'+version or row.get('검사')!='check-'+version or row.get('초기 입력')!='목표, 계약, 검사, 기대 근거' or row.get('추가 맥락')!='해당 버전 구현·통합 코드' or row.get('역할')!='별도 반례 검토자': problems.append('review version/context/role link incorrect')
  expected_result='CP1 후보와 CP2 후보; 전체 셸 지원 보장 아님' if version=='v1' else 'CP1 수정 재확인; 해당 성질·환경에 한정'
  if row.get('조건')!='AC-1' or row.get('근거')!=('E1' if version=='v1' else 'E2') or row.get('결과·한계')!=expected_result: problems.append('review condition/evidence/result/limits disagree with summary')
 counterexamples=checker.rows(root,'examples/verification-design.md','## 반례 내역')
 if len(counterexamples)!=2: problems.append('counterexample and false-positive distinction absent')
 else:
  by_id={r.get('ID'):r for r in counterexamples}
  if by_id.get('CP1',{}).get('확인')!='reproduced' or by_id.get('CP1',{}).get('처리')!='check-v2; code-v2; E2' or by_id.get('CP1',{}).get('일반 성질')!='도달하지 않은 명령은 실행 근거가 아니다': problems.append('confirmed violation not linked to general regression and repair')
  if by_id.get('CP2',{}).get('확인')!='rejected' or by_id.get('CP2',{}).get('처리')!='공개 계약의 허용 입력; E1': problems.append('false positive lacks rejection grounds')
  for identifier,fields in {
   'CP1':('exec true 뒤 ha 호출 텍스트','AC-1; 문자열 존재를 실행으로 인정','지원 밖 셸 문법은 판정 불가'),
   'CP2':('허용된 대상 ID의 실제 실행','AC-1 위반 의심이나 공개 계약은 허용','다른 대상 ID는 별도 경계')}.items():
   row=by_id.get(identifier,{})
   if tuple(row.get(c) for c in ('입력·상황','예상 위반·검사의 공백','남은 범위'))!=fields: problems.append('counterexample input/condition/gap differs from summarized observation')
 gaps=checker.rows(root,'examples/verification-design.md','## 부족한 필수 근거')
 if len(gaps)!=2 or {r.get('ID') for r in gaps}!={'G1','G2'} or any(not all(r.get(c) for c in ('영향 조건','원인','부족한 근거','해소 조건','계속할 작업')) for r in gaps): problems.append('required gap cause/condition/resolution absent')
 pending={r.get('근거'):r.get('조건') for r in cases if r.get('완료')=='pending'}
 for gap in gaps:
  if gap.get('영향 조건')!=pending.get(gap.get('ID')): problems.append('gap targets a different condition than the required pending work')
 path=root/'examples/verification-design.md'
 if path.exists():
  source=path.read_text()
  for clause in ('실제 실행 기록이 아니다','E2는 R2·check-v2·code-v2의 가상 근거다','E3의 fake 결과는 실제 모델 관측이 아니다','R1을 v2의 검토로 재사용하지 않는다','확인된 위반을 위험 문구만 추가하여 닫지 않는다'):
   if clause not in source: problems.append('synthetic assurance example lost scope/decision clause')
 if problems: observation.update(status='violation',violation=f'{suite}.{criterion}.missing-contract')
 return observation,problems
checker.observe=observe
if __name__=='__main__': raise SystemExit(checker.main())
