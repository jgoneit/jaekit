#!/bin/sh
# Emulates Git's detached automatic maintenance for the AC-6 checks.
#
# Every call runs the real Git unchanged. After a successful `commit` in a
# repository whose maintenance.auto is not false (on the command line or in
# the configuration that Git call reads), the next Git call writes
# objects/maintenance.lock into that repository's Git directory and appends
# the directory to "$JAEKIT_FAKE_GIT_STATE.log". That is what
# `git maintenance run --auto --detach`, started by the commit, does while it
# runs. Real Git's timing cannot be reproduced; this models the case in which
# the lock appears after the commit, during the next Git work.
real=${JAEKIT_REAL_GIT:?}
state=${JAEKIT_FAKE_GIT_STATE:?}

if [ -s "$state" ]; then
	pending=$(cat "$state")
	: >"$state"
	mkdir -p "$pending/objects" && : >"$pending/objects/maintenance.lock" && printf '%s\n' "$pending" >>"$state.log"
fi

"$real" "$@"
status=$?
[ "$status" -eq 0 ] || exit "$status"

command=
off=
while [ $# -gt 0 ]; do
	case $1 in
	-c)
		case $2 in
		maintenance.auto=false | maintenance.auto=no | maintenance.auto=off | maintenance.auto=0) off=1 ;;
		esac
		shift 2
		;;
	-C) shift 2 ;;
	-*) shift ;;
	*)
		command=$1
		break
		;;
	esac
done

if [ "$command" = commit ] && [ -z "$off" ]; then
	value=$("$real" config --type=bool --get maintenance.auto 2>/dev/null)
	if [ "$value" != false ]; then
		"$real" rev-parse --absolute-git-dir >"$state" 2>/dev/null || : >"$state"
	fi
fi
exit 0
