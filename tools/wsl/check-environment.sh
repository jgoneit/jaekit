#!/usr/bin/env bash
# Read-only diagnostics for the initial WSL2 route; no host or Skill attestation.
set -euo pipefail

fail() { printf 'WSL2 preparation: %s\n' "$*" >&2; exit 1; }
[[ $# -ge 1 && $# -le 2 ]] || fail 'usage: bash check-environment.sh PROJECT [HOST_COMMAND]'
[[ "$(uname -s)" == Linux ]] || fail 'run this in the intended WSL2 Linux terminal'
[[ "$(uname -r)" == *[Mm]icrosoft*WSL2* ]] || fail 'WSL2 kernel was not identified; inspect wsl --list --verbose in Windows'
[[ "$(uname -m)" == x86_64 ]] || fail 'this preparation route targets x86_64 only; ordinary Linux installation is unchanged'

# Parse data without sourcing /etc/os-release as executable shell code.
os_id='' os_version=''
while IFS='=' read -r key value; do
  value="${value#\"}"; value="${value%\"}"
  value="${value#\'}"; value="${value%\'}"
  case "$key" in ID) os_id="$value";; VERSION_ID) os_version="$value";; esac
done < <(cat /etc/os-release)
[[ "$os_id" == ubuntu && "$os_version" == 24.04 ]] || fail 'this preparation route targets Ubuntu 24.04; ordinary Linux installation is unchanged'

for utility in realpath stat; do
  command -v "$utility" >/dev/null || fail "missing Linux utility: $utility"
done
[[ -d "$1" ]] || fail 'project is not an existing directory'
project="$(realpath -e -- "$1")" || fail 'cannot resolve project path'
linux_home="$(realpath -e -- "$HOME")" || fail 'cannot resolve Linux home'
[[ "$project" == "$linux_home/"* ]] || fail 'put the project under the Linux home directory'

linux_path() {
  local path="$1" kind
  case "$path" in /mnt/*|*.exe|*.EXE) fail "Windows or mounted-drive path selected: $path";; esac
  kind="$(stat -f -c %T -- "$path")" || fail "cannot inspect filesystem: $path"
  # Container overlays and memory filesystems are not the distribution's own
  # Linux filesystem that this route covers.
  case "$kind" in ext2/ext3|ext4|btrfs|xfs) ;;
    *) fail "filesystem is outside the checked Linux route ($kind): $path";;
  esac
}
linux_path "$project"
printf 'Project: %s\n' "$project"

git_command=''
for tool in git ha "${2:-}"; do
  [[ -n "$tool" ]] || continue
  # Only resolve a command name or path, never eval it or invoke the host.
  selected="$(command -v -- "$tool")" || fail "missing Linux command: $tool"
  [[ -f "$selected" && -x "$selected" ]] || fail "not an executable file: $selected"
  selected="$(realpath -e -- "$selected")" || fail "cannot resolve command: $tool"
  linux_path "$selected"
  [[ "$tool" == git && -z "$git_command" ]] && git_command="$selected"
  printf '%s: %s\n' "$tool" "$selected"
done

# ha finds the code repository from the goal path. Ask the selected Linux Git,
# read-only, whether the project is inside a worktree rooted under home.
repository="$("$git_command" -C "$project" rev-parse --show-toplevel 2>/dev/null)" ||
  fail "Git does not recognize the project as a worktree: $project"
repository="$(realpath -e -- "$repository")" || fail 'cannot resolve the Git repository root'
[[ "$repository" == "$linux_home/"* ]] || fail 'put the Git repository root under the Linux home directory'
printf 'Repository: %s\n' "$repository"

printf '%s\n' 'WSL2 route prerequisites identified. No files or settings changed.' \
  'Host Skill loading and actual goal execution remain unverified.' \
  'Inspect ha --version and ha capabilities --format json before use.'
