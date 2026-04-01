# Release Process

This document describes the process for releasing the bpfman-operator
to the OpenShift catalogue via Konflux. It covers both y-stream
(minor) and z-stream (patch) releases.

## Background

### Y-Stream and Z-Stream

A **y-stream** release is a new minor version (e.g., 0.6.0) that
ships with a new OpenShift release. A **z-stream** release is a
patch (e.g., 0.6.1, 0.6.2) that fixes bugs or CVEs in an already
shipped y-stream.

The two streams have different lifecycles:

- **Y-stream** development happens on `main`. When a y-stream is
  released, `main` is branched to a release branch (e.g.,
  `release-0.6`) and `main` moves on to the next minor version.
- **Z-stream** patches are made on the release branch. Each patch
  goes through the same release process but uses the z-stream
  release plan.

Only **one z-stream is active at a time** in Konflux. When a new
y-stream is released, the z-stream components are re-pointed from
the old release branch to the new one (e.g., from `release-0.5.8`
to `release-0.6`). The old branch remains in git but is no longer
built or monitored by Konflux. This matches the netobserv
convention.

If an emergency patch were needed for an older release, the
z-stream components would need to be temporarily re-pointed back
to the old branch.

### Source Repositories

Two repositories contain the component source code:

- `openshift/bpfman-operator` -- operator, agent, and bundle
- `openshift/bpfman` -- daemon

Both maintain a `main` branch (y-stream) and a release branch
(z-stream). The release branch is created from `main` at the
point of the y-stream release.

### Konflux Applications

The bpfman project has two Konflux applications for component
builds:

- **bpfman-ystream**: tracks `main` in both repos. Used for minor
  releases (e.g., 0.6.0).
- **bpfman-zstream**: tracks a release branch (e.g., `release-0.6`)
  in both repos. Used for patch releases (e.g., 0.6.1, 0.6.2).

Each application has a staging release plan (automated) and a
production release plan (manual):

| Application | Staging (automated) | Production (manual) |
|-------------|--------------------|--------------------|
| bpfman-ystream | bpfman-ystream-staging | bpfman-ystream |
| bpfman-zstream | bpfman-zstream-staging | bpfman-zstream |

Catalogue release plans are per OCP version:

| Application | Staging (automated) | Production (manual) |
|-------------|--------------------|--------------------|
| catalog-4-21 | catalog-4-21-staging | catalog-4-21 |

## Snapshot Validation

Due to parallel builds and the nudge system, most Konflux snapshots
are not self-consistent. The bundle image embeds component SHAs at
build time, but the snapshot selects the latest available build of
each component. If a component rebuilt between the bundle build and
snapshot creation, the references diverge.

Enterprise Contract only validates the CSV operator SHA. If the
agent or daemon SHAs mismatch, the release will pass EC but fail
at runtime. Snapshot validation is therefore mandatory.

The validation tool lives in the bpfman-operator repository:

```bash
python3 ~/src/github.com/openshift/bpfman-operator/hack/konflux/scripts/validate-snapshot.py <snapshot-name>
```

To scan all snapshots from a given day:

```bash
for snapshot in $(oc get snapshots -n ocp-bpfman-tenant \
  -l appstudio.openshift.io/application=bpfman-ystream \
  --sort-by=.metadata.creationTimestamp \
  -o jsonpath='{.items[*].metadata.name}' | tr ' ' '\n' | grep "$(date +%Y%m%d)"); do
  echo "--- $snapshot ---"
  python3 ~/src/github.com/openshift/bpfman-operator/hack/konflux/scripts/validate-snapshot.py "$snapshot" 2>&1 | tail -8
done
```

Only proceed with a snapshot where all references match.

## Y-Stream Release (e.g., 0.6.0)

A y-stream release ships a new minor version. After release, `main`
branches to a release branch for future z-stream patches, and `main`
moves on to the next development cycle.

### Phase 1: Prepare Release Branches

Create release branches from the current HEAD of `main` in both
component repositories. These branches become the z-stream source
for future patch releases.

