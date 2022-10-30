#!/usr/bin/env bats

set -eu

: ${IMAGE:=mgoltzsche/khelm:latest}
EXAMPLE_CHART_NAMESPACE="`pwd`/example/namespace"
TMP_DIR="$(mktemp -d)"

teardown() {
	rm -rf $TMP_DIR
}

@test "kpt fn should run example/kpt/local-chart" {
	cd example/kpt/local-chart
	rm -rf output
	mkdir output
	make fn
	[ -f ./output/output.yaml ]
	grep -q jenkins-role-binding ./output/output.yaml
}

@test "kpt fn should run cache chart dependency" {
	cd example/kpt/local-chart
	rm -rf output
	mkdir output
	kpt fn eval --image="$IMAGE" --fn-config=./fn-config.yaml \
		--mount "type=bind,src=$TMP_DIR,dst=/helm,rw=true" \
		--mount "type=bind,source=`pwd`/../..,target=/examples,rw=true" \
		--as-current-user output --network
	kpt fn eval --image="$IMAGE" --fn-config=./fn-config.yaml \
		--mount "type=bind,src=$TMP_DIR,dst=/helm,rw=true" \
		--mount "type=bind,source=`pwd`/../..,target=/examples,rw=true" \
		--as-current-user output --truncate-output=false --network

	[ -f ./output/output.yaml ]
	grep -q jenkins-role-binding ./output/output.yaml
	grep -qv myconfiga ./output/output.yaml
}

@test "kpt fn should run example/kpt/chart-to-kustomization" {
	cd example/kpt/chart-to-kustomization
	rm -rf output-kustomization
	make fn

	[ -f ./output-kustomization/configmap_myconfiga.yaml ]
	[ -f ./output-kustomization/configmap_myconfigb.yaml ]
	[ -f ./output-kustomization/kustomization.yaml ]
	kustomize build ./output-kustomization | grep -q ' myconfiga'
}

@test "kpt fn should run example/kpt/remote-chart" {
	cd example/kpt/remote-chart
	rm -f output-remote.yaml
	make fn

	[ -f ./output-remote.yaml ]
	grep -q cainjector ./output-remote.yaml
}

@test "kpt fn should cache remote chart" {
	cd example/kpt/remote-chart
	rm -f output-remote.yaml
	kpt fn eval --as-current-user --network \
		--mount "type=bind,src=$TMP_DIR,dst=/helm,rw=true" \
		--mount "type=bind,src=$EXAMPLE_CHART_NAMESPACE,dst=/source" \
		--image="$IMAGE" \
		--fn-config=./fn-config.yaml .
	[ -f ./output-remote.yaml ]
	rm -f output-remote.yaml

	ls -la $TMP_DIR/cache/khelm
	kpt fn eval --as-current-user \
		--mount "type=bind,src=$TMP_DIR,dst=/helm,rw=true" \
		--mount "type=bind,src=$EXAMPLE_CHART_NAMESPACE,dst=/source" \
		--image="$IMAGE" \
		--fn-config=./fn-config.yaml .
	[ -f ./output-remote.yaml ]
	grep -q cainjector ./output-remote.yaml
}
