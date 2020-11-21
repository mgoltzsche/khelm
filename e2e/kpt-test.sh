#!/bin/sh

cd "$(dirname "$0")/../example"

echo '##'
echo "# TEST $0: Run as kpt function"
echo '##'

set -ex

kpt fn run --mount "type=bind,source=`pwd`/no-namespace,target=/source" ./kpt
