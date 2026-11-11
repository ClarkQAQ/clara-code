#!/usr/bin/env bash
set -euo pipefail

: "${GH_REPO:?}" "${GITHUB_SHA:?}" "${VERSION:?}"
cd "$1"
assets=(
  clara-code-windows-amd64.zip
  clara-code-linux-amd64.tar.gz
  clara-code-linux-arm64.tar.gz
  clara-code-darwin-arm64.tar.gz
  SHA256SUMS
)
for asset in "${assets[@]}"; do
  test -s "$asset"
done
sha256sum --check --strict SHA256SUMS

fail() {
  echo "::error::$*" >&2
  exit 1
}

check_tag() {
  local tag_ref
  tag_ref="$(gh api "repos/$GH_REPO/git/matching-refs/tags/$VERSION" |
    jq -r --arg ref "refs/tags/$VERSION" '.[] | select(.ref == $ref) | .ref')"
  tag_commit=""
  if [[ -n "$tag_ref" ]]; then
    tag_commit="$(gh api "repos/$GH_REPO/commits/$tag_ref" --jq '.sha')"
    if [[ "$tag_commit" != "$GITHUB_SHA" ]]; then
      fail "Tag $VERSION points to $tag_commit, expected $GITHUB_SHA."
    fi
  fi
}

check_release_assets() {
  local release="$1" uploaded asset
  uploaded="$(jq -r '.assets[] | select(.state == "uploaded" and .size > 0) | .name' <<< "$release")"
  for asset in "${assets[@]}"; do
    if ! grep -Fxq -- "$asset" <<< "$uploaded"; then
      fail "Release $VERSION is missing the uploaded asset $asset."
    fi
  done
}

check_tag

# Drafts can be found by pending tag name through GraphQL, even before the Git tag exists.
release_id="$(gh api graphql \
  -f owner="${GH_REPO%/*}" -f repo="${GH_REPO#*/}" -f tag="$VERSION" \
  -f query='query($owner: String!, $repo: String!, $tag: String!) {
    repository(owner: $owner, name: $repo) {
      release(tagName: $tag) { databaseId }
    }
  }' --jq '.data.repository.release.databaseId // empty')"

if [[ -n "$release_id" ]]; then
  release="$(gh api "repos/$GH_REPO/releases/$release_id")"
  if [[ "$(jq -r '.draft' <<< "$release")" == false ]]; then
    if [[ -z "$tag_commit" ]]; then
      fail "Published release $VERSION has no Git tag."
    fi
    check_release_assets "$release"
    echo "Release $VERSION is already published."
    exit 0
  fi
  if [[ -z "$tag_commit" && "$(jq -r '.target_commitish' <<< "$release")" != "$GITHUB_SHA" ]]; then
    fail "Draft release $VERSION targets a different commit."
  fi
else
  gh release create "$VERSION" --draft --target "$GITHUB_SHA" --title "$VERSION" --generate-notes
fi

gh release upload "$VERSION" "${assets[@]}" --clobber
release="$(gh release view "$VERSION" --json assets)"
check_release_assets "$release"
check_tag

master_commit="$(gh api "repos/$GH_REPO/git/ref/heads/master" --jq '.object.sha')"
latest=false
if [[ "$master_commit" == "$GITHUB_SHA" ]]; then
  latest=true
fi
gh release edit "$VERSION" --draft=false --prerelease=false --title "$VERSION" --latest="$latest"
check_tag
if [[ -z "$tag_commit" ]]; then
  fail "Published release $VERSION has no Git tag."
fi
