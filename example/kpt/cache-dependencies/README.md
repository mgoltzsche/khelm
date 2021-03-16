# kpt function example with caching

This example is used as e2e test and shows that Helm dependencies can be cached.  

Caching can be enabled by mounting a host directory into the container at `/helm` (see `Makefile`).
