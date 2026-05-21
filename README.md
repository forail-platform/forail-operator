# forge-operator

A Kubernetes operator that reconciles native Forge Platform resources
(Organizations, Teams, Projects, Inventories, Credentials, JobTemplates,
Schedules, Workflows) declared as Kubernetes Custom Resources. Each CR is
translated to a Forge REST API call so the cluster becomes the source of
truth — `kubectl apply -f workflow.yaml` builds the corresponding
WorkflowJobTemplate + DAG inside Forge; `kubectl delete` removes it.

## At a glance

| CRD | Forge resource | Notes |
|---|---|---|
| `Organization` | `/api/v2/organizations/` | Top-level tenant, max-host quota |
| `Team` | `/api/v2/teams/` | Membership reconciled via `/teams/{id}/users/` |
| `Project` | `/api/v2/projects/` | Git/Hg/SVN/manual, optional credential + EE |
| `Inventory` | `/api/v2/inventories/` | Hosts + groups + nested children |
| `Credential` | `/api/v2/credentials/` | Sensitive fields sourced from k8s Secrets |
| `JobTemplate` | `/api/v2/job_templates/` | Multi-credential attach |
| `Schedule` | `/api/v2/schedules/` | RFC 5545 RRULE |
| `Workflow` | `/api/v2/workflow_job_templates/` | Declarative DAG (nodes + edges) |
| `ForgeInstance` | n/a (control plane) | Pointer to a Forge backend for multi-cluster |

All CRDs live in API group `forge.forgeplatform.io/v1alpha1`.

## Multi-cluster

A single operator deployment can sync against any number of Forge
backends. Declare each backend as a `ForgeInstance`:

```yaml
apiVersion: v1
kind: Secret
metadata: { name: forge-eu-token, namespace: default }
stringData:
  token: <PAT from forge-manage create_oauth2_token>
---
apiVersion: forge.forgeplatform.io/v1alpha1
kind: ForgeInstance
metadata: { name: forge-eu, namespace: default }
spec:
  url: https://forge-eu.example.com
  tokenSecretRef: { name: forge-eu-token, key: token }
```

Then point any CR at it via `spec.forgeInstance: forge-eu`. CRs that
omit the field fall back to the global default supplied via
`--forge-url` / `--forge-token`. The reconcile loop on `ForgeInstance`
also probes `/api/v2/ping/` every 60 seconds and surfaces reachability
+ server version in `status`.

## Install

### Via Helm (recommended for dev)

```bash
TOKEN=$(kubectl -n forge exec deploy/forge-web -- \
    forge-manage create_oauth2_token --user admin | tail -1)
helm install forge-operator ./helm -n forge-operator --create-namespace \
    --set forge.url=http://forge-web.forge.svc.cluster.local:8013 \
    --set forge.token=$TOKEN
```

### Via OLM (recommended for OpenShift / OperatorHub)

```bash
# 1. Build + push bundle and catalog images.
make bundle bundle-build bundle-push \
    BUNDLE_IMG=krlex/forge-operator-bundle:v1.0.0
make catalog-build catalog-push \
    BUNDLE_IMG=krlex/forge-operator-bundle:v1.0.0 \
    CATALOG_IMG=krlex/forge-operator-catalog:v1.0.0

# 2. Apply a CatalogSource pointing at the catalog image.
cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata: { name: forge, namespace: olm }
spec:
  sourceType: grpc
  image: krlex/forge-operator-catalog:v1.0.0
  displayName: Forge Operators
  publisher: Forge Platform
EOF

# 3. Subscribe.
cat <<EOF | kubectl apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata: { name: forge-operator, namespace: operators }
spec:
  channel: alpha
  name: forge-operator
  source: forge
  sourceNamespace: olm
EOF
```

OLM creates the ClusterServiceVersion, installs the CRDs, and runs the
operator in `AllNamespaces` mode.

## Layout

```
forge-operator/
├── api/v1alpha1/                          # 9 CRD type definitions
├── internal/
│   ├── controller/                        # one reconciler per CRD
│   ├── forgeapi/                          # thin client over Forge REST
│   │   └── clientpool.go                  # per-ForgeInstance routing
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
make docker-build  # build IMAGE=krlex/forge-operator:latest
make bundle        # populate bundle/manifests/ from CRDs + CSV base
make run           # run the operator out-of-cluster against $KUBECONFIG
```

`make test` downloads envtest assets via `setup-envtest`; the first run
takes a minute as it fetches the matching apiserver + etcd binaries.

## See also

- [forge-dev-cluster](../forge-dev-cluster) — k3s test cluster (3 m + 4 w)
- [forge-helm](../forge-helm) — Helm chart for the Forge platform itself
- [Forge backend](../forge-backend) — Django API the operator drives
