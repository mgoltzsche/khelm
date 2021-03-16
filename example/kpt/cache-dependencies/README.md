# kpt function example with caching

This example is used as e2e test and shows that Helm dependencies can be cached.  

Caching can be enabled by mounting a host directory to `/helm` within the container (see `Makefile`).