```bash
# bpfman-operator
cd ~/src/github.com/openshift/bpfman-operator
git fetch upstream
git push upstream upstream/main:refs/heads/release-0.6

# bpfman
cd ~/src/github.com/openshift/bpfman
git fetch downstream
git push downstream downstream/main:refs/heads/release-0.6
```

The branch name follows the convention `release-X.Y` (e.g.,
`release-0.6`), matching the existing `release-0.5.8` pattern and
the netobserv convention (e.g., `release-1.11`).

### Phase 1b: Prepare OPENSHIFT-VERSION Bump PRs

After creating the release branch, prepare (but do not merge) PRs
to bump `OPENSHIFT-VERSION` on `main` in both repos.

In **openshift/bpfman-operator**, create a PR against `main` that
updates `OPENSHIFT-VERSION`:

```
BUILDVERSION=0.7.0
CPE_VERSION=0.7
```

In **openshift/bpfman**, create a PR against `main` with the same
change:

```
BUILDVERSION=0.7.0
CPE_VERSION=0.7
```

**Do not merge these PRs until the entire release is complete**
(i.e., after Phase 4, when the FBC release has succeeded and the
ledger is committed). Merging earlier would trigger new builds
from `main` tagged as `0.7.0`, generating nudge PRs and new
snapshots that interleave with the in-flight `0.6.0` release
work. The automated staging releases would also tag images
incorrectly until the ReleasePlanAdmission bump (Phase 1c) lands.

The y-stream template in the bpfman-catalog repo
(`templates/y-stream.yaml` and `templates/y-stream.Dockerfile-args`)
will also need updating to reflect the new version once the next
y-stream development cycle begins.

### Phase 1c: Register Product Security Stream

Before the y-stream ReleasePlanAdmissions can be bumped to the
next version, the product security stream must be registered in
`gitlab.cee.redhat.com/prodsec/product-definitions`. Without
this, the konflux-release-data CI will reject the RPA bump.

Two files need updating:

- `data/openshift/ps_update_streams.json` -- add the new stream
  (e.g., `bpfman-0.7`) with its CPE identifier
- `data/openshift/ps_modules.json` -- add the stream to the
  `bpfman-operator-0` module's `ps_update_streams`,
  `active_ps_update_streams`, and `default_ps_update_streams`

You may not have push access to this repository. If not, fork it
on GitLab and create an MR from the fork. The MR requires review
from ProdSec -- contact `#wg-cpe-assignments` on Slack if needed.

