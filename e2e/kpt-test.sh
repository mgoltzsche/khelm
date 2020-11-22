#!/bin/sh

cd "$(dirname "$0")/../example"

echo
echo "  TEST $0: Run kpt function"
echo

set -ex

rm -rf ./kpt/output-local ./kpt/output-kustomization ./kpt/output-remote

kpt fn run --network --mount "type=bind,source=`pwd`/no-namespace,target=/source" ./kpt

[ -f ./kpt/output-local/configmap_release-a-myconfigb.yaml ]
[ ! -f ./kpt/output-local/kustomization.yaml ]
[ -f ./kpt/output-kustomization/configmap_myconfiga.yaml ]
[ -f ./kpt/output-kustomization/configmap_release-b-myconfigb.yaml ]
[ -f ./kpt/output-kustomization/kustomization.yaml ]
[ -f ./kpt/output-remote/deployment_release-c-cert-manager.yaml ]

kustomize build ./kpt/output-kustomization | grep -q ' myconfiga'

echo success
