import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("go_result", Path(__file__).with_name("go-result.py"))
producer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(producer)


def event(action, test="TestBehavior", output=""):
    return {"Package": "example", "Test": test, "Action": action, "Output": output}


class Classification(unittest.TestCase):
    def test_assertion_requires_execution_and_explicit_marker(self):
        self.assertEqual(producer.classify([
            event("run"), event("output", output="[requirement] wrong result"), event("fail")
        ], 1), ("violation", None))
        self.assertEqual(producer.classify([event("run"), event("fail")], 1), ("error", "setup_error"))

    def test_build_missing_skipped_and_crashed_are_not_baselines(self):
        for events, code in (([], 1), ([], 0), ([event("run"), event("skip")], 0),
                             ([event("run")], 1), ([event("fail")], 1),
                             ([event("run"), event("output", output="[requirement] panic:"), event("fail")], 1)):
            self.assertNotEqual(producer.classify(events, code)[0], "violation")

    def test_mixed_environment_failure_is_not_hidden(self):
        events = [event("run", "TestParent"), event("run", "TestParent/assert"),
                  event("output", "TestParent/assert", "[requirement] wrong"),
                  event("fail", "TestParent/assert"), event("run", "TestParent/setup"),
                  event("fail", "TestParent/setup"), event("fail", "TestParent")]
        self.assertEqual(producer.classify(events, 1), ("error", "setup_error"))

    def test_passing_and_failed_parent_summary(self):
        self.assertEqual(producer.classify([event("run"), event("pass")], 0), ("pass", None))
        self.assertEqual(producer.classify([
            event("run", "TestParent"), event("run", "TestParent/case"),
            event("output", "TestParent/case", "[requirement] wrong"),
            event("fail", "TestParent/case"), event("fail", "TestParent")
        ], 1), ("violation", None))

    def test_parent_fixture_and_package_crash_override_child_assertion(self):
        events = [event("run", "TestParent"), event("run", "TestParent/case"),
                  event("output", "TestParent/case", "[requirement] wrong"),
                  event("fail", "TestParent/case")]
        self.assertEqual(producer.classify(events + [
            event("output", "TestParent", "fixture_test.go:42: missing dependency"),
            event("fail", "TestParent")], 1), ("error", "setup_error"))
        self.assertEqual(producer.classify(events + [
            event("output", "TestParent", "assert_test.go:8: [requirement] wrong result"),
            event("output", "TestParent", "fixture_test.go:42: missing dependency"),
            event("fail", "TestParent")], 1), ("error", "setup_error"))
        self.assertEqual(producer.classify(events + [event("fail", "TestParent"),
            event("output", "", "panic: cleanup failed")], 1), ("error", "execution_error"))


if __name__ == "__main__":
    unittest.main()