See prior example: commit `43f39bcd` ("Add bpfman-0.6 product
security stream") in the product-definitions repo.

### Phase 1d: Bump ReleasePlanAdmissions

The ReleasePlanAdmissions in the konflux-release-data repository
(gitlab.cee.redhat.com/releng/konflux-release-data) contain
hardcoded version numbers for both y-stream and z-stream. After
a y-stream release, both must be bumped in a single PR. The
Phase 1c prodsec registration must merge first.

Following the netobserv convention (see their commit "Prepare
netobserv 1.11.1 and 1.12.0"), update all four files under
`config/stone-prd-rh01.pg1f.p1/product/ReleasePlanAdmission/ocp-bpfman/`:

**Y-stream** (next minor version):
- `bpfman-ystream.yaml`: `0.6.0` -> `0.7.0`
- `bpfman-ystream-staging.yaml`: `0.6.0` -> `0.7.0`

**Z-stream** (first patch off the new release branch):
- `bpfman-zstream.yaml`: `0.5.10` -> `0.6.1`
- `bpfman-zstream-staging.yaml`: `0.5.10` -> `0.6.1`

In each file, update tags, `product_version`, `synopsis`, and
`topic` to reflect the new version numbers.

Examples in gitlab.cee.redhat.com/releng/konflux-release-data:
- MR !16625: "Prepare netobserv 1.11.1 and 1.12.0" -- bumps both
  y-stream and z-stream RPAs in a single commit
- MR !12185: "Update bpfman z-stream release version from 0.5.9
  to 0.5.10" -- prior bpfman z-stream bump

### Phase 2: Validate and Release Components

#### 2.1 Find a Valid Snapshot

List recent snapshots and validate them:

```bash
oc get snapshots -n ocp-bpfman-tenant \
  -l appstudio.openshift.io/application=bpfman-ystream \
  --sort-by=.metadata.creationTimestamp
```

Run the validation tool against candidates. Pick the most recent
valid snapshot.

#### 2.2 Check Nudge File Sync

Before releasing, verify the nudge files in `bpfman-operator` match
the Konflux `lastPromotedImage` for each component:

```bash
for comp in bpfman-operator-ystream bpfman-agent-ystream bpfman-daemon-ystream; do
  echo "=== $comp ==="
  oc get component "$comp" -n ocp-bpfman-tenant \
    -o jsonpath='{.status.lastPromotedImage}'
  echo
done
```

Compare against the contents of
`hack/konflux/images/{bpfman-operator,bpfman-agent,bpfman}.txt`. If
they differ, sync the nudge files first (see openshift/bpfman-operator
PR #1441 for an example) and wait for a fresh valid snapshot.

#### 2.3 Verify ReleasePlanAdmission

Before applying a release, confirm that a matching
ReleasePlanAdmission exists on the managed workspace. Without
this, the release will fail immediately.

```bash
oc get releaseplanadmission -n rhtap-releng-tenant | grep bpfman-ystream
```

Check that:
- Status condition is `Matched`
- `block-releases` label is `false`
- Component mapping lists all four components with correct
  `registry.redhat.io` URLs
- Product version and tags match the release (e.g., `0.6.0`)

```bash
oc get releaseplanadmission bpfman-ystream -n rhtap-releng-tenant -o yaml
```

For z-stream releases, check `bpfman-zstream` instead.

#### 2.4 Create Component Release Manifest

Create `releases/<version>/bpfman.yaml`:

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Release
metadata:
  name: release-bpfman-0-6-0-0
  namespace: ocp-bpfman-tenant
  labels:
    release.appstudio.openshift.io/author: 'frobware'
spec:
  releasePlan: bpfman-ystream
  snapshot: <validated-snapshot-name>
  data:
    releaseNotes:
      type: RHEA
```

#### 2.5 Apply the Release

```bash
oc apply -f releases/0.6.0/bpfman.yaml
```

Monitor progress:

```bash
oc get release -n ocp-bpfman-tenant release-bpfman-0-6-0-0 \
  -o custom-columns='NAME:.metadata.name,STATUS:.status.conditions[?(@.type=="Released")].reason'
```

Wait for status `Succeeded`. This publishes the component images to
`registry.redhat.io`.

#### 2.6 Verify Released Images

Confirm all four images are pullable from `registry.redhat.io`:

```bash
skopeo inspect docker://registry.redhat.io/bpfman/bpfman-operator-bundle@sha256:<bundle-sha>
skopeo inspect docker://registry.redhat.io/bpfman/bpfman-rhel9-operator@sha256:<operator-sha>
skopeo inspect docker://registry.redhat.io/bpfman/bpfman-agent@sha256:<agent-sha>
skopeo inspect docker://registry.redhat.io/bpfman/bpfman@sha256:<daemon-sha>
```

Then verify the binaries report the correct version:

```bash
podman run --rm --entrypoint /bpfman-operator \
  registry.redhat.io/bpfman/bpfman-rhel9-operator@sha256:<operator-sha> --version
podman run --rm --entrypoint /bpfman-agent \
  registry.redhat.io/bpfman/bpfman-agent@sha256:<agent-sha> --version
podman run --rm --entrypoint /bpfman \
  registry.redhat.io/bpfman/bpfman@sha256:<daemon-sha> --version
```

Each should report the release version (e.g., `0.6.0`) and a git
SHA matching the validated snapshot.

Do not proceed to Phase 3 until this verification passes.

#### 2.7 Get the Released Bundle SHA

The bundle SHA from the validated snapshot is the same digest that
appears at `registry.redhat.io`. Extract it from the snapshot:

```bash
oc get snapshot -n ocp-bpfman-tenant <snapshot-name> -o json | \
  jq -r '.spec.components[] | select(.name=="bpfman-operator-bundle-ystream") | .containerImage' | \
  cut -d@ -f2
```

### Phase 3: Update Released Catalogue

**Important**: Ensure the Phase 1b and Phase 1c PRs have been
created before reaching this point, but do not merge them yet.
They are merged after Phase 4 completes.

#### 3.1 Update the Released Template

Edit `templates/released.yaml` to add the new version. Add a channel
entry and a bundle entry with the `registry.redhat.io` digest:

```yaml
  - schema: olm.channel
    package: bpfman-operator
    name: stable
    entries:
      # ... existing entries ...
      - name: bpfman-operator.v0.6.0
        replaces: bpfman-operator.v0.5.10
  # ... existing bundles ...
  - schema: olm.bundle          # 0.6.0
    image: registry.redhat.io/bpfman/bpfman-operator-bundle@sha256:<bundle-sha>
    name: bpfman-operator.v0.6.0
```

Update `templates/released.Dockerfile-args`:

```
BUILDVERSION=0.6.0
```

#### 3.2 Regenerate Catalogues

```bash
make generate-catalogs
```

Verify `auto-generated/catalog/released.yaml` changed:

```bash
git diff auto-generated/catalog/released.yaml
```

#### 3.3 Create PR and Merge

Push the template and auto-generated changes to `main`. This PR
does **not** release anything to customers. What happens when it
merges:

1. The `catalog-4-21` Tekton pipeline triggers and builds a new
   catalogue image containing the updated bundle entry.
2. Konflux creates a `catalog-4-21` snapshot -- this is just a
   build artefact, not a release.
3. The automated staging release (`catalog-4-21-staging`) pushes
   the catalogue image to `registry.stage.redhat.io` for internal
   testing. This is not visible to customers.

The catalogue is **not** published to the production OpenShift
catalogue index until Phase 4, where we explicitly create and
apply an FBC Release resource. That is the step that makes the
operator available to customers.

#### 3.4 Wait for Catalogue Snapshot

After the PR merges, wait for the `catalog-4-21` pipeline to
complete and produce a snapshot:

```bash
oc get snapshots -n ocp-bpfman-tenant \
  -l appstudio.openshift.io/application=catalog-4-21 \
  --sort-by=.metadata.creationTimestamp | tail -5
```

Note the snapshot name -- you will need it for Phase 4.

### Phase 4: Release Catalogue

This is the step that publishes the operator to the production
OpenShift catalogue index, making it available to customers. Do
not proceed until you are confident the catalogue snapshot from
Phase 3 is correct.

#### 4.1 Create FBC Release Manifest

Create `releases/<version>/fbc.yaml`. Following the netobserv
convention, this is a multi-document YAML file with one Release per
OCP version. Use `# TODO` and `# Done` comments to track progress.

```yaml
# TODO
apiVersion: appstudio.redhat.com/v1alpha1
kind: Release
metadata:
  name: bpfman-0-6-0-fbc-4-21-0
  namespace: ocp-bpfman-tenant
  labels:
    release.appstudio.openshift.io/author: 'frobware'
spec:
  releasePlan: catalog-4-21
  snapshot: <catalog-4-21-snapshot-name>
```

#### 4.2 Apply the FBC Release

```bash
oc apply -f releases/0.6.0/fbc.yaml
```

Monitor progress:

```bash
oc get release -n ocp-bpfman-tenant bpfman-0-6-0-fbc-4-21-0 \
  -o custom-columns='NAME:.metadata.name,STATUS:.status.conditions[?(@.type=="Released")].reason'
```

The release is also visible in the Konflux UI under the releases
tab for the catalog-4-21 application. Look for the release name
matching the `metadata.name` in the manifest (e.g.,
`bpfman-0-6-0-fbc-4-21-0`).

The FBC release runs a different pipeline from component releases.
It uses the `fbc-release.yaml` pipeline which builds an IIB index
image, signs it, and publishes to the production operator index.
The managed pipeline runs in the `rhtap-releng-tenant` namespace
and typically takes 7-15 minutes.

Wait for status `Succeeded`. This publishes the catalogue to the
OpenShift catalogue index, making the operator available to
customers.

#### 4.3 Commit the Release Ledger

Once all releases have completed, commit the release manifests to
git. The `releases/` directory serves as a permanent record of what
was released and when.

#### 4.4 Merge Version Bump PRs

Now that the release is complete, merge the Phase 1b and 1c PRs
in this order:

1. **konflux-release-data MR** (Phase 1c) -- bumps the RPAs so
   that automated staging releases tag images with the correct
   new version numbers
2. **OPENSHIFT-VERSION PRs** (Phase 1b) -- triggers new builds
   from `main` which will be correctly tagged by the updated RPAs

Merging in this order avoids a window where new builds are tagged
with the old version. If the OPENSHIFT-VERSION PRs merge first,
nudge PRs and staging releases will fire with incorrect tags until
the RPA MR lands.

### Phase 5: Re-point Z-Stream to New Release Branch

This phase prepares the z-stream infrastructure for future patch
releases from the new release branch. Do this only after the
y-stream release is fully complete and the ledger is committed.

There are two parts: updating the Tekton pipeline files in git,
and re-pointing the Konflux component resources.

#### 5.1 Update Tekton Pipelines on the Release Branch

The release branch was created from `main` and contains both
y-stream and z-stream Tekton pipeline files. The y-stream files
are the known-working ones (they built the release). The z-stream
files are stale (they reference the old release branch).

Following the netobserv convention (compare `main` vs
`release-1.11` in `netobserv/network-observability-operator`),
the release branch should contain only z-stream files.

On the `release-0.6` branch in **openshift/bpfman-operator**:

1. Delete the stale z-stream files:
   ```
   .tekton/bpfman-operator-zstream-push.yaml
   .tekton/bpfman-operator-zstream-pull-request.yaml
   .tekton/bpfman-operator-bundle-zstream-push.yaml
   .tekton/bpfman-operator-bundle-zstream-pull-request.yaml
   .tekton/bpfman-agent-zstream-push.yaml
   .tekton/bpfman-agent-zstream-pull-request.yaml
   ```

2. Rename the y-stream files to z-stream:
   ```
   bpfman-operator-ystream-push.yaml -> bpfman-operator-zstream-push.yaml
   bpfman-operator-ystream-pull-request.yaml -> bpfman-operator-zstream-pull-request.yaml
   (and so on for bundle and agent)
   ```

3. In each renamed file, update:
   - `target_branch == "main"` to `target_branch == "release-0.6"`
   - Component name references from `*-ystream` to `*-zstream`

On the `release-0.6` branch in **openshift/bpfman**:

Same process for the daemon files:
1. Delete `bpfman-daemon-zstream-{push,pull-request}.yaml`
2. Rename `bpfman-daemon-ystream-*` to `bpfman-daemon-zstream-*`
3. Update `target_branch` and component names

#### 5.2 Bump OPENSHIFT-VERSION on the Release Branch

The release branch still has `OPENSHIFT-VERSION` set to the version
that was just released (e.g., `0.6.0`). Bump it to the next patch
version (e.g., `0.6.1`) so that z-stream builds produce images
tagged correctly. Only `BUILDVERSION` changes; `CPE_VERSION` stays
at the minor version (e.g., `0.6`).

Create PRs against the release branch in both repos.

#### 5.3 Clean Up Stale Z-Stream Files on Main

On `main`, the z-stream Tekton files still reference the old
release branch (e.g., `release-0.5.8`). These are now dead and
should be removed. This applies to both repos.

#### 5.4 Re-point Konflux Components

**Do this only after step 5.1 has merged**, so the z-stream
Tekton pipeline definitions are in place on the release branch.

Update each `bpfman-zstream` component's `spec.source.git.revision`
from the old branch to the new one:

```bash
for comp in bpfman-operator-zstream bpfman-operator-bundle-zstream \
            bpfman-agent-zstream bpfman-daemon-zstream; do
  oc patch component "$comp" -n ocp-bpfman-tenant \
    --type merge -p '{"spec":{"source":{"git":{"revision":"release-0.6"}}}}'
done
```

Verify the change:

```bash
for comp in bpfman-operator-zstream bpfman-operator-bundle-zstream \
            bpfman-agent-zstream bpfman-daemon-zstream; do
  echo "=== $comp ==="
  oc get component "$comp" -n ocp-bpfman-tenant \
    -o jsonpath='revision={.spec.source.git.revision}'
  echo
done
```

All four should report `revision=release-0.6`.

When re-pointed, PaC will detect the new branch and the updated
Tekton files from step 5.1. This will trigger initial builds from
the release branch, creating new z-stream snapshots.

## Z-Stream Release (e.g., 0.6.1)

A z-stream release ships a patch from the release branch. The process
is the same as above but uses the z-stream application and release
plans:

- Application: `bpfman-zstream`
- Release plan: `bpfman-zstream`
- Snapshot label: `appstudio.openshift.io/application=bpfman-zstream`

The catalogue update follows the same phases (update
`templates/released.yaml`, regenerate, merge, wait for catalogue
snapshot, apply FBC release).

## Release Manifest Conventions

Release manifests in `releases/<version>/` are a **ledger** -- a
permanent record of completed releases, not instructions to execute.
The workflow is:

1. Write the manifest locally
2. `oc apply` it from your working directory
3. Wait for the release to succeed and verify externally
4. Only then commit the manifest to git

Do not commit release manifests before the release has completed
and been verified. The git history should reflect what actually
happened.

The layout follows the netobserv convention (see
`netobserv/network-observability-operator/releases/`):

- `bpfman.yaml` -- component release (one document)
- `fbc.yaml` -- catalogue releases (multi-document, one per OCP version)

Naming:
- Component: `release-bpfman-X-Y-Z-<increment>`
- FBC: `bpfman-X-Y-Z-fbc-4-NN-<increment>`

## Release History

| Version | Type | Release Plan | Snapshot | Bundle SHA |
|---------|------|-------------|----------|------------|
| 0.5.8 | z-stream | bpfman-zstream | (pre-validation era) | `c186f984...` |
| 0.5.9 | z-stream | bpfman-zstream | bpfman-zstream-nk6d4 | `f6177142...` |
| 0.5.10 | z-stream | bpfman-zstream | bpfman-zstream-mzn27 | `f015580d...` |
| 0.6.0 | y-stream | bpfman-ystream | bpfman-ystream-20260401-122823-000 | `785c2c96...` |

## Addendum: v0.6.0 Y-Stream Release PRs

The following PRs and MRs were created as part of the v0.6.0
y-stream release on 2026-04-01. They are listed in the order they
should be merged.

### Prerequisites (completed before this release)

- releng/konflux-release-data MR !16643 -- Added catalog-4-21
  release infrastructure (release plans and ReleasePlanAdmissions)

- Create the `catalog-4-21` Application and Component in the
  Konflux UI console. This must be done through the UI, not via
  `oc apply`, because the console performs additional setup (PaC
  webhook registration, secret provisioning) that does not happen
  when resources are created directly. The console will raise a
  PaC PR on the bpfman-catalog repository with auto-generated
  Tekton pipeline definitions.

- The auto-generated pipeline definitions from PaC lack the
  required build args (BASE_IMAGE, INDEX_FILE, BUILDVERSION) and
  CEL path filters. These must be manually added to match the
  existing pipeline pattern. See [openshift/bpfman-catalog#94](https://github.com/openshift/bpfman-catalog/pull/94),
  specifically commit `bd05856` ("Add build args and CEL filters
  for catalog-4-21 pipelines").

### Release Branches (no PR required)

Created directly on the upstream repositories at the point of
release, before any other changes:

- `openshift/bpfman-operator`: branch `release-0.6` at `a92869ed`
- `openshift/bpfman`: branch `release-0.6` at `29bc2ea7`

### Component Release (applied, not committed)

The component release manifest was applied directly from the
working directory and is not committed until the full release
completes (Phase 4.3):

- `oc apply -f releases/0.6.0/bpfman.yaml` -- release plan
  `bpfman-ystream`, snapshot `bpfman-ystream-20260401-122823-000`

### Catalogue Update (merge first)

This PR triggers the catalogue build and must merge before the FBC
release can proceed:

- [openshift/bpfman-catalog#95](https://github.com/openshift/bpfman-catalog/pull/95) -- Add v0.6.0 to released catalogue for OpenShift 4.21

### FBC Release (after catalogue snapshot appears)

After PR #95 merged, snapshot `catalog-4-21-20260401-182354-000`
was created. The staging release was verified, then the production
release was applied:

- `oc apply -f releases/0.6.0/fbc.yaml` -- release plan
  `catalog-4-21`, snapshot `catalog-4-21-20260401-182354-000`
- Result: Succeeded -- operator visible in OpenShift 4.21
  production catalogue

### Release Ledger (after FBC release succeeds)

- [openshift/bpfman-catalog#96](https://github.com/openshift/bpfman-catalog/pull/96) -- Record release manifests for v0.6.0

### Release Process Documentation

- [openshift/bpfman-catalog#97](https://github.com/openshift/bpfman-catalog/pull/97) -- Add RELEASE-PROCESS.md (this document)

### Version Bumps (merge after ledger is committed)

These prepare the next development cycle. Merge the
konflux-release-data MR first so that automated staging releases
tag images correctly when the OPENSHIFT-VERSION bumps trigger new
builds.

1. [releng/konflux-release-data MR](https://gitlab.cee.redhat.com/releng/konflux-release-data/-/merge_requests/new?merge_request%5Bsource_branch%5D=bump-bpfman-streams-0.7.0) -- Bump RPAs: y-stream to 0.7.0, z-stream to 0.6.1
2. [openshift/bpfman-operator#1694](https://github.com/openshift/bpfman-operator/pull/1694) -- Bump OPENSHIFT-VERSION to 0.7.0
3. [openshift/bpfman#521](https://github.com/openshift/bpfman/pull/521) -- Bump OPENSHIFT-VERSION to 0.7.0

### Z-Stream Re-pointing (Phase 5)

Tekton pipeline rename on `release-0.6` branch:
- [openshift/bpfman-operator#1695](https://github.com/openshift/bpfman-operator/pull/1695) -- Prepare release-0.6 for z-stream patches
- [openshift/bpfman#522](https://github.com/openshift/bpfman/pull/522) -- Prepare release-0.6 for z-stream patches

Bump OPENSHIFT-VERSION to 0.6.1 on `release-0.6`:
- [openshift/bpfman-operator#1697](https://github.com/openshift/bpfman-operator/pull/1697) -- Bump OPENSHIFT-VERSION to 0.6.1
- [openshift/bpfman#524](https://github.com/openshift/bpfman/pull/524) -- Bump OPENSHIFT-VERSION to 0.6.1

Stale z-stream file cleanup on `main`:
- [openshift/bpfman-operator#1696](https://github.com/openshift/bpfman-operator/pull/1696) -- Remove stale z-stream Tekton files from main
- [openshift/bpfman#523](https://github.com/openshift/bpfman/pull/523) -- Remove stale z-stream Tekton files from main

Re-point Konflux z-stream components (done, after #1695 and #522 merged):
- `bpfman-operator-zstream`: `release-0.5.8` -> `release-0.6`
- `bpfman-operator-bundle-zstream`: `release-0.5.8` -> `release-0.6`
- `bpfman-agent-zstream`: `release-0.5.8` -> `release-0.6`
- `bpfman-daemon-zstream`: `release-0.5.8` -> `release-0.6`
