#!/bin/sh

cd "$(dirname "$0")/../example"

echo
echo "  TEST $0: Run kpt functions of example/cert-manager"
echo

set -e

rm -rf ./kpt/cert-manager/generated-kustomization ./kpt/cert-manager/static/generated-manifest.yaml

(
	set -ex
	kpt fn run --network ./kpt/cert-manager

	[ -f ./kpt/cert-manager/static/generated-manifest.yaml ]
	! grep -m1 -B10 -A1 ' namespace: ""' ./kpt/cert-manager/static/generated-manifest.yaml || (echo 'FAIL: output contains empty namespace field' >&2; false)
	grep -Eq ' name: cert-manager-webhook$' ./kpt/cert-manager/static/generated-manifest.yaml || (echo 'FAIL: does not contain expected resource' >&2; false)
	grep -Eq ' namespace: kube-system$' ./kpt/cert-manager/static/generated-manifest.yaml || (echo 'FAIL: did not preserve chart resource namespace' >&2; false)
	grep -Eq '^kind: CustomResourceDefinition$' ./kpt/cert-manager/static/generated-manifest.yaml || (echo 'FAIL: does not contain CustomResourceDefinition' >&2; false)
)

echo SUCCESS
