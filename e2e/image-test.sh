#!/bin/sh

IMAGE=mgoltzsche/khelm

echo
echo "  TEST $0: Run CLI without repositories.yaml"
echo

cd "$(dirname "$0")/../example"

set -ex

STATUS=0

DIR="$(mktemp -d)"
docker run --rm -v "$DIR:/out" $IMAGE template cert-manager \
	--version 1.0.4 \
	--repo https://charts.jetstack.io \
	--output /out/manifest.yaml \
	--debug || STATUS=1
[ $STATUS -eq 1 ] || [ -f "$DIR/manifest.yaml" ] || (echo 'fail: output not written to file' >&2; false) || STATUS=1
rm -rf "$DIR"

DIR="$(mktemp -d)"
docker run --rm -v "$DIR:/out" -v "$(pwd)/no-namespace:/chart" $IMAGE template ./chart \
	--output /out/ \
	--debug || STATUS=1
[ $STATUS -eq 1 ] || [ -f "$DIR/kustomization.yaml" ] || (echo 'fail: kustomization.yaml not written' >&2; false) || STATUS=1
[ $STATUS -eq 1 ] || [ -f "$DIR/configmap_release-name-myconfigb.yaml" ] || (echo 'fail: resource not written' >&2; false) || STATUS=1
[ $STATUS -eq 0 ] || ls -la "$DIR" >&2
rm -rf "$DIR"

[ $STATUS -eq 0 ] && echo SUCCESS || (echo FAIL >&2; false)
