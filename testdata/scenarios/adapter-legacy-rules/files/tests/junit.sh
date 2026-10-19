# Exit 3 when the placeholder arrives as written, 4 when it became a path.
case "$1" in
"{HA_JUNIT_XML}") exit 3 ;;
esac
exit 4
