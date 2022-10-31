ARG IMAGE=mgoltzsche/khelm
FROM $IMAGE
COPY ./chart /chart
ENV KHELM_BUILTIN_CHART=/chart \
	KHELM_KIND=GenerateSampleApp \
	KHELM_APIVERSION=blueprints.example.org/v1alpha1 \
	KHELM_OUTPUT_PATH=generated/
# populate cache
RUN khelm template $KHELM_BUILTIN_CHART >/dev/null && chmod +rx /helm/cache
