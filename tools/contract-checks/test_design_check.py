"""Mutations of policy, observation, counterexample, and version links."""
from pathlib import Path
import shutil,tempfile,unittest
import design_check
checker=design_check.checker
SUITE='design'
class DesignContracts(unittest.TestCase):
 def test_registered_contracts(self):
  for criterion in checker.REQUIREMENTS[SUITE]:
   with self.subTest(criterion=criterion):
    result,problems=checker.observe(checker.ROOT,SUITE,criterion);self.assertEqual(result['status'],'pass',problems)
 def test_mutations_do_not_turn_gaps_into_success(self):
  changes=[
   ('| none | pending | code-v1 |','| none | complete | code-v1 |'),
   ('| required:unavailable | none | pending |','| required:observed | none | complete |'),
   ('| code-v2 | check-v2 | R2 |','| code-v2 | check-v2 | R1 |'),
   ('| reproduced | 도달하지 않은 명령은 실행 근거가 아니다 | check-v2; code-v2; E2 |','| reproduced | 도달하지 않은 명령은 실행 근거가 아니다 | 위험 기록만 추가 |'),
   ('| rejected | 대상과 호출의 연결 유지 |','| reproduced | 대상과 호출의 연결 유지 |'),
   ('| 목표, 계약, 검사, 기대 근거 |','| 구현자 해법 설명만 |'),
   ('| low-risk | AC-4 | low |','| low-risk | AC-4 | high |'),
   ('E3의 fake 결과는 실제 모델 관측이 아니다','E3의 fake 결과는 실제 모델 관측이다'),
   ('| G2 | AC-2 | 실제 모델 호출 권한 없음 | 실제 응답 계약 관측 | 사용자 권한 범위 내 실제 경계 관측 |','| G2 | AC-2 | 실제 모델 호출 권한 없음 | 실제 응답 계약 관측 | |'),
   ('| R2 | AC-1 |','| R2 | AC-99 |'),
   ('| CP1 수정 재확인; 해당 성질·환경에 한정 | E2 |','| CP1 수정 재확인; 해당 성질·환경에 한정 | E999 |'),
   ('| CP1 수정 재확인; 해당 성질·환경에 한정 |','| |'),
   ('| G1 | AC-1 |','| G1 | AC-3 |'),
   ('| G2 | AC-2 |','| G2 | AC-4 |'),
   ('| AC-1; 문자열 존재를 실행으로 인정 |','| AC-99; 무관한 입력 |'),
   ('| exec true 뒤 ha 호출 텍스트 |','| |'),
  ]
  paths={p for clauses in checker.REQUIREMENTS[SUITE].values() for p,_ in clauses}
  for old,new in changes:
   with self.subTest(change=old),tempfile.TemporaryDirectory() as directory:
    root=Path(directory)
    for name in paths:
     dest=root/name;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(checker.ROOT/name,dest)
    path=root/'examples/verification-design.md';text=path.read_text();self.assertIn(old,text);path.write_text(text.replace(old,new))
    result,problems=checker.observe(root,SUITE,'AC-9');self.assertEqual(result['status'],'violation',problems)
if __name__=='__main__':unittest.main()
