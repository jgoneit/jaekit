# Synthetic go test -json events of a marked failure, then a background
# process that keeps standard output open after the command exits 1. ha stops
# reading after its drain bound; the cut-short output is never evidence.
printf '%s\n' '{"Action":"run","Package":"example.com/greet","Test":"TestGreets"}' '{"Action":"output","Package":"example.com/greet","Test":"TestGreets","Output":"    greet_test.go:9: jaekit:violation=greeting-missing the greeting is hi\n","OutputType":"error"}' '{"Action":"fail","Package":"example.com/greet","Test":"TestGreets"}' '{"Action":"fail","Package":"example.com/greet"}'
sleep 8 &
exit 1
