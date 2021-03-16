#!/bin/sh

cd "$(dirname "$0")/../example"

echo
echo "  TEST $0: Run kpt functions of example/kpt/cache-dependencies"
echo

set -e

cd kpt/cache-dependencies
make clean

(
	set -ex
	make manifest
	[ -f generated-manifests/manifest1.yaml ]
	[ -f generated-manifests/manifest1-from-cache.yaml ]
	[ -f generated-manifests/manifest2.yaml ]
	[ -f generated-manifests/manifest2-from-cache.yaml ]
)

echo SUCCESS
