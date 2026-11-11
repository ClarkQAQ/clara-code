#!/usr/bin/env bash
set -euo pipefail

: "${VERSION:?}"
mkdir -p "$1"
assets_dir="$(cd "$1" && pwd)"
build_dir="$(mktemp -d)"
trap 'rm -r -- "$build_dir"' EXIT
host_os="$(go env GOHOSTOS)"
host_arch="$(go env GOHOSTARCH)"
archives=()

for platform in windows/amd64 linux/amd64 linux/arm64 darwin/arm64; do
  os="${platform%/*}"
  arch="${platform#*/}"
  package="clara-code-$os-$arch"
  stage="$build_dir/$package"
  mkdir "$stage"
  executable=clara-code
  if [[ "$os" == windows ]]; then
    executable+=.exe
  fi

  echo "Building $package ($VERSION)"
  binary="$stage/$executable"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -buildvcs=false -ldflags "-s -w -buildid= -X main.version=$VERSION" \
    -o "$binary" .

  metadata="$(go version -m "$binary")"
  for setting in "GOOS=$os" "GOARCH=$arch" "CGO_ENABLED=0"; do
    grep -Fxq -- $'\tbuild\t'"$setting" <<< "$metadata"
  done
  LC_ALL=C grep -aFq -- "$VERSION" "$binary"
  if [[ "$platform" == "$host_os/$host_arch" ]]; then
    test "$("$binary" --version)" = "Clara Code $VERSION"
  fi

  cp LICENSE NOTICE "$stage/"
  expected_contents="$(printf '%s\n' LICENSE NOTICE "$executable")"
  if [[ "$os" == windows ]]; then
    archive="$package.zip"
    (
      cd "$stage"
      zip -X -q "$assets_dir/$archive" LICENSE NOTICE "$executable"
    )
    unzip -tq "$assets_dir/$archive"
    test "$(unzip -Z -1 "$assets_dir/$archive")" = "$expected_contents"
    unzip -p "$assets_dir/$archive" "$executable" | cmp - "$binary"
  else
    archive="$package.tar.gz"
    COPYFILE_DISABLE=1 tar -czf "$assets_dir/$archive" -C "$stage" LICENSE NOTICE "$executable"
    test "$(tar -tzf "$assets_dir/$archive")" = "$expected_contents"
    tar -xOzf "$assets_dir/$archive" "$executable" | cmp - "$binary"
  fi
  archives+=("$archive")
done

cd "$assets_dir"
sha256sum "${archives[@]}" > SHA256SUMS
sha256sum --check --strict SHA256SUMS
