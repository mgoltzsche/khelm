#!/bin/sh

cd "$(dirname "$0")/.."

echo
echo "  TEST $0: Run kustomize plugin"
echo

TMP_DIR="$(mktemp -d)"
STATUS=0
(
	PLUGIN_DIR=$TMP_DIR/kustomize/plugin/khelm.mgoltzsche.github.com/v2/chartrenderer
	set -ex
	mkdir -p $PLUGIN_DIR
	cp build/bin/khelm $PLUGIN_DIR/ChartRenderer
	chmod +x $PLUGIN_DIR/ChartRenderer
	export XDG_CONFIG_HOME=$TMP_DIR
	kustomize build --enable_alpha_plugins example/namespace | grep -q ' myconfiga'
) || STATUS=1
rm -rf $TMP_DIR

[ $STATUS -eq 0 ] && echo SUCCESS || (echo FAIL >&2; false)
