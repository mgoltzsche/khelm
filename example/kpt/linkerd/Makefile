all: manifest

manifest:
	kpt fn run --as-current-user --mount "type=bind,src=`pwd`,dst=/source,rw=true" manifests

update:
	rm -rf chart
	kpt pkg sync .

deploy:
	kpt live apply ../cert-manager/static
	kpt live apply manifests/static

undeploy:
	kpt live destroy manifests/static
