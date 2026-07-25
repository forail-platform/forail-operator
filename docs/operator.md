# Kubernetes Operator

`forail-operator` manages Forail resources as Kubernetes custom resources. You
declare an Organization, Project, Inventory, Credential, JobTemplate, Schedule,
Team or Workflow in YAML, and the operator reconciles it against a Forail
backend over the REST API — creating what is missing, updating what drifted, and
reporting the outcome in `.status`.

It is the same object model you would build by hand in the UI, expressed as
GitOps-friendly manifests.

## Install

```bash
TOKEN=$(kubectl -n forail exec deploy/forail-web -- \
    forail-manage create_oauth2_token --user admin | tail -1)

helm install forail-operator ./helm -n forail-operator --create-namespace \
    --set forail.url=http://forail-web.forail.svc.cluster.local:8013 \
    --set forail.token="$TOKEN"
```

`forail.url` has to be a host Forail accepts. The chart's `forail.allowedHosts`
already covers the `forail-web` Service DNS names, so the URL above works
unchanged. Reaching Forail under any other name — an ingress host, an external
load balancer — needs `--set forail.hostHeader=<that host>`, or Django rejects
every call with `400`.

For OpenShift / OperatorHub there is an OLM bundle under `bundle/`; see the
repository README for the catalog build.

## Resource model

Nine kinds, all in API group `forail.forail-platform.io/v1alpha1`:

| Kind | Forail resource |
|---|---|
| `Organization` | `/api/v2/organizations` |
| `Team` | `/api/v2/teams` + `/teams/{id}/users/` |
| `Project` | `/api/v2/projects` |
| `Inventory` | `/api/v2/inventories` |
| `Credential` | `/api/v2/credentials` |
| `JobTemplate` | `/api/v2/job_templates` |
| `Schedule` | `/api/v2/schedules` |
| `Workflow` | `/api/v2/workflow_job_templates` + `/workflow_nodes/` |
| `ForailInstance` | control-plane only — see *Multiple backends* |

Every reconciler writes `Synced` and `Ready` conditions plus the upstream id, so
`kubectl describe` tells you whether Forail actually accepted the object:

```
status:
  conditions:
    - type: Synced
      status: "True"
      reason: InSync
      message: Organization is in sync with Forail
  forailId: 16
```

## Credential Secrets and namespaces

A `Credential` keeps its sensitive fields in a Kubernetes `Secret` and
references them from `spec.inputsFrom`:

```yaml
apiVersion: forail.forail-platform.io/v1alpha1
kind: Credential
metadata:
  name: deploy-key
  namespace: forail-operator
spec:
  organization: Default
  credentialType: Machine
  inputs:
    username: deploy
  inputsFrom:
    - name: ssh_key_data
      valueFrom:
        name: deploy-ssh
        key: ssh_key_data
```

**The operator never holds a cluster-wide grant on Secrets.** Its access is a
namespaced `Role`, and its Secret cache is scoped to the same set — deliberately,
because a controller that can read every Secret in the cluster is a large blast
radius for a convenience nobody asked for.

The practical consequence: a `Credential` resolves `spec.inputsFrom` in **its
own** namespace, and that namespace has to be one the operator was given. Out of
the box that is only the release namespace, so Credentials live beside the
operator. To keep them elsewhere, name those namespaces at install time:

```bash
helm install forail-operator ./helm ... \
    --set 'secretNamespaces={team-a,team-b}'
```

That single value renders both halves — the Secret `Role`/`RoleBinding` in each
namespace, and `--secret-namespaces` on the Deployment so the cache covers the
same set. Adding a `RoleBinding` by hand does not work on its own; the cache
would still reject the namespace with:

```
read Secret team-a/deploy-ssh: unable to get: team-a/deploy-ssh
  because of unknown namespace for the cache
```

## Multiple backends

Every CR takes an optional `spec.forailInstance`. When set, the controller looks
up a `ForailInstance` by that name in the same namespace, reads its bearer token
from the Secret named by `spec.tokenSecretRef`, and builds a per-instance client
(cached, invalidated when the observed generation moves). When it is empty the
CR falls back to the default backend from `--forail-url` / `--forail-token`.

That is what lets one operator drive several Forail installations:

```yaml
---
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
---
apiVersion: forail.forail-platform.io/v1alpha1
kind: Project
metadata: { name: eu-roles, namespace: default }
spec:
  forailInstance: forail-eu       # routed to the EU backend
  organization: Default
  scmType: git
  scmUrl: https://github.com/mycorp/eu-roles.git
---
apiVersion: forail.forail-platform.io/v1alpha1
kind: Project
metadata: { name: us-roles, namespace: default }
spec:
  # no forailInstance — uses the default backend
  organization: Default
  scmType: git
  scmUrl: https://github.com/mycorp/us-roles.git
```

The `ForailInstance` reconciler polls `/api/v2/ping/` every 60 seconds (30 on
failure) and surfaces reachability and server version, so a backend going dark
is visible without digging:

```
$ kubectl get forailinstance -A
NAMESPACE  NAME        URL                             REACHABLE  VERSION     LAST CHECKED
default    forail-eu   https://forail-eu.example.com   true       2026.07.0   12s
default    forail-us   https://forail-us.example.com   false      —           22s
```

Remember that `tokenSecretRef` is subject to the same namespace rule as
Credential Secrets above.

## Workflows

`Workflow` is the only kind with a graph model: `spec.nodes[]` is a DAG of
job-template nodes joined by `success`, `failure` and `always` edges, keyed by a
per-node identifier. The reconciler creates the workflow job template first,
then the nodes, then the edges between them, so a partially applied graph
converges on the next pass rather than erroring.

## Verifying an install

```bash
kubectl -n forail-operator get pods                 # 1/1 Running, 0 restarts
kubectl -n forail-operator logs deploy/forail-operator-forail-operator | head

kubectl apply -f config/samples/organization-sample.yaml
kubectl get organization platform-team -o jsonpath='{.status}'
# -> conditions Synced/Ready True, forailId set
```

If a CR stays without status, check that the operator can reach the backend
(`kubectl get forailinstance -A`, or the operator log for `400`/`401`), and that
any referenced Secret is in a namespace the operator was given.

## Versions

The operator moved to the platform's date-based scheme; `v1.0.x` predates it.

| Version | Notable |
|---|---|
| `2026.07.1` | `secretNamespaces` — Credential Secrets outside the operator's namespace |
| `2026.07.0` | Secret access narrowed from cluster-wide to a namespaced `Role` |
| `2026.06.0` | Date-based versioning |
| `v1.0.0` | Nine CRDs, multi-cluster routing, OLM bundle |
