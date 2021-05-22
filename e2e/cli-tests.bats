#!/usr/bin/env bats

IMAGE=${IMAGE:-mgoltzsche/khelm:latest}
OUT_DIR="$(mktemp -d)"

teardown() {
	rm -rf $OUT_DIR
}

@test "CLI should template remote chart without repositories.yaml" {
	docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" "$IMAGE" template cert-manager \
		--version 1.0.4 \
		--repo https://charts.jetstack.io \
		--output /out/subdir/manifest.yaml \
		--debug
	ls -la $OUT_DIR/subdir
	[ -f "$OUT_DIR/subdir/manifest.yaml" ]
}

@test "CLI should output kustomization" {
	docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" -v "$(pwd)/example/namespace:/chart" "$IMAGE" template /chart \
		--output /out/kdir/ \
		--debug
	ls -la "$OUT_DIR" "$OUT_DIR/kdir" >&2
	[ -f "$OUT_DIR/kdir/kustomization.yaml" ]
	[ -f "$OUT_DIR/kdir/configmap_myconfigb.yaml" ]
}
