#!/bin/sh

cd "$(dirname "$0")/../example"

echo
echo "  TEST $0: Run kpt functions"
echo

set -e

rm -rf ./kpt/output-local ./kpt/output-kustomization ./kpt/output-remote \
	./kpt/output-helm-kustomize/output-kustomization ./kpt/output-helm-kustomize/static/generated-manifest.yaml

(
	set -ex
	kpt fn run --network --mount "type=bind,source=`pwd`/namespace,target=/source" ./kpt

	[ -f ./kpt/output-local.yaml ]
	[ -f ./kpt/output-kustomization/configmap_myconfiga.yaml ]
	[ -f ./kpt/output-kustomization/configmap_myconfigb.yaml ]
	[ -f ./kpt/output-kustomization/kustomization.yaml ]
	[ -f ./kpt/output-remote.yaml ]
	[ -f ./kpt/output-helm-kustomize/static/generated-manifest.yaml ]
	! grep -m1 -B10 -A1 ' namespace: ""' ./kpt/output-helm-kustomize/static/generated-manifest.yaml || (echo 'FAIL: output contains empty namespace field' >&2; false)
	grep -q ' namespace: kube-system' ./kpt/output-helm-kustomize/static/generated-manifest.yaml || (echo 'FAIL: did not preserve chart resource namespace' >&2; false)

	kustomize build ./kpt/output-kustomization | grep -q ' myconfiga'
)

echo SUCCESS
