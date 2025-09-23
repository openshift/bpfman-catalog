FROM quay.io/operator-framework/opm:v1.54.0

ARG INDEX_FILE=./auto-generated/catalog/dev.yaml
COPY $INDEX_FILE /configs/bpfman-operator/index.yaml

ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

RUN ["/bin/opm", "serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

# Indicate where the catalog configuration is located in the image.
LABEL operators.operatorframework.io.index.configs.v1=/configs
