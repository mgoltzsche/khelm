#!/bin/sh

EXAMPLE_DIR="$(dirname "$0")/../example"

GENERATED_MANIFEST="$EXAMPLE_DIR/kpt/linkerd/manifests/static/linkerd/generated-manifest.yaml"

echo
echo "  TEST $0: Run kpt functions of example/kpt/linkerd"
echo

set -e

rm -f "$GENERATED_FILE"

(
	set -ex
	rm -f $GENERATED_MANIFEST
	make -C "$EXAMPLE_DIR/kpt/linkerd" update manifest

	[ -f "$GENERATED_MANIFEST" ]
	grep -Eq ' name: linkerd-config-tpl$' "$GENERATED_MANIFEST" || (echo 'FAIL: output does not contain linkerd-config-tpl' >&2; false)
)

echo SUCCESS
