#!/usr/bin/env bash
# Build release archives and checksums into dist/.
# Usage: scripts/build-release.sh v0.1.0
set -euo pipefail

version="${1:?usage: $0 vMAJOR.MINOR.PATCH[-pre]}"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]]; then
  echo "invalid version: $version" >&2
  exit 1
fi

cd "$(dirname "$0")/.."
rm -rf dist && mkdir dist

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os="${target%/*}" arch="${target#*/}"
  name="stackmedic_${version#v}_${os}_${arch}"
  bin="stackmedic"; [[ "$os" == windows ]] && bin="stackmedic.exe"
  mkdir "dist/$name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" \
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o "dist/$name/$bin" ./cmd/stackmedic
  cp LICENSE README.md "dist/$name/"
  if [[ "$os" == windows ]]; then
    (cd dist && zip -qr "$name.zip" "$name")
  else
    tar -C dist -czf "dist/$name.tar.gz" "$name"
  fi
  rm -rf "dist/$name"
done

(cd dist && sha256sum -- *.tar.gz *.zip > checksums.txt)
ls -1 dist
