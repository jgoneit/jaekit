"""Checkout-root Go package selection through the public static registry.

These fixtures have a real module root. The static reader must preserve Go's
./... layout boundary without launching Go or executing a test body.
"""
from integration_selection_go import GoFixture, HEADER, TEST, registry


class GoLayoutSelection(GoFixture):
    def setUp(self):
        super().setUp()
        (self.root / 'go.mod').write_text('module example.invalid/selection\n\ngo 1.23\n')

    def excluded(self, file):
        report, reason = self.observe(HEADER + TEST, file=file)
        if report is not None:
            self.fail('[violation:go-build-selection-mismatch] ./...-excluded package layout was accepted as protected')
        self.assertIsNotNone(reason)

    def test_hidden_and_testdata_directories_are_excluded(self):
        for directory in ('testdata', '_hidden', '.hidden', 'normal/testdata', 'normal/_hidden', 'normal/.hidden'):
            with self.subTest(directory=directory):
                self.excluded(directory + '/sample_test.go')

    def test_nested_modules_are_excluded(self):
        for directory in ('nested', 'normal/nested'):
            with self.subTest(directory=directory):
                nested = self.root / directory
                nested.mkdir(parents=True, exist_ok=True)
                (nested / 'go.mod').write_text('module example.invalid/nested\n\ngo 1.23\n')
                for suffix in ('sample_test.go', 'child/sample_test.go'):
                    with self.subTest(file=suffix):
                        self.excluded(directory + '/' + suffix)

    def test_vendor_package_is_selected_but_its_descendants_are_excluded(self):
        for directory in ('vendor', 'normal/vendor'):
            with self.subTest(directory=directory):
                report, reason = self.observe(HEADER + TEST, file=directory + '/sample_test.go')
                self.assertIsNone(reason)
                self.assertEqual(report['assurance'], 'selection-only')
                self.excluded(directory + '/inside/sample_test.go')

    def test_inside_checkout_directory_symlink_is_not_walked(self):
        file = 'linked/sample_test.go'
        report, reason = self.observe(HEADER + TEST, file=file)
        self.assertIsNone(reason)
        self.assertEqual(report['assurance'], 'selection-only')
        # The checkout root itself may be reached through an intentional host
        # alias; only descendant directory links are outside the Go walk.
        root_alias = self.root.parent / (self.root.name + '-alias')
        root_alias.symlink_to(self.root, target_is_directory=True)
        self.addCleanup(root_alias.unlink)
        self.assertEqual(self.module.validate(root_alias, registry(file))['assurance'], 'selection-only')
        # Retain the tracked logical file, then replace its ancestor by a
        # symlink. The real target remains inside the checkout but outside the
        # ./... walk, so other selected packages cannot substitute for it.
        (self.root / 'linked').rename(self.root / '_stored')
        (self.root / 'linked').symlink_to('_stored', target_is_directory=True)
        before = ((self.root / file).read_bytes(), (self.root / '.git/index').read_bytes())
        try:
            self.module.validate(self.root, registry(file))
        except self.module.Invalid:
            pass
        else:
            self.fail('[violation:go-build-selection-mismatch] an unwalked directory symlink established selection')
        self.assertEqual(((self.root / file).read_bytes(), (self.root / '.git/index').read_bytes()), before)


if __name__ == '__main__':
    import unittest
    unittest.main()
