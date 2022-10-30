IMAGE ?= mgoltzsche/khelm:latest

fn:
	kpt fn eval --image="$(IMAGE)" --fn-config=./fn-config.yaml \
		--mount "type=bind,source=$(CURDIR)/../..,target=/examples,rw=true" \
		--network --as-current-user output --truncate-output=false
