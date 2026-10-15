IMAGE ?= mgoltzsche/khelm:latest

render: image
	kpt fn render
	yq eval 'del(.status)' -i Kptfile

image:
	docker build --force-rm -t local/declarative-khelm-fn-example:unstable --build-arg IMAGE="$(IMAGE)" .
