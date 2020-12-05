#!/bin/sh

cd "$(dirname "$0")/../example"

echo
echo "  TEST $0: Run kpt functions"
echo

set -e

rm -rf ./kpt/output-local ./kpt/output-kustomization ./kpt/output-remote

(
	set -ex
	kpt fn run --network --mount "type=bind,source=`pwd`/no-namespace,target=/source" ./kpt

	[ -f ./kpt/output-local.yaml ]
	[ -f ./kpt/output-kustomization/configmap_myconfiga.yaml ]
	[ -f ./kpt/output-kustomization/configmap_release-b-myconfigb.yaml ]
	[ -f ./kpt/output-kustomization/kustomization.yaml ]
	[ -f ./kpt/output-remote.yaml ]

	kustomize build ./kpt/output-kustomization | grep -q ' myconfiga'
)

echo SUCCESS
