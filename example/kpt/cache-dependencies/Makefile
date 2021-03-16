all: manifest

manifest:
	mkdir -p $$HOME/.helm
	kpt fn run --as-current-user --network \
		--mount "type=bind,src=$$HOME/.helm,dst=/helm,rw=true" \
		--mount "type=bind,src=`pwd`,dst=/source,rw=true" \
		.

clean:
	rm -rf chart/charts chart/requirements.lock generated-manifests
