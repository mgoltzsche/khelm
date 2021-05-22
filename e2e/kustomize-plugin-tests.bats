#!/usr/bin/env bats

TMP_DIR="$(mktemp -d)"

teardown() {
	rm -rf $TMP_DIR
}

@test "kustomize plugin should template helm chart" {
	PLUGIN_DIR=$TMP_DIR/kustomize/plugin/khelm.mgoltzsche.github.com/v1/chartrenderer
	mkdir -p $PLUGIN_DIR
	cp build/bin/khelm $PLUGIN_DIR/ChartRenderer
	chmod +x $PLUGIN_DIR/ChartRenderer
	export XDG_CONFIG_HOME=$TMP_DIR
	kustomize build --enable_alpha_plugins example/namespace | grep -q ' myconfiga'
}
