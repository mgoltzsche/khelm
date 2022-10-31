IMAGE ?= mgoltzsche/khelm:latest

render: image
	kpt fn render

image:
	docker build --force-rm -t local/declarative-khelm-fn-example:unstable --build-arg IMAGE="$(IMAGE)" .
