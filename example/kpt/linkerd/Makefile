all: manifest

manifest:
	kpt fn run --as-current-user --mount "type=bind,src=`pwd`,dst=/source,rw=true" manifests

update:
	rm -rf dep
	kpt pkg sync .

deploy:
	kpt live apply manifests/static/linkerd

undeploy:
	kpt live destroy manifests/static/linkerd

deploy-cert-manager:
	kpt live apply ../cert-manager/static

deploy-reloader:
	kpt live apply manifests/reloader
