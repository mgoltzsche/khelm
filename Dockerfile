FROM alpine:3.14 AS khelm
RUN apk update --no-cache
RUN mkdir /helm && chown root:nobody /helm && chmod 1777 /helm
ENV HELM_HOME=/helm
COPY khelm /usr/local/bin/khelmfn
ENTRYPOINT ["/usr/local/bin/khelmfn"]

FROM khelm AS test
RUN khelmfn version

FROM khelm
