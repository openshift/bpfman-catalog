FROM quay.io/operator-framework/opm:v1.54.0

ARG COMMIT
ARG INDEX_FILE=./auto-generated/catalog/y-stream.yaml
#This files will be copied twice but it is not possible to COPY if not empty
COPY $INDEX_FILE /configs/bpfman-operator/index.yaml

# Configure the entrypoint and command
ENTRYPOINT ["/bin/opm"]
CMD ["serve", "/configs", "--cache-dir=/tmp/cache"]

RUN ["/bin/opm", "serve", "/configs", "--cache-dir=/tmp/cache", "--cache-only"]

# Set DC-specific label for the location of the DC root directory
# in the image
LABEL operators.operatorframework.io.index.configs.v1=/configs
