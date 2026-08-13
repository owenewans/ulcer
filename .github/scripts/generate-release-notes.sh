#!/usr/bin/env bash
set -euo pipefail

usage() {
	printf 'usage: %s <current-tag> <output-file>\n' "${0##*/}" >&2
	exit 2
}

die() {
	printf 'generate-release-notes: %s\n' "$*" >&2
	exit 1
}

valid_login() {
	if [[ $1 == *'[bot]' ]]; then
		local base=${1%'[bot]'}
		[[ $base =~ ^[A-Za-z0-9][A-Za-z0-9-]{0,38}$ ]]
	else
		[[ $1 =~ ^[A-Za-z0-9][A-Za-z0-9-]{0,38}$ ]]
	fi
}

sanitize_change() {
	jq -nr --arg value "$1" '
        $value
        | explode
        | map(
            if (. == 9 or . == 10 or . == 13 or . == 133
                or . == 8232 or . == 8233)
            then 32
            elif (. < 32 or (. >= 127 and . <= 159)
                or (. >= 8234 and . <= 8238)
                or (. >= 8294 and . <= 8297) or . == 65279)
            then empty
            else .
            end
          )
        | implode
        | gsub(" +"; " ")
        | sub("^ "; "")
        | sub(" $"; "")
        | explode
        | map(
            . as $character
            | if ($character == 33 or $character == 35 or $character == 38
                or $character == 40 or $character == 41 or $character == 42
                or $character == 60 or $character == 62 or $character == 64
                or $character == 91 or $character == 92 or $character == 93
                or $character == 95 or $character == 96 or $character == 126)
              then "&#\($character);"
              else [$character] | implode
              end
          )
        | join("")
    '
}

commit_login() {
	local sha=$1 response login email candidate

	response=''
	if response=$(gh api "repos/${repository}/commits/${sha}" 2>/dev/null); then
		login=$(jq -r '.author.login // .committer.login // empty' <<<"$response" 2>/dev/null || true)
		if valid_login "$login"; then
			printf '%s\n' "$login"
			return
		fi
	fi

	email=$(git show --no-patch --format=%ae "$sha")
	case $email in
		*@users.noreply.github.com)
			candidate=${email%@users.noreply.github.com}
			candidate=${candidate#*+}
			if valid_login "$candidate"; then
				printf '%s\n' "$candidate"
				return
			fi
			;;
	esac

	printf 'unknown\n'
}

[[ $# -eq 2 ]] || usage

current=$1
output=$2
repository=${GITHUB_REPOSITORY:-owenewans/ulcer}
semver_pattern='^v[0-9]+\.[0-9]+\.[0-9]+$'

[[ $current =~ $semver_pattern ]] || die "invalid release tag: $current"
[[ $repository =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || die "invalid repository name"

for required_command in gh git jq; do
	command -v "$required_command" >/dev/null 2>&1 || die "$required_command is required"
done

if ! current_commit=$(git rev-parse --verify "refs/tags/${current}^{commit}" 2>/dev/null); then
	die "tag does not resolve to a commit: $current"
fi

previous=''
first_release=false
if parent_commit=$(git rev-parse --verify "${current_commit}^" 2>/dev/null); then
	while IFS= read -r candidate; do
		[[ $candidate =~ $semver_pattern ]] || continue
		if git merge-base --is-ancestor "$candidate" "$parent_commit"; then
			previous=$candidate
			break
		fi
	done < <(git tag --list --sort=-version:refname)
fi

if [[ -z $previous ]]; then
	mapfile -t root_commits < <(git rev-list --max-parents=0 --reverse "$current_commit")
	((${#root_commits[@]} > 0)) || die "could not determine the first commit"
	previous=${root_commits[0]}
	first_release=true
fi

if [[ $first_release == true ]]; then
	mapfile -t commits < <(git rev-list --reverse "$current_commit")
else
	mapfile -t commits < <(git rev-list --reverse "${previous}..${current_commit}")
fi
if ((${#commits[@]} == 0)); then
	commits=("$current_commit")
fi

declare -a changes=()
declare -a contributors=()
declare -A seen_contributors=()
declare -A seen_pull_requests=()
base_url="https://github.com/${repository}"

for sha in "${commits[@]}"; do
	change=$(git show --no-patch --format=%s "$sha")
	change=$(sanitize_change "$change")
	[[ -n $change ]] || change="Commit ${sha:0:12}"

	pr_number=''
	pr_login=''
	pr_title=''
	pulls=''
	if pulls=$(gh api "repos/${repository}/commits/${sha}/pulls" 2>/dev/null); then
		pr=$(jq -c --arg repository "$repository" '
            [
              .[]
              | select(.merged_at != null and .base.repo.full_name == $repository)
            ]
            | unique_by(.number)
            | if length == 1 then .[0] else empty end
        ' <<<"$pulls" 2>/dev/null || true)
		if [[ -n $pr ]]; then
			candidate_number=$(jq -r '.number // empty' <<<"$pr")
			candidate_login=$(jq -r '.user.login // empty' <<<"$pr")
			candidate_title=$(jq -r '.title // empty' <<<"$pr")
			if [[ $candidate_number =~ ^[0-9]+$ ]]; then
				pr_number=$candidate_number
			fi
			if valid_login "$candidate_login"; then
				pr_login=$candidate_login
			fi
			if [[ -n $candidate_title ]]; then
				pr_title=$(sanitize_change "$candidate_title")
			fi
		fi
	fi

	login=$pr_login
	[[ -n $login ]] || login=$(commit_login "$sha")
	valid_login "$login" || login=unknown

	if [[ -n $pr_number ]]; then
		if [[ -n ${seen_pull_requests[$pr_number]+present} ]]; then
			continue
		fi
		seen_pull_requests[$pr_number]=present
		[[ -z $pr_title ]] || change=$pr_title
		change_url="${base_url}/pull/${pr_number}"
	else
		change_url="${base_url}/commit/${sha}"
	fi
	changes+=("${change} by @${login} in ${change_url}")

	if [[ -z ${seen_contributors[$login]+present} ]]; then
		seen_contributors[$login]=present
		contributors+=("$login")
	fi
done

output_directory=$(dirname -- "$output")
[[ -d $output_directory ]] || die "output directory does not exist: $output_directory"
temporary_file=$(mktemp "${output_directory}/.release-notes.XXXXXX")
cleanup() {
	[[ -z ${temporary_file:-} ]] || rm -f -- "$temporary_file"
}
trap cleanup EXIT

{
	printf "## What's Changed\n"
	for change in "${changes[@]}"; do
		printf '* %s\n' "$change"
	done
	printf '\n##  Contributors\n'
	for login in "${contributors[@]}"; do
		printf '* @%s\n' "$login"
	done
	printf '\n**Full Changelog**: %s/compare/%s...%s\n' "$base_url" "$previous" "$current"
} >"$temporary_file"

mv -- "$temporary_file" "$output"
temporary_file=''
