#!/bin/sh

EXAMPLE_DIR="$(dirname "$0")/../example"

GENERATED_FILE="$EXAMPLE_DIR/kpt/linkerd/manifests/static/generated-manifest.yaml"

echo
echo "  TEST $0: Run kpt functions of example/kpt/linkerd"
echo

set -e

rm -f "$GENERATED_FILE"

(
	set -ex
	make -C "$EXAMPLE_DIR/kpt/linkerd" update generate

	[ -f "$GENERATED_FILE" ]
	grep -Eq ' name: linkerd-config-tpl$' "$GENERATED_FILE" || (echo 'FAIL: output does not contain linkerd-config-tpl' >&2; false)
)

echo SUCCESS
