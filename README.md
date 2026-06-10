# forail-operator

[![CI](https://github.com/forail-platform/forail-operator/actions/workflows/ci.yml/badge.svg)](https://github.com/forail-platform/forail-operator/actions/workflows/ci.yml)

A Kubernetes operator that reconciles native Forail Platform resources
(Organizations, Teams, Projects, Inventories, Credentials, JobTemplates,
Schedules, Workflows) declared as Kubernetes Custom Resources. Each CR is
translated to a Forail REST API call so the cluster becomes the source of
truth — `kubectl apply -f workflow.yaml` builds the corresponding
WorkflowJobTemplate + DAG inside Forail; `kubectl delete` removes it.

## At a glance

| CRD | Forail resource | Notes |
|---|---|---|
| `Organization` | `/api/v2/organizations/` | Top-level tenant, max-host quota |
| `Team` | `/api/v2/teams/` | Membership reconciled via `/teams/{id}/users/` |
| `Project` | `/api/v2/projects/` | Git/Hg/SVN/manual, optional credential + EE |
| `Inventory` | `/api/v2/inventories/` | Hosts + groups + nested children |
| `Credential` | `/api/v2/credentials/` | Sensitive fields sourced from k8s Secrets |
| `JobTemplate` | `/api/v2/job_templates/` | Multi-credential attach |
| `Schedule` | `/api/v2/schedules/` | RFC 5545 RRULE |
| `Workflow` | `/api/v2/workflow_job_templates/` | Declarative DAG (nodes + edges) |
| `ForailInstance` | n/a (control plane) | Pointer to a Forail backend for multi-cluster |

All CRDs live in API group `forail.forail-platform.io/v1alpha1`.

## Multi-cluster

A single operator deployment can sync against any number of Forail
backends. Declare each backend as a `ForailInstance`:

```yaml
apiVersion: v1
kind: Secret
metadata: { name: forail-eu-token, namespace: default }
stringData:
  token: <PAT from forail-manage create_oauth2_token>
---
apiVersion: forail.forail-platform.io/v1alpha1
kind: ForailInstance
metadata: { name: forail-eu, namespace: default }
spec:
  url: https://forail-eu.example.com
  tokenSecretRef: { name: forail-eu-token, key: token }
```

Then point any CR at it via `spec.forailInstance: forail-eu`. CRs that
omit the field fall back to the global default supplied via
`--forail-url` / `--forail-token`. The reconcile loop on `ForailInstance`
also probes `/api/v2/ping/` every 60 seconds and surfaces reachability
+ server version in `status`.

## Install

### Via Helm (recommended for dev)

```bash
TOKEN=$(kubectl -n forail exec deploy/forail-web -- \
    forail-manage create_oauth2_token --user admin | tail -1)
helm install forail-operator ./helm -n forail-operator --create-namespace \
    --set forail.url=http://forail-web.forail.svc.cluster.local:8013 \
    --set forail.token=$TOKEN
```

### Via OLM (recommended for OpenShift / OperatorHub)

```bash
# 1. Build + push bundle and catalog images.
make bundle bundle-build bundle-push \
    BUNDLE_IMG=krlex/forail-operator-bundle:v1.0.0
make catalog-build catalog-push \
    BUNDLE_IMG=krlex/forail-operator-bundle:v1.0.0 \
    CATALOG_IMG=krlex/forail-operator-catalog:v1.0.0

# 2. Apply a CatalogSource pointing at the catalog image.
cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata: { name: forail, namespace: olm }
spec:
  sourceType: grpc
  image: krlex/forail-operator-catalog:v1.0.0
  displayName: Forail Operators
  publisher: Forail Platform
EOF

# 3. Subscribe.
cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata: { name: forail-operator, namespace: operators }
spec:
  channel: alpha
  name: forail-operator
  source: forail
  sourceNamespace: olm
EOF
```

OLM creates the ClusterServiceVersion, installs the CRDs, and runs the
operator in `AllNamespaces` mode.

## Layout

```
forail-operator/
├── api/v1alpha1/                          # 9 CRD type definitions
├── internal/
│   ├── controller/                        # one reconciler per CRD
│   ├── forailapi/                          # thin client over Forail REST
│   │   └── clientpool.go                  # per-ForailInstance routing
│   └── ...
├── cmd/main.go                            # manager bootstrap
├── config/
│   ├── crd/bases/                         # generated CRD YAMLs
│   ├── samples/                           # one example per CRD
│   ├── rbac/                              # generated ClusterRole
│   └── manifests/bases/                   # CSV base for OLM
├── helm/                                  # Helm chart for in-cluster install
├── bundle/                                # OLM bundle (manifests + metadata)
└── bundle.Dockerfile                      # bundle image
```

## Development

```bash
make tidy          # go mod tidy
make generate      # regen zz_generated.deepcopy.go
make manifests     # regen config/crd/bases/*.yaml + config/rbac/role.yaml
make build         # binary at bin/manager
make vet
make test          # envtest-driven integration tests
make docker-build  # build IMAGE=krlex/forail-operator:latest
make bundle        # populate bundle/manifests/ from CRDs + CSV base
make run           # run the operator out-of-cluster against $KUBECONFIG
```

`make test` downloads envtest assets via `setup-envtest`; the first run
takes a minute as it fetches the matching apiserver + etcd binaries.

## See also

- [forail-dev-cluster](../forail-dev-cluster) — k3s test cluster (3 m + 4 w)
- [forail-helm](../forail-helm) — Helm chart for the Forail platform itself
- [Forail backend](../forail-backend) — Django API the operator drives
