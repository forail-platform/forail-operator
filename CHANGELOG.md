# Changelog

All notable changes to the Forail Operator will be documented in
this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses SemVer.

## [Unreleased]

## [1.0.0] - 2026-05-21

### Added
- **5 new CRDs** completing the Forail resource model:
  - `Project` — SCM-backed source of playbooks, optional credential +
    execution-environment refs (`api/v1alpha1/project_types.go`,
    `internal/controller/project_controller.go`).
  - `Organization` — top-level tenant container with max-host quota
    and default-EE reference.
  - `Team` — namespaced team within an organization, with declarative
    `spec.users[]` membership reconciled against
    `/api/v2/teams/{id}/users/`.
  - `Workflow` — workflow_job_template wrapper with a declarative DAG
    of nodes (`spec.nodes[]` keyed by `identifier`) and edges
    (`successNodes`, `failureNodes`, `alwaysNodes`). The reconciler
    diffs against `/workflow_job_template_nodes/` + each node's
    `success_nodes/failure_nodes/always_nodes` sub-relation.
  - `ForailInstance` — describes a Forail backend (URL + bearer token
    via `tokenSecretRef`) that other CRs can target by name via
    `spec.forailInstance`.
- **Multi-cluster support** via `forailapi.ClientPool`: per-CR
  resolution of which Forail backend to write to. CRs without
  `spec.forailInstance` fall back to the default client supplied via
  `--forail-url` / `--forail-token`. Cache invalidation hooks into the
  ForailInstance reconciler so spec changes (Generation bump) rebuild
  the client lazily.
- **OLM packaging**:
  - `config/manifests/bases/forail-operator.clusterserviceversion.yaml`
    — CSV with `alm-examples`, `customresourcedefinitions.owned`
    entries for all 9 CRDs, deployment spec, and cluster-scoped RBAC.
  - `bundle.Dockerfile` + `bundle/metadata/annotations.yaml`
    (registry+v1, alpha channel default) build a bundle image.
  - `Makefile` targets: `bundle`, `bundle-build`, `bundle-push`,
    `bundle-validate`, `catalog-build`, `catalog-push` (uses `opm` for
    the catalog index).
- Helm chart RBAC extended for all 9 CRD verbs (incl. /status and
  /finalizers subresources).
- envtest lifecycle tests for Project, Organization, Team, Workflow,
  and ForailInstance reconcilers — covering create, drift PATCH, and
  finalizer-driven delete; Workflow test verifies DAG edge
  association.

### Changed
- All existing reconcilers (`JobTemplate`, `Inventory`, `Credential`,
  `Schedule`) and the new ones accept a `Pool` field alongside the
  existing `Forail` client. Per-reconcile, the right client is selected
  via the `clientFor()` helper. No behavior change for CRs that omit
  `spec.forailInstance` — they continue to use the default backend.
- `ForailInstanceReconciler` probes `/api/v2/ping/` every 60 seconds
  and surfaces `reachable` + `serverVersion` in status.

### Breaking
- None at the API level (existing CRs continue to reconcile unchanged
  against the default Forail backend). Operator deployment now requires
  the new RBAC for the 5 added CRDs — re-apply
  `helm/templates/rbac.yaml` (or the OLM CSV's clusterPermissions) on
  upgrade.

## [0.3.1] - 2026-04-29

### Added
- Credential controller now watches Secrets via
  `Watches(&corev1.Secret{}, ...)` so a `kubectl edit secret`
  (or `kubectl apply -f new-secret.yaml`) triggers a reconcile
  within seconds. Before this change, rotating an SSH key
  required an explicit annotation kick on the Credential CR
  to fire the next reconcile.

### Changed
- Sample fixes:
  - `schedule-sample.yaml` RRULE now includes `INTERVAL=1` (Forail
    rejects RRULEs without it: `400 INTERVAL required in rrule`)
  - `jobtemplate-sample.yaml` references `hello_world.yml` instead
    of `site.yml` (Demo Project ships `hello_world.yml` only)
  - `credential-sample.yaml` ships a clearly-fake placeholder
    private key with explicit `ssh-keygen` instructions; previous
    placeholder triggered Forail HTTP 500
    (`binascii.Error: Invalid base64-encoded string`)

## [0.3.0] - 2026-04-27

### Added
- 4 CRDs reconciled against the Forail REST API:
  - `JobTemplate` (forail.forail-platform.io/v1alpha1) — name,
    description, jobType, project / inventory / credential refs by
    name, playbook, forks, verbosity, extraVars, ask*OnLaunch flags
  - `Inventory` — organization, variables, hosts[], groups[]
    (including nested children and host membership)
  - `Credential` — organization, credentialType, inputs map,
    `inputsFrom` block reading sensitive fields from k8s Secrets
  - `Schedule` — RFC 5545 RRULE, jobTemplate ref, enabled flag,
    extraData (JSON, sent as string on POST per Forail API quirk)
- OAuth2 Bearer auth (PAT issued by `forail-manage create_oauth2_token`)
  with optional `HostHeader` override for Ingress-based access
- Finalizer-driven cleanup: deleting a CR deletes the corresponding
  Forail resource
- 60-second drift requeue: manual edits in the Forail UI are
  reverted toward the CR spec
- envtest-based integration tests for each CRD lifecycle
  (create, update / drift, delete)
- Helm chart at `helm/` with ServiceAccount, ClusterRole,
  ClusterRoleBinding (incl. read on Secrets), Deployment, and
  optional pre-rendered CRDs
- Multi-stage `Dockerfile` building a distroless static image
- Jenkinsfile with 8 stages (Info → Tooling → Lint → Test → Build →
  Scan → Push → Helm Package)
