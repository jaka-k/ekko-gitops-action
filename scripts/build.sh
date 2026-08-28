#!/usr/bin/env bash
# Builds the per-platform binaries invoke-binary.js dispatches to and pins its
# VERSION constant to the current commit. Rerun (and commit the binaries)
# whenever the Go code changes — the runner only sees what's in the repo.
set -euo pipefail
cd "$(dirname "$0")/.."

sha=$(git rev-parse HEAD)

mkdir -p bin
rm -f bin/main-*
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  goos=${target%/*}
  goarch=${target#*/}
  echo "building bin/main-${goos}-${goarch}-${sha}"
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags='-s -w' \
    -o "bin/main-${goos}-${goarch}-${sha}" ./cmd
done

perl -pi -e "s/const VERSION = '.*'/const VERSION = '${sha}'/" invoke-binary.js
echo "VERSION set to ${sha}"
