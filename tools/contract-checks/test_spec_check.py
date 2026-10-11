"""Boundary contract and mutations of public synthetic expected documents."""
from pathlib import Path
import shutil
import tempfile
import unittest
import spec_check

checker = spec_check.checker
SUITE = "spec-boundaries"

class SpecContracts(unittest.TestCase):
    def test_registered_document_requirements(self):
        for criterion in checker.REQUIREMENTS[SUITE]:
            with self.subTest(criterion=criterion):
                observation, problems = checker.observe(checker.ROOT, SUITE, criterion)
                self.assertEqual(observation["status"], "pass", problems)

    def test_counterexamples_change_relationships_not_only_words(self):
        mutations = [
            ("| observation-draft | Draft |", "| observation-draft | Ready |"),
            ("| 요청 | 근거 불충분 |", "| 세션 | 근거 불충분 |"),
            ("| 사용자 답 A |", "| 현재 구현 |"),
            ("| 전체 목록 | 열거한 값만 허용 | 거부 |", "| 전체 목록 | 열거한 값만 허용 | 허용 |"),
            ("| cleanup-ready | Ready | 0 |", "| cleanup-ready | Ready | 4 |"),
            ("| 흐름 | 선택한 구버전 설치 자료가 정리된다 | AC-1 |", "| 흐름 | 선택한 구버전 설치 자료가 정리된다 | AC-9 |"),
            ("- **AC-2** 해당 요청에서 실제 실행한", "- **AC-3** 해당 요청에서 실제 실행한"),
            ("관측 단위는 요청인가 세션인가? 판독할 수 없는 입력은 어떤 결과인가?", "없음."),
            ("## Open Decisions\n없음.", "## Open Decisions\n관측 단위 미정."),
            ("판독 불가는 근거 불충분이고 다른 요청의 근거는 섞이지 않는다.", "판독 불가는 성공이고 다른 요청의 근거를 쓸 수 있다."),
            ("호출 전후 선택 범위 밖의 모든 파일은 추가·삭제·변경되지 않는다.", "호출 전후 선택 범위 밖의 파일도 삭제할 수 있다."),
            ("전체 목록 밖의 값과 판정 불가는 거부한다.", "전체 목록 밖의 값은 허용한다."),
            ("출처: 사용자 답 A.", "출처: 현재 코드."),
        ]
        paths = {path for clauses in checker.REQUIREMENTS[SUITE].values() for path, _ in clauses}
        for old, new in mutations:
            with self.subTest(mutation=old), tempfile.TemporaryDirectory() as directory:
                root=Path(directory)
                for path in paths:
                    destination=root/path
                    destination.parent.mkdir(parents=True,exist_ok=True)
                    shutil.copyfile(checker.ROOT/path,destination)
                path=root/'evals/spec-cases/decision-boundaries.md'
                source=path.read_text(); self.assertIn(old,source)
                path.write_text(source.replace(old,new))
                observation, problems=checker.observe(root,SUITE,'AC-7')
                self.assertEqual(observation['status'],'violation',problems)

if __name__ == '__main__':
    unittest.main()
