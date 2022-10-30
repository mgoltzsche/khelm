IMAGE ?= mgoltzsche/khelm:latest

fn:
	kpt fn eval --image="$(IMAGE)" --fn-config=./fn-config.yaml \
		--mount "type=bind,src=$(CURDIR)/../../namespace,dst=/source" .
