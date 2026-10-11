"""Historical completion, discovery, correction, authority and evidence links."""
from pathlib import Path
import shutil,tempfile,unittest
import finding_docs_check
checker=finding_docs_check.checker
SUITE='finding-docs'
class FindingDocuments(unittest.TestCase):
 def test_registered_contracts(self):
  for criterion in checker.REQUIREMENTS[SUITE]:
   with self.subTest(criterion=criterion):
    result,problems=checker.observe(checker.ROOT,SUITE,criterion);self.assertEqual(result['status'],'pass',problems)
 def test_false_history_and_summary_are_detected(self):
  mutations=[
   ('| C1 | example-goal | code-v1 |','| C1 | example-goal | code-v2 |'),
   ('| F1 | C1 | code-v1 | AC-1 | 기존 조건 위반 | 후보 |','| F1 | C1 | code-v1 | AC-1 | 기존 조건 위반 | 확인 |'),
   ('| F4 | C1 | code-v1 | 조건 미포함 | 새 요구 |','| F4 | C1 | code-v1 | AC-3 | 기존 조건 위반 |'),
   ('| 미상 | 가상 2026-01-02T09:00:00+09:00 |','| 가상 2026-01-02T09:00:00+09:00 | 가상 2026-01-02T09:00:00+09:00 |'),
   ('| F2 원본 | F1 | E2 |','| F2 원본 | F99 | E2 |'),
   ('| J1 | 없음 | E3 | code-v2; check-v2 |','| J1 | 없음 | 없음 | 없음 |'),
   ('| F5 | 없음 | 없음 | 미요청 |','| F5 | U9 | 가상 자동 요청 | 진행 |'),
   ('| C1 보존 | F5 |','| C1 보존 | 없음 |'),
   ('| E3 | code-v2 | AC-1 | F1 |','| E3 | code-v1 | AC-99 | F5 |'),
   ('문서 표의 상태는 기계가 검증한 최신 상태가 아니다','문서 표의 상태는 기계가 검증한 최신 상태다'),
  ]
  paths={p for clauses in checker.REQUIREMENTS[SUITE].values() for p,_ in clauses}
  for old,new in mutations:
   with self.subTest(mutation=old),tempfile.TemporaryDirectory() as directory:
    root=Path(directory)
    for name in paths:
     dest=root/name;dest.parent.mkdir(parents=True,exist_ok=True);shutil.copyfile(checker.ROOT/name,dest)
    path=root/'examples/verification-findings.md';text=path.read_text();self.assertIn(old,text);path.write_text(text.replace(old,new))
    result,problems=checker.observe(root,SUITE,'AC-6');self.assertEqual(result['status'],'violation',problems)
if __name__=='__main__':unittest.main()
