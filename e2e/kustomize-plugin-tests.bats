#!/usr/bin/env bats

set -eu

TMP_DIR="$(mktemp -d)"

teardown() {
	rm -rf $TMP_DIR
}

# ARGS: KUSTOMIZATION_DIR EXPECT_OUTPUT_CONTAINS
kustomizeBuild() {
	PLUGIN_DIR=$TMP_DIR/kustomize/plugin/khelm.mgoltzsche.github.com/v2/chartrenderer
	mkdir -p $PLUGIN_DIR
	cp build/bin/khelm $PLUGIN_DIR/ChartRenderer
	chmod +x $PLUGIN_DIR/ChartRenderer
	export XDG_CONFIG_HOME=$TMP_DIR
	kustomize build --enable-alpha-plugins "$1" | grep -q "$2" || (
		echo OUTPUT:
		kustomize build --enable-alpha-plugins "$1" || true
		echo FAIL: kustomize output does not contain "'$2'"
		false
	)
}

@test "kustomize plugin should template helm chart" {
	kustomizeBuild example/namespace ' myconfiga'
}

@test "kustomize vars can overwrite helm values" {
	kustomizeBuild example/kustomize-var/overlay ' value: changed by kustomization overlay'
}

@test "kustomize generator should support OCI registry" {
	kustomizeBuild example/oci-image 'app.kubernetes.io/name: karpenter'
}
