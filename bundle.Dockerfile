# OLM bundle image for forail-operator.
#
# The bundle layout:
#   bundle/
#   ├── manifests/
#   │   ├── forail-operator.clusterserviceversion.yaml
#   │   └── forail.forail-platform.io_*.yaml      # one per CRD
#   └── metadata/
#       └── annotations.yaml
#
# `make bundle` populates bundle/manifests/ from
# config/manifests/bases/*.yaml + config/crd/bases/*.yaml; this
# Dockerfile then wraps it into a scratch-based registry+v1 image
# that an OLM CatalogSource can serve.
#
# Build:
#   make bundle bundle-build BUNDLE_IMG=krlex/forail-operator-bundle:v1.0.0
#
# Publish:
#   make bundle-push BUNDLE_IMG=krlex/forail-operator-bundle:v1.0.0
FROM scratch

LABEL operators.operatorframework.io.bundle.mediatype.v1=registry+v1
LABEL operators.operatorframework.io.bundle.manifests.v1=manifests/
LABEL operators.operatorframework.io.bundle.metadata.v1=metadata/
LABEL operators.operatorframework.io.bundle.package.v1=forail-operator
LABEL operators.operatorframework.io.bundle.channels.v1=alpha
LABEL operators.operatorframework.io.bundle.channel.default.v1=alpha

COPY bundle/manifests /manifests/
COPY bundle/metadata  /metadata/
