# A synthetic JUnit runner: it writes to the location in its argument and
# fails with a marked message until the greeting says hello. The period after
# the marker is not part of its identifier.
if grep -qx hello src/greeting.txt; then
	printf '<?xml version="1.0" encoding="utf-8"?>\n<testsuites><testcase classname="tests.greeting" name="says hello"/></testsuites>\n' > "$1"
	exit 0
fi
printf '<?xml version="1.0" encoding="utf-8"?>\n<testsuites><testcase classname="tests.greeting" name="says hello"><failure message="jaekit:violation=greeting-missing. the greeting is hi">trace</failure></testcase></testsuites>\n' > "$1"
exit 1
