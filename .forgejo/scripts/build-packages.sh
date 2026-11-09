#!/usr/bin/env bash
set -euo pipefail

: "${RUNNER_TEMP:?}" "${VERSION:?}" "${SOURCE_DATE_EPOCH:?}"
docker info >/dev/null

context="$(mktemp -d "$RUNNER_TEMP/clara-code-build.XXXXXX")"
package_dir="$RUNNER_TEMP/clara-code-packages"
image="clara-code-package:$VERSION-${context##*.}"
container=""
cleanup() {
  if [[ -n "$container" ]]; then
    docker rm "$container" >/dev/null || true
  fi
  docker image rm "$image" >/dev/null 2>&1 || true
  rm -rf "$context"
}
trap cleanup EXIT
mkdir -p "$package_dir" "$context/homebrew"

build() {
  local os="$1" arch="$2" output="$3"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -buildvcs=false -ldflags "-s -w -buildid= -X main.version=$VERSION" \
    -o "$output" .

  local metadata
  metadata="$(go version -m "$output")"
  grep -Fq "GOOS=$os" <<< "$metadata"
  grep -Fq "GOARCH=$arch" <<< "$metadata"
  grep -Fq "CGO_ENABLED=0" <<< "$metadata"
}

build linux amd64 "$context/clara-code-linux-amd64"
build linux arm64 "$context/clara-code-linux-arm64"
build darwin arm64 "$context/homebrew/clara-code"

cp LICENSE NOTICE "$context/"
cp LICENSE NOTICE "$context/homebrew/"
tar --sort=name --mtime="@$SOURCE_DATE_EPOCH" --owner=0 --group=0 --numeric-owner \
  -C "$context/homebrew" -cf - LICENSE NOTICE clara-code \
  | gzip -n > "$package_dir/clara-code-darwin-arm64.tar.gz"
tar -tzf "$package_dir/clara-code-darwin-arm64.tar.gz"
rm -r "$context/homebrew"

cp packaging/arch/Dockerfile packaging/arch/PKGBUILD "$context/"
docker build --platform linux/amd64 --tag "$image" \
  --build-arg "VERSION=$VERSION" \
  --build-arg "SOURCE_DATE_EPOCH=$SOURCE_DATE_EPOCH" "$context"
container="$(docker create --platform linux/amd64 "$image" /usr/bin/true)"
for arch in x86_64 aarch64; do
  package="clara-code-$VERSION-1-$arch.pkg.tar.zst"
  docker cp "$container:/work/out/$package" "$package_dir/$package"
done
