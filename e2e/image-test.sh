#!/bin/sh

IMAGE=mgoltzsche/helmr

echo '##'
echo "# TEST $0: Run CLI twice with initially empty dir and default repository policy"
echo '##'

cd "$(dirname "$0")/../example"

set -ex

DIR="$(mktemp -d)"
(
docker run --rm \
	--mount "type=bind,source=$DIR,target=/helm" \
	$IMAGE template cert-manager --version 1.0.4 --repo https://charts.jetstack.io &&
docker run --rm \
	--mount "type=bind,source=$DIR,target=/helm" \
	$IMAGE template cert-manager --version 1.0.4 --repo https://charts.jetstack.io
)
STATUS=$?
rm -rf "$DIR" || true
exit $STATUS
