#!/bin/sh

cd "$(dirname "$0")/../example"

echo
echo "  TEST $0: Run kpt functions"
echo

set -e

rm -rf ./kpt/output-local ./kpt/output-kustomization ./kpt/output-remote ./kpt/output-helm-kustomize

(
	set -ex
	kpt fn run --network --mount "type=bind,source=`pwd`/no-namespace,target=/source" ./kpt

	[ -f ./kpt/output-local.yaml ]
	[ -f ./kpt/output-kustomization/configmap_myconfiga.yaml ]
	[ -f ./kpt/output-kustomization/configmap_release-b-myconfigb.yaml ]
	[ -f ./kpt/output-kustomization/kustomization.yaml ]
	[ -f ./kpt/output-remote.yaml ]
	[ -f ./kpt/output-helm-kustomize/static/manifest.yaml ]
	! grep -m1 -B10 -A1 ' namespace: ""' ./kpt/output-helm-kustomize/static/manifest.yaml || (echo FAIL: output contains empty namespace field >&2; false)

	kustomize build ./kpt/output-kustomization | grep -q ' myconfiga'
)

echo SUCCESS
