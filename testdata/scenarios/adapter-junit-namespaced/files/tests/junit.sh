# A synthetic JUnit runner whose only failure in the linked testcase is an
# extension element with a namespace: it is not a JUnit failure.
printf '<?xml version="1.0" encoding="utf-8"?>\n<testsuites xmlns:ext="urn:example:ext"><testcase classname="tests.greeting" name="says hello"><ext:failure message="jaekit:violation=greeting-missing the greeting is hi"/></testcase></testsuites>\n' > "$1"
exit 1
