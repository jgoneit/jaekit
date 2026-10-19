# Synthetic go test -json events of one Go 1.27 test: a marked error until
# the greeting says hello. Standard error is not read. The marker is followed
# by a colon, which is not part of its identifier; an attr event changes nothing.
echo "go: downloading example.com/x v1.0.0" >&2
run='{"Action":"run","Package":"example.com/greet","Test":"TestGreets"}
{"Action":"attr","Package":"example.com/greet","Test":"TestGreets","Key":"area","Value":"greeting"}'
if grep -qx hello src/greeting.txt; then
	printf '%s\n' "$run" '{"Action":"pass","Package":"example.com/greet","Test":"TestGreets"}' '{"Action":"pass","Package":"example.com/greet"}'
	exit 0
fi
printf '%s\n' "$run" '{"Action":"output","Package":"example.com/greet","Test":"TestGreets","Output":"    greet_test.go:9: jaekit:violation=greeting-missing: the greeting is hi\n","OutputType":"error"}' '{"Action":"fail","Package":"example.com/greet","Test":"TestGreets"}' '{"Action":"fail","Package":"example.com/greet"}'
exit 1
