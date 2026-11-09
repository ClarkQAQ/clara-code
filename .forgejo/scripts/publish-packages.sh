#!/usr/bin/env bash
set -euo pipefail

: "${RUNNER_TEMP:?}" "${VERSION:?}" "${FORGEJO_URL:?}"
: "${FORGEJO_OWNER:?}" "${FORGEJO_REPO:?}" "${FORGEJO_USER:?}"
: "${HOMEBREW_TAP:?}" "${PUBLISH_TOKEN:?Configure the Forgejo Actions secret}"
registry="$FORGEJO_URL/api/packages/$FORGEJO_OWNER"
archive_url="$registry/generic/clara-code/$VERSION/clara-code-darwin-arm64.tar.gz"
tap_repo_url="$FORGEJO_URL/api/v1/repos/$FORGEJO_OWNER/$HOMEBREW_TAP"
tap_url="$tap_repo_url/contents/Formula/clara-code.rb"
auth="Authorization: token $PUBLISH_TOKEN"
package_dir="$RUNNER_TEMP/clara-code-packages"
work="$(mktemp -d "$RUNNER_TEMP/clara-code-publish.XXXXXX")"
trap 'rm -rf "$work"' EXIT

login="$(curl -fsS -H "$auth" "$FORGEJO_URL/api/v1/user" | jq -er '.login')"
if [[ "$login" != "$FORGEJO_USER" ]]; then
  echo "PUBLISH_TOKEN belongs to $login, expected $FORGEJO_USER" >&2
  exit 1
fi
tap_branch="$(curl -fsS -H "$auth" "$tap_repo_url" | jq -er '.default_branch')"

upload() {
  local url="$1" file="$2" status
  status="$(curl -sS -o "$work/response" -w '%{http_code}' \
    -X PUT -H "$auth" -H 'Content-Type: application/octet-stream' \
    --data-binary "@$file" "$url")"
  case "$status" in
    201) echo "Published ${file##*/}" ;;
    409) echo "Already published: ${file##*/}" ;;
    *)
      echo "Upload failed for ${file##*/}: HTTP $status" >&2
      cat "$work/response" >&2
      exit 1
      ;;
  esac
}

upload "$archive_url" "$package_dir/clara-code-darwin-arm64.tar.gz"
curl -fsS -H "$auth" "$archive_url" -o "$work/remote-archive"
checksum="$(sha256sum "$work/remote-archive")"
sed \
  -e "s|@ARCHIVE_URL@|$archive_url|g" \
  -e "s|@VERSION@|$VERSION|g" \
  -e "s|@SHA256@|${checksum%% *}|g" \
  -e "s|@HOMEPAGE@|$FORGEJO_URL/$FORGEJO_OWNER/$FORGEJO_REPO|g" \
  .forgejo/packaging/homebrew/clara-code.rb.template > "$work/formula.rb"
docker run --rm -i ruby:3.4-alpine ruby -c < "$work/formula.rb"

for arch in x86_64 aarch64; do
  upload "$registry/arch/clara-code" "$package_dir/clara-code-$VERSION-1-$arch.pkg.tar.zst"
done
curl -fsS --retry 5 --retry-all-errors --retry-delay 2 -H "$auth" \
  "$registry/arch/repository.key" -o /dev/null

status="$(curl -sS -o "$work/tap.json" -w '%{http_code}' -H "$auth" "$tap_url")"
case "$status" in
  200)
    sha="$(jq -er '.sha' "$work/tap.json")"
    jq -er '.content' "$work/tap.json" | base64 -d > "$work/current.rb"
    if cmp -s "$work/formula.rb" "$work/current.rb"; then
      echo "Homebrew formula already current"
      exit 0
    fi
    method=PUT
    ;;
  404)
    sha=""
    method=POST
    ;;
  *)
    echo "Failed to read Homebrew formula: HTTP $status" >&2
    exit 1
    ;;
esac

content="$(base64 < "$work/formula.rb" | tr -d '\n')"
payload="$(jq -n --arg content "$content" --arg message "clara-code $VERSION" \
  --arg branch "$tap_branch" --arg sha "$sha" \
  '{content: $content, message: $message, branch: $branch} +
   (if $sha == "" then {} else {sha: $sha} end)')"
curl -fsS -X "$method" -H "$auth" -H 'Content-Type: application/json' \
  -d "$payload" "$tap_url" -o /dev/null
echo "Updated $FORGEJO_OWNER/$HOMEBREW_TAP to Clara Code $VERSION"
