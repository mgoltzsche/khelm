FROM alpine:3.17 AS khelm
RUN apk update --no-cache
RUN mkdir /helm && chown root:nobody /helm && chmod 1777 /helm
ENV HELM_REPOSITORY_CONFIG=/helm/repository/repositories.yaml
ENV HELM_REPOSITORY_CACHE=/helm/cache
COPY khelm /usr/local/bin/khelmfn
RUN ln -s khelmfn /usr/local/bin/khelm
ENTRYPOINT ["/usr/local/bin/khelmfn"]

FROM khelm AS test
RUN khelmfn version

FROM khelm
