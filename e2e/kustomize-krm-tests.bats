#!/usr/bin/env bats

@test "kustomize Containerized KRM Function plugin should template cert-manager" {
	MANIFEST="$(kustomize build --enable-alpha-plugins --network ./example/kustomize-krm/cert-manager)"
	echo "$MANIFEST" | grep -q 'app: cert-manager'
}
