#!/bin/sh

cd "$(dirname "$0")/../example"

echo
echo "  TEST $0: Run kpt functions of example/kpt/test-cases"
echo

set -e

rm -rf ./kpt/test-cases/output-local ./kpt/test-cases/output-kustomization ./kpt/test-cases/output-remote

(
	set -ex
	kpt fn run --network --mount "type=bind,source=`pwd`/namespace,target=/source" ./kpt/test-cases

	[ -f ./kpt/test-cases/output-local.yaml ]
	[ -f ./kpt/test-cases/output-kustomization/configmap_myconfiga.yaml ]
	[ -f ./kpt/test-cases/output-kustomization/configmap_myconfigb.yaml ]
	[ -f ./kpt/test-cases/output-kustomization/kustomization.yaml ]
	[ -f ./kpt/test-cases/output-remote.yaml ]

	kustomize build ./kpt/test-cases/output-kustomization | grep -q ' myconfiga'
)

echo SUCCESS
