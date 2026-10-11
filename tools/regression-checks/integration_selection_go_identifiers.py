"""Go identifiers through the real registry, independent of compiler execution."""
from integration_selection_go import GoFixture, HEADER, TEST


class GoIdentifierDefinitions(GoFixture):
    def test_keywords_cannot_supply_package_import_or_parameter_identifiers(self):
        cases = (
            HEADER.replace('package sample', 'package func') + TEST,
            HEADER.replace('"testing"', 'func "testing"') + TEST.replace('*testing.T', '*func.T'),
            HEADER + TEST.replace('(t *testing.T)', '(func *testing.T)'),
        )
        for source in cases:
            with self.subTest(source=source):
                report, _ = self.observe(source)
                if report is not None:
                    self.fail('[violation:go-definition-not-real] a Go keyword was accepted as a declaration identifier')
