IMAGE ?= mgoltzsche/khelm:latest

render: image
	kpt fn render
	sed -i.bak '/status:/,$$d' Kptfile
	rm -f Kptfile.bak

image:
	docker build --force-rm -t local/declarative-khelm-fn-example:unstable --build-arg IMAGE="$(IMAGE)" .
