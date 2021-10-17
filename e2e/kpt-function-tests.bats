#!/usr/bin/env bats

@test "kpt fn should run example/kpt/test-cases" {
	EXAMPLE_CHART_NAMESPACE="`pwd`/example/namespace"
	cd example/kpt/test-cases
	rm -rf output-local output-kustomization output-remote
	kpt fn run --network --mount "type=bind,source=$EXAMPLE_CHART_NAMESPACE,target=/source" .

	[ -f ./output-local.yaml ]
	[ -f ./output-kustomization/configmap_myconfiga.yaml ]
	[ -f ./output-kustomization/configmap_myconfigb.yaml ]
	[ -f ./output-kustomization/kustomization.yaml ]
	[ -f ./output-remote.yaml ]

	kustomize build ./output-kustomization | grep -q ' myconfiga'
}

@test "kpt fn should template cert-manager" {
	cd example/kpt
	rm -rf cert-manager/generated-kustomization cert-manager/static/generated-manifest.yaml
	kpt fn run --network ./cert-manager

	[ -f ./cert-manager/static/generated-manifest.yaml ]
	! grep -m1 -B10 -A1 ' namespace: ""' ./cert-manager/static/generated-manifest.yaml || (echo 'FAIL: output contains empty namespace field' >&2; false)
	grep -Eq ' name: cert-manager-webhook$' ./cert-manager/static/generated-manifest.yaml || (echo 'FAIL: does not contain expected resource' >&2; false)
	grep -Eq ' namespace: kube-system$' ./cert-manager/static/generated-manifest.yaml || (echo 'FAIL: did not preserve chart resource namespace' >&2; false)
	grep -Eq '^kind: CustomResourceDefinition$' ./cert-manager/static/generated-manifest.yaml || (echo 'FAIL: does not contain CustomResourceDefinition' >&2; false)
}

@test "kpt fn should template linkerd experiment" {
	EXAMPLE_DIR="example"
	GENERATED_MANIFEST="$EXAMPLE_DIR/kpt/linkerd/manifests/static/linkerd/generated-manifest.yaml"
	rm -f "$GENERATED_MANIFEST"
	make -C "$EXAMPLE_DIR/kpt/linkerd" update manifest

	[ -f "$GENERATED_MANIFEST" ]
	grep -Eq ' name: linkerd-config-tpl$' "$GENERATED_MANIFEST" || (echo 'FAIL: output does not contain linkerd-config-tpl' >&2; false)
}

@test "kpt fn should cache charts" {
	cd example/kpt/cache-dependencies
	make clean
	make manifest
	[ -f generated-manifests/manifest1.yaml ]
	[ -f generated-manifests/manifest1-from-cache.yaml ]
	[ -f generated-manifests/manifest2.yaml ]
	[ -f generated-manifests/manifest2-from-cache.yaml ]
}
