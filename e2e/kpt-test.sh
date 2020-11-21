#!/bin/sh

cd "$(dirname "$0")/../example"

echo '##'
echo "# TEST $0: Run as kpt function"
echo '##'

set -ex

rm -rf ./kpt/output-local ./kpt/output-remote

kpt fn run --network --mount "type=bind,source=`pwd`/no-namespace,target=/source" ./kpt

[ -f ./kpt/output-local/configmap_myrelease-myconfigb.yaml ]
[ ! -f ./kpt/output-local/kustomization.yaml ]
[ -f ./kpt/output-remote/kustomization.yaml ]

echo success
