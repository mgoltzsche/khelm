#!/usr/bin/env bats

bats_require_minimum_version 1.5.0

: ${IMAGE:=mgoltzsche/khelm:latest}

EXAMPLE_DIR="$(pwd)/example"
OUT_DIR="$(mktemp -d)"

teardown() {
	rm -rf $OUT_DIR
}

@test "CLI should template remote chart without repositories.yaml" {
	docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" "$IMAGE" template cert-manager \
		--name=myrelease \
		--version 1.0.4 \
		--repo https://charts.jetstack.io \
		--output /out/subdir/manifest.yaml \
		--debug
	ls -la $OUT_DIR/subdir
	[ -f "$OUT_DIR/subdir/manifest.yaml" ]
	grep -q myrelease "$OUT_DIR/subdir/manifest.yaml"
}

@test "CLI should reject repository when not in repositories.yaml and trust-any disabled" {
	run -1 docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" -e KHELM_TRUST_ANY_REPO=false "$IMAGE" template cert-manager \
		--name=myrelease \
		--version 1.0.4 \
		--repo https://charts.jetstack.io \
		--output /out/subdir/manifest.yaml \
		--debug
}

@test "CLI should build local chart" {
	docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" -v "$EXAMPLE_DIR/release-name:/chart" "$IMAGE" template /chart \
		--version 1.2.3 \
		--output /out/manifest.yaml \
		--debug
	ls -la "$OUT_DIR" "$OUT_DIR/manifest.yaml" >&2
	cat "$OUT_DIR/manifest.yaml" | tee /dev/stdout /dev/stderr | grep -q 'chartVersion: 1.2.3'
}

@test "CLI should output kustomization" {
	docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" -v "$EXAMPLE_DIR/namespace:/chart" "$IMAGE" template /chart \
		--output /out/kdir/ \
		--debug
	ls -la "$OUT_DIR" "$OUT_DIR/kdir" >&2
	[ -f "$OUT_DIR/kdir/kustomization.yaml" ]
	[ -f "$OUT_DIR/kdir/configmap_myconfigb.yaml" ]
}

@test "CLI should accept release name as argument" {
	docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" "$IMAGE" template myreleasex cert-manager \
		--version 1.0.4 \
		--repo https://charts.jetstack.io \
		--output /out/manifest.yaml \
		--debug
	[ -f "$OUT_DIR/manifest.yaml" ]
	grep -q myreleasex "$OUT_DIR/manifest.yaml"
}

@test "CLI should accept git url as helm repository" {
	docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" \
		-e KHELM_ENABLE_GIT_GETTER=true \
		"$IMAGE" template cert-manager \
		--repo git+https://github.com/cert-manager/cert-manager@deploy/charts?ref=v0.6.2 \
		--output /out/manifest.yaml \
		--debug
	[ -f "$OUT_DIR/manifest.yaml" ]
	grep -q ca-sync "$OUT_DIR/manifest.yaml"
}

@test "CLI should cache git repository" {
	mkdir $OUT_DIR/cache
	docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" -v "$OUT_DIR/cache:/helm/cache" \
		-e KHELM_ENABLE_GIT_GETTER=true \
		"$IMAGE" template cert-manager \
		--repo git+https://github.com/cert-manager/cert-manager@deploy/charts?ref=v0.6.2 \
		--output /out/manifest.yaml \
		--debug
	[ -f "$OUT_DIR/manifest.yaml" ]
	grep -q ca-sync "$OUT_DIR/manifest.yaml"
	rm -f "$OUT_DIR/manifest.yaml"
	docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" -v "$OUT_DIR/cache:/helm/cache" \
		-e KHELM_ENABLE_GIT_GETTER=true \
		--network=none "$IMAGE" template cert-manager \
		--repo git+https://github.com/cert-manager/cert-manager@deploy/charts?ref=v0.6.2 \
		--output /out/manifest.yaml \
		--debug
	[ -f "$OUT_DIR/manifest.yaml" ]
	grep -q ca-sync "$OUT_DIR/manifest.yaml"
}

@test "CLI should reject git repository when not in repositories.yaml and trust-any disabled" {
	run -1 docker run --rm -u $(id -u):$(id -g) -v "$OUT_DIR:/out" \
		-e KHELM_ENABLE_GIT_GETTER=true \
		-e KHELM_TRUST_ANY_REPO=false \
		"$IMAGE" template cert-manager \
		--repo git+https://github.com/cert-manager/cert-manager@deploy/charts?ref=v0.6.2 \
		--output /out/manifest.yaml \
		--debug
}
