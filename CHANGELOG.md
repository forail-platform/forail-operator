# Changelog

All notable changes to the Forge Operator will be documented in
this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses SemVer.

## [Unreleased]

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
  - `schedule-sample.yaml` RRULE now includes `INTERVAL=1` (Forge
    rejects RRULEs without it: `400 INTERVAL required in rrule`)
  - `jobtemplate-sample.yaml` references `hello_world.yml` instead
    of `site.yml` (Demo Project ships `hello_world.yml` only)
  - `credential-sample.yaml` ships a clearly-fake placeholder
    private key with explicit `ssh-keygen` instructions; previous
    placeholder triggered Forge HTTP 500
    (`binascii.Error: Invalid base64-encoded string`)

## [0.3.0] - 2026-04-27

### Added
- 4 CRDs reconciled against the Forge REST API:
  - `JobTemplate` (forge.forgeplatform.io/v1alpha1) — name,
    description, jobType, project / inventory / credential refs by
    name, playbook, forks, verbosity, extraVars, ask*OnLaunch flags
  - `Inventory` — organization, variables, hosts[], groups[]
    (including nested children and host membership)
  - `Credential` — organization, credentialType, inputs map,
    `inputsFrom` block reading sensitive fields from k8s Secrets
  - `Schedule` — RFC 5545 RRULE, jobTemplate ref, enabled flag,
    extraData (JSON, sent as string on POST per Forge API quirk)
- OAuth2 Bearer auth (PAT issued by `forge-manage create_oauth2_token`)
  with optional `HostHeader` override for Ingress-based access
- Finalizer-driven cleanup: deleting a CR deletes the corresponding
  Forge resource
- 60-second drift requeue: manual edits in the Forge UI are
  reverted toward the CR spec
- envtest-based integration tests for each CRD lifecycle
  (create, update / drift, delete)
- Helm chart at `helm/` with ServiceAccount, ClusterRole,
  ClusterRoleBinding (incl. read on Secrets), Deployment, and
  optional pre-rendered CRDs
- Multi-stage `Dockerfile` building a distroless static image
- Jenkinsfile with 8 stages (Info → Tooling → Lint → Test → Build →
  Scan → Push → Helm Package)
