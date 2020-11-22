#!/bin/sh

IMAGE=mgoltzsche/helmr

echo
echo "  TEST $0: Run CLI without repositories.yaml"
echo

cd "$(dirname "$0")/../example"

set -ex

DIR="$(mktemp -d)"
STATUS=0
docker run --rm -v "$DIR:/out" $IMAGE template cert-manager \
	--version 1.0.4 \
	--repo https://charts.jetstack.io \
	--out-file /out/manifest.yaml || STATUS=1
[ $STATUS -eq 1 ] || [ -f "$DIR/manifest.yaml" ] || (echo 'fail: output not written to file' >&2; false) || STATUS=1
rm -rf "$DIR"
[ $STATUS -eq 0 ] && echo success || (echo fail >&2; false)
