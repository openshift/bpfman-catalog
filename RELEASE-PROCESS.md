# Release Process

This document describes the process for releasing a new version of the bpfman-operator to the OpenShift catalogue.

## Overview

The release process has four main phases:

1. **Component Release**: Release the operator, daemon, and agent bundles to `registry.redhat.io`
2. **Z-Stream Catalogue Update**: Pin the released bundle SHA in the z-stream catalogue for internal testing
3. **Released Catalogue Update**: Add the version to the public production catalogue
4. **Catalogue Release**: Publish the updated catalogue to the OpenShift 4.20 catalogue index

## Complete Release Workflow

```
1. Validate snapshot self-consistency (CRITICAL)
   └─> Only ~7% of snapshots are valid due to race conditions

2. Release components to registry.redhat.io
   └─> bpfman-zstream release plan

3. Update z-stream catalogue template with released SHA
   └─> Triggers catalog-zstream pipeline
   └─> Creates catalog-zstream snapshot
   └─> Automated staging release (catalog-zstream-staging)

4. Update released catalogue template with released SHA
   └─> Triggers catalog-4-20 pipeline
   └─> Creates catalog-4-20 snapshot

5. Apply catalog-4-20 release (public production)

6. Commit complete ledger to git
```

## Prerequisites

- Access to the `ocp-bpfman-tenant` namespace in the Konflux cluster
- The component snapshot name (e.g., `bpfman-zstream-8qn2w`)
- `oc` CLI configured and authenticated
- `gh` CLI for creating pull requests
- Python 3 for running the snapshot validation tool

## Phase 1: Prepare for Release

### 1.1 Validate Snapshot Self-Consistency (CRITICAL)

**Why this matters**: Due to parallel builds and the nudge system, only approximately 7% of snapshots are self-consistent. A snapshot is considered self-consistent when the component SHAs embedded inside the bundle (in both the CSV and ConfigMap) match the component SHAs actually present in the snapshot.

The race condition occurs because:
1. Multiple components (agent, daemon, operator) build in parallel
2. Each completion triggers a nudge PR that updates `hack/konflux/images/*.txt` files
3. Multiple operator builds are triggered, each triggering bundle builds
4. Bundle builds freeze component references at build time (from .txt files)
5. Snapshot creation selects the latest available build of each component
6. By the time the snapshot is created, the bundle's embedded references may be stale

**Using an inconsistent snapshot will result in**:
- **Enterprise Contract validation failure** if operator SHA mismatches (blocks release)
- **Runtime failures** if agent/daemon SHAs mismatch (passes Enterprise Contract but fails in production)

**Validate before releasing**:

```bash
./validate-snapshot.py bpfman-zstream-8qn2w
```

Example output for a valid snapshot:
```
=== Validating Snapshot: bpfman-zstream-nk6d4 ===

Snapshot contains:
  Operator: sha256:60d64887fb8b0c2fd13e4bb08fc7a3c84225b4e9f59bf80992d7f3ec6df086ca
  Agent:    sha256:642641e14a86f34c15d97b90029e0d0f558b7e5cf8fc7bf086d2ebfbfe3e5f5e
  Daemon:   sha256:642641e14a86f34c15d97b90029e0d0f558b7e5cf8fc7bf086d2ebfbfe3e5f5e
  Bundle:   sha256:f6177142b9cf34025053d5585054de85d31090126679b4125f5082b1f504e641

Bundle references:
  CSV Operator:     sha256:60d64887fb8b0c2fd13e4bb08fc7a3c84225b4e9f59bf80992d7f3ec6df086ca
  ConfigMap Agent:  sha256:642641e14a86f34c15d97b90029e0d0f558b7e5cf8fc7bf086d2ebfbfe3e5f5e
  ConfigMap Daemon: sha256:642641e14a86f34c15d97b90029e0d0f558b7e5cf8fc7bf086d2ebfbfe3e5f5e

✅ CSV operator matches snapshot
✅ ConfigMap agent matches snapshot
✅ ConfigMap daemon matches snapshot

✅ VALID: This snapshot is self-consistent and safe to release
```

Example output for an invalid snapshot:
```
=== Validating Snapshot: bpfman-zstream-b7mrw ===

Snapshot contains:
  Operator: sha256:60d64887fb8b0c2fd13e4bb08fc7a3c84225b4e9f59bf80992d7f3ec6df086ca
  Agent:    sha256:88fa734ac07ee4d8d8adfbbf1f6e1f0d16c1d20ebab8913b1aaba4f4d59a9a0e
  Daemon:   sha256:88fa734ac07ee4d8d8adfbbf1f6e1f0d16c1d20ebab8913b1aaba4f4d59a9a0e
  Bundle:   sha256:0aab9296469786da27c965343c293a4a7f1ec1b445e3badcd5e58ba322ca5958

Bundle references:
  CSV Operator:     sha256:60d64887fb8b0c2fd13e4bb08fc7a3c84225b4e9f59bf80992d7f3ec6df086ca
  ConfigMap Agent:  sha256:642641e14a86f34c15d97b90029e0d0f558b7e5cf8fc7bf086d2ebfbfe3e5f5e
  ConfigMap Daemon: sha256:642641e14a86f34c15d97b90029e0d0f558b7e5cf8fc7bf086d2ebfbfe3e5f5e

✅ CSV operator matches snapshot
❌ MISMATCH: ConfigMap agent != Snapshot agent
   Bundle wants:  sha256:642641e14a86f34c15d97b90029e0d0f558b7e5cf8fc7bf086d2ebfbfe3e5f5e
   Snapshot has:  sha256:88fa734ac07ee4d8d8adfbbf1f6e1f0d16c1d20ebab8913b1aaba4f4d59a9a0e
❌ MISMATCH: ConfigMap daemon != Snapshot daemon
   Bundle wants:  sha256:642641e14a86f34c15d97b90029e0d0f558b7e5cf8fc7bf086d2ebfbfe3e5f5e
   Snapshot has:  sha256:88fa734ac07ee4d8d8adfbbf1f6e1f0d16c1d20ebab8913b1aaba4f4d59a9a0e

❌ INVALID: This snapshot has mismatches
   - Will fail Enterprise Contract if operator mismatched
   - Will fail in production if agent/daemon mismatched
```

**If validation fails**, you have two options:
1. Find another snapshot from the same day that is self-consistent
2. Trigger new builds and wait for a self-consistent snapshot

To scan multiple snapshots from a given day:
```bash
# List all snapshots from today
oc get snapshots -n ocp-bpfman-tenant \
  -l appstudio.openshift.io/application=bpfman-zstream \
  --sort-by=.metadata.creationTimestamp

# Check each one
for snapshot in $(oc get snapshots -n ocp-bpfman-tenant \
  -l appstudio.openshift.io/application=bpfman-zstream \
  -o jsonpath='{.items[*].metadata.name}'); do
  echo "Checking $snapshot..."
  ./validate-snapshot.py $snapshot
done
```

### 1.2 Bump OPENSHIFT-VERSION for Next Development Version

Before releasing version X.Y.Z, bump OPENSHIFT-VERSION to X.Y.(Z+1) in both component repositories to ensure the `:latest` tag continues to reference the development version after release.

**Example: Releasing 0.5.9, bump to 0.5.10**

In `bpfman-operator`:
```bash
cd ~/src/github.com/openshift/bpfman-operator
git fetch --all
git checkout -b bump-openshift-version-0.5.10 upstream/release-0.5.8
echo "BUILDVERSION=0.5.10" > OPENSHIFT-VERSION
git add OPENSHIFT-VERSION
git commit -m "Bump OPENSHIFT-VERSION to 0.5.10

Increment version to 0.5.10 to prepare for the next z-stream release
following 0.5.9. This ensures the :latest tag continues to reference
the development version after 0.5.9 is released to the catalogue."
git push -u origin bump-openshift-version-0.5.10
gh pr create --head frobware:bump-openshift-version-0.5.10 --base release-0.5.8 \
  --title "Bump OPENSHIFT-VERSION to 0.5.10" \
  --body "Increment OPENSHIFT-VERSION to prepare for next z-stream release"
```

Repeat the same process in `bpfman`:
```bash
cd ~/src/github.com/openshift/bpfman
git fetch --all
git checkout -b bump-openshift-version-0.5.10 downstream/release-0.5.8
echo "BUILDVERSION=0.5.10" > OPENSHIFT-VERSION
git add OPENSHIFT-VERSION
git commit -m "Bump OPENSHIFT-VERSION to 0.5.10

Increment version to 0.5.10 to prepare for the next z-stream release
following 0.5.9. This ensures the :latest tag continues to reference
the development version after 0.5.9 is released to the catalogue."
git push -u origin bump-openshift-version-0.5.10
gh pr create --head frobware:bump-openshift-version-0.5.10 --base release-0.5.8 \
  --title "Bump OPENSHIFT-VERSION to 0.5.10" \
  --body "Increment OPENSHIFT-VERSION to prepare for next z-stream release"
```

**Important**: Wait for these PRs to merge before proceeding. The version bump must be in place before the release.

### 1.3 Update Catalogue Template with Next Version

Add the next version entry to `templates/z-stream.yaml` with the `:latest` tag:

```yaml
    entries:
      - name: bpfman-operator.v0.5.8
      - name: bpfman-operator.v0.5.9
        replaces: bpfman-operator.v0.5.8
      - name: bpfman-operator.v0.5.10    # Add this
        replaces: bpfman-operator.v0.5.9  # Add this
  # ... existing bundles ...
  - schema: olm.bundle          # 0.5.9
    image: quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-zstream:latest
  - schema: olm.bundle          # 0.5.10 - Add this
    image: quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-zstream:latest
```

This allows testing the next development version in-house before it's released.

### 1.4 Create Component Release Manifest

Create the release manifest in `releases/0.5.9/bpfman.yaml`:

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Release
metadata:
  name: release-bpfman-0-5-9-1
  namespace: ocp-bpfman-tenant
  labels:
    release.appstudio.openshift.io/author: 'frobware'
spec:
  releasePlan: bpfman-zstream
  snapshot: bpfman-zstream-nk6d4  # Use the validated snapshot!
  data:
    releaseNotes:
      type: RHEA
```

**Critical**: Use the snapshot name that you validated in step 1.1. Do not use an unvalidated snapshot.

## Phase 2: Component Release

**Important**: You do not need to commit or push anything to git before starting this phase. The `releases/` directory is a ledger of releases that will be committed after all steps are complete. You can apply the manifest directly from your working directory.

### 2.1 Apply Component Release Manifest

```bash
oc apply -f releases/0.5.9/bpfman.yaml
```

This triggers the Konflux release pipeline which:
- Publishes the operator bundle to `registry.redhat.io/bpfman/bpfman-operator-bundle@sha256:...`
- Publishes the daemon image to `registry.redhat.io/bpfman/bpfman-daemon@sha256:...`
- Publishes the agent image to `registry.redhat.io/bpfman/bpfman-agent@sha256:...`

### 2.2 Monitor Release Pipeline

Check the release status:
```bash
oc get release -n ocp-bpfman-tenant release-bpfman-0-5-9-1
```

The output shows:
```
NAME                     SNAPSHOT               RELEASEPLAN      RELEASE STATUS   AGE
release-bpfman-0-5-9-1   bpfman-zstream-nk6d4   bpfman-zstream   Progressing      2m
```

The release process involves several stages:

1. **Enterprise Contract Validation**: Runs first to validate the snapshot
   ```bash
   oc get pipelinerun -n ocp-bpfman-tenant | grep enterprise-contract
   ```

   **Note**: Enterprise Contract only validates the CSV operator SHA, not the ConfigMap agent/daemon SHAs. This is why validating with `validate-snapshot.py` is critical.

2. **Managed Pipeline**: Publishes components to registry.redhat.io (runs in `rhtap-releng-tenant` namespace)
   ```bash
   # Get the managed pipeline name from the release status
   oc get release -n ocp-bpfman-tenant release-bpfman-0-5-9-1 -o jsonpath='{.status.managedProcessing.pipelineRun}'

   # Check its status (note the different namespace)
   oc get pipelinerun -n rhtap-releng-tenant <pipeline-name>
   ```

3. **Completion**: When the release status changes from "Progressing" to "Succeeded"
   ```bash
   oc get release -n ocp-bpfman-tenant release-bpfman-0-5-9-1
   # Look for RELEASE STATUS: Succeeded
   ```

You can also view detailed progress in the Konflux UI. The URL is available in the release annotations:
```bash
oc get release -n ocp-bpfman-tenant release-bpfman-0-5-9-1 -o jsonpath='{.metadata.annotations.pac\.test\.appstudio\.openshift\.io/log-url}'
```

### 2.3 Get Released Bundle SHA

Once the release completes, find the published bundle SHA:

```bash
skopeo inspect docker://registry.redhat.io/bpfman/bpfman-operator-bundle:v0.5.9 | jq -r '.Digest'
```

Or extract it from the snapshot:
```bash
oc get snapshot -n ocp-bpfman-tenant bpfman-zstream-nk6d4 -o json | \
  jq -r '.spec.components[] | select(.name=="bpfman-operator-bundle-zstream") | .containerImage' | \
  cut -d@ -f2
```

Example SHA: `sha256:f6177142b9cf34025053d5585054de85d31090126679b4125f5082b1f504e641`

### 2.4 Verify Running Cluster (Optional but Recommended)

If you have a test cluster running the operator, verify that the pod image pull specs match the snapshot component SHAs exactly:

```bash
# Get all pod image pull specs in the bpfman namespace
oc get pods -n bpfman -o json | \
  jq -r '.items[].spec.containers[].image' | sort -u

# Compare these with your validated snapshot
./validate-snapshot.py bpfman-zstream-nk6d4
```

The SHAs should match exactly. Registry names may differ (quay.io vs registry.redhat.io) due to ImageDigestMirrorSet configuration on the cluster, but the SHAs must match.

**Why registry names don't matter**: OpenShift uses ImageDigestMirrorSet to redirect pulls from `registry.redhat.io` to `quay.io` automatically. The cluster configuration handles this transparently. Only the SHA digest matters for correctness.

## Phase 3: Z-Stream Catalogue Update (Internal Testing)

### 3.1 Update Z-Stream Catalogue Template

Update `templates/z-stream.yaml` to pin version 0.5.9 to the `registry.redhat.io` SHA:

```yaml
  - schema: olm.bundle          # 0.5.9
    image: registry.redhat.io/bpfman/bpfman-operator-bundle@sha256:f6177142b9cf34025053d5585054de85d31090126679b4125f5082b1f504e641
```

Replace the `:latest` tag reference with the actual digest from `registry.redhat.io`.

**Important**: Keep the v0.5.10 entry with `:latest` tag for continued development testing.

### 3.2 Regenerate Catalogue

```bash
make generate-catalogs
```

This updates `auto-generated/catalog/z-stream.yaml` with the pinned bundle reference.

**Verify the generated file changed**:
```bash
git status
# Should show: modified: auto-generated/catalog/z-stream.yaml
git diff auto-generated/catalog/z-stream.yaml
# Should show the bundle reference updated with the registry.redhat.io SHA
```

If `auto-generated/catalog/z-stream.yaml` is not modified, the template change was not processed correctly.

### 3.3 Create Pull Request for Z-Stream Update

```bash
git checkout -b releases/0.5.9
git add templates/z-stream.yaml auto-generated/catalog/z-stream.yaml releases/0.5.9/bpfman.yaml
git commit -m "Update z-stream catalogue with bpfman-operator v0.5.9

Add bpfman-operator version 0.5.9 to the z-stream catalogue template.
This release references the bundle published to registry.redhat.io
with digest f6177142b9cf34025053d5585054de85d31090126679b4125f5082b1f504e641.

Component release manifest included in releases/0.5.9/bpfman.yaml
using snapshot bpfman-zstream-nk6d4."

git push -u origin releases/0.5.9
gh pr create --head frobware:releases/0.5.9 --base main \
  --title "Add bpfman-operator v0.5.9 to z-stream catalogue" \
  --body "..."
```

### 3.4 Wait for Z-Stream Catalogue Build

After the PR merges, monitor the catalogue build:

```bash
# Watch for catalog-zstream pipeline to trigger
oc get pipelinerun -n ocp-bpfman-tenant --sort-by=.metadata.creationTimestamp | grep catalog-zstream

# Once complete, find the snapshot
oc get snapshot -n ocp-bpfman-tenant --sort-by=.metadata.creationTimestamp | grep catalog-zstream | tail -1
```

Note the snapshot name (e.g., `catalog-zstream-kqj29`).

**What happens next**: The `catalog-zstream` pipeline triggers an automated staging release (`catalog-zstream-staging`). You don't need to manually create or apply this release - it happens automatically.

## Phase 4: Released Catalogue Update (Public Production)

### 4.1 Update Released Catalogue Template

Now update the public production catalogue in `templates/released.yaml`:

```yaml
  - schema: olm.channel
    package: bpfman-operator
    name: stable
    entries:
      - name: bpfman-operator.v0.5.8
      - name: bpfman-operator.v0.5.9
        replaces: bpfman-operator.v0.5.8
  - schema: olm.bundle
    image: registry.redhat.io/bpfman/bpfman-operator-bundle@sha256:c186f98463c7afda27f8813a1401901f74c9ffe0414980b1dc04a90e057b5bb0
    name: bpfman-operator.v0.5.8
  - schema: olm.bundle
    image: registry.redhat.io/bpfman/bpfman-operator-bundle@sha256:f6177142b9cf34025053d5585054de85d31090126679b4125f5082b1f504e641
    name: bpfman-operator.v0.5.9
```

Also update `templates/released.Dockerfile-args`:
```
BUILDVERSION=0.5.9
```

### 4.2 Regenerate Released Catalogue

**CRITICAL**: After updating the template, you must regenerate the catalog files:

```bash
make generate-catalogs
```

This updates `auto-generated/catalog/released.yaml` with the new bundle entry.

**Verify the generated file changed**:
```bash
git status
# Should show: modified: auto-generated/catalog/released.yaml
git diff auto-generated/catalog/released.yaml
# Should show the new bundle entry added
```

If `auto-generated/catalog/released.yaml` is not modified, the template change was not processed correctly.

### 4.3 Create Pull Request for Released Catalogue

```bash
git checkout -b releases/0.5.9-catalog upstream/main
git add templates/released.yaml templates/released.Dockerfile-args auto-generated/catalog/released.yaml
git commit -m "Add bpfman-operator version 0.5.9 to released catalog

Update the released catalogue template to include bpfman-operator
version 0.5.9, which supersedes version 0.5.8 in the stable channel.
This release references the bundle published to registry.redhat.io
with digest f6177142b9cf34025053d5585054de85d31090126679b4125f5082b1f504e641.

The BUILDVERSION has been updated to 0.5.9 to match the new release
version."

git push -u origin releases/0.5.9-catalog
gh pr create --head frobware:releases/0.5.9-catalog --base main \
  --title "Add bpfman-operator v0.5.9 to released catalogue" \
  --body "..."
```

### 4.4 Wait for Released Catalogue Build

After the PR merges, monitor the `catalog-4-20` pipeline:

```bash
# Watch for catalog-4-20 pipeline to trigger
oc get pipelinerun -n ocp-bpfman-tenant --sort-by=.metadata.creationTimestamp | grep catalog-4-20

# Once complete, find the snapshot
oc get snapshot -n ocp-bpfman-tenant --sort-by=.metadata.creationTimestamp | grep catalog-4-20 | tail -1
```

Note the snapshot name (e.g., `catalog-4-20-wr5rc`). **This is the snapshot you need for the next step**.

### 4.5 Create Released Catalogue Release Manifest

Create `releases/0.5.9/fbc-4.20.yaml` with the `catalog-4-20` snapshot:

```yaml
apiVersion: appstudio.redhat.com/v1alpha1
kind: Release
metadata:
  name: bpfman-0-5-9-fbc-4-20-0
  namespace: ocp-bpfman-tenant
  labels:
    release.appstudio.openshift.io/author: 'frobware'
spec:
  releasePlan: catalog-4-20
  snapshot: catalog-4-20-wr5rc  # Use the snapshot from step 4.4
```

**Important**: The snapshot comes from the `catalog-4-20` pipeline that ran after merging the `templates/released.yaml` changes. You must wait for that pipeline to complete before creating this manifest.

### 4.6 Apply Released Catalogue Release

```bash
oc apply -f releases/0.5.9/fbc-4.20.yaml
```

This publishes the catalogue to the OpenShift 4.20 catalogue index, making it available to customers.

Monitor the release:
```bash
oc get release -n ocp-bpfman-tenant bpfman-0-5-9-fbc-4-20-0
```

### 4.7 Commit Complete Release Ledger

After the catalogue release completes successfully, commit the final ledger entry:

```bash
git checkout releases/0.5.9  # Return to the z-stream branch
git add releases/0.5.9/fbc-4.20.yaml
git commit -m "Add catalogue release manifest for bpfman-operator v0.5.9

Record the catalogue release to OpenShift 4.20 catalogue index using
snapshot catalog-4-20-wr5rc.

Complete release ledger for version 0.5.9:
- Component release: bpfman.yaml (snapshot: bpfman-zstream-nk6d4)
- Catalogue release: fbc-4.20.yaml (snapshot: catalog-4-20-wr5rc)"

git push
```

Or create a pull request if that matches your workflow.

## Template Types

- **`templates/dev.yaml`**: Development/testing catalogue with latest builds
- **`templates/z-stream.yaml`**: Patch releases (e.g., 0.5.x) - for in-house testing before public release
- **`templates/y-stream.yaml`**: Minor releases (e.g., 0.x.0) - for in-house testing before public release
- **`templates/released.yaml`**: Public production catalogue with pinned digests only

The z-stream and y-stream templates use `:latest` tags during development, which get replaced with `registry.redhat.io` digests when released. The released template always uses pinned digests from `registry.redhat.io`.

## Release Plans (Execution Order)

**Component Releases**:
- **`bpfman-zstream`**: Releases components (bundle, daemon, agent) to `registry.redhat.io` for z-stream (patch) releases
- **`bpfman-ystream`**: Releases components to `registry.redhat.io` for y-stream (minor) releases

**Catalogue Releases** (in execution order):
1. **`catalog-zstream`**: Internal testing catalogue build (from z-stream template)
2. **`catalog-zstream-staging`**: Automated staging release (triggered automatically after catalog-zstream)
3. **`catalog-4-20`**: Public production release to OpenShift 4.20 catalogue index (from released template)

Similarly for y-stream:
1. **`catalog-ystream`**: Internal testing catalogue build (from y-stream template)
2. **`catalog-ystream-staging`**: Automated staging release
3. **`catalog-4-20`**: Public production release (from released template)

## Common Issues

### Snapshot Validation Failures

**Symptom**: `validate-snapshot.py` reports mismatches between bundle references and snapshot components.

**Cause**: Race condition in parallel builds. The bundle embedded component SHAs from the time it was built, but the snapshot selected different component builds.

**Solutions**:
1. **Find another snapshot**: Check other snapshots from the same day - approximately 7% will be valid
   ```bash
   # Scan all today's snapshots
   for snapshot in $(oc get snapshots -n ocp-bpfman-tenant \
     -l appstudio.openshift.io/application=bpfman-zstream \
     --sort-by=.metadata.creationTimestamp -o jsonpath='{.items[*].metadata.name}'); do
     ./validate-snapshot.py $snapshot && break
   done
   ```

2. **Trigger fresh builds**: Force new builds and wait for a self-consistent snapshot to appear
   ```bash
   # Make a trivial change to trigger new builds
   git commit --allow-empty -m "Trigger fresh builds"
   git push
   ```

**Do not proceed with an invalid snapshot**. It will either fail Enterprise Contract or fail in production.

### Bundle Not Found in registry.redhat.io

**Symptom**: `skopeo inspect docker://registry.redhat.io/bpfman/bpfman-operator-bundle:v0.5.9` fails with "manifest unknown"

**Cause**: The release pipeline may still be processing, or the release failed.

**Solutions**:
1. Check release status: `oc get release -n ocp-bpfman-tenant release-bpfman-0-5-9-1`
2. If status is "Progressing", wait a few minutes and retry
3. If status is "Failed", check pipeline logs for errors

### Catalogue Build Fails

**Symptom**: The catalogue pipeline fails with OPM errors.

**Common causes**:
- Invalid bundle digest (typo in SHA)
- Bundle not accessible from the build environment
- OPM render errors (template syntax issues)
- Missing channel entry or bundle entry mismatch

**Solutions**:
1. Check pipeline logs: `oc logs -n ocp-bpfman-tenant <pipelinerun-name> --all-containers`
2. Verify bundle SHA is correct and complete (should be 64 hex characters after `sha256:`)
3. Verify bundle is accessible: `skopeo inspect docker://registry.redhat.io/bpfman/bpfman-operator-bundle@sha256:...`
4. Validate template syntax: ensure channel entries have corresponding bundle entries

### Catalogue Snapshot Not Created

**Symptom**: After merging the catalogue template update, no new snapshot appears.

**Solutions**:
1. Check if pipeline was triggered:
   ```bash
   oc get pipelinerun -n ocp-bpfman-tenant --sort-by=.metadata.creationTimestamp | head
   ```
2. Check pipeline status and logs for failures
3. Verify the git push actually triggered the pipeline (check Tekton EventListener logs)

### Enterprise Contract Failures

**Symptom**: Release gets stuck in "Progressing" with Enterprise Contract validation failures.

**Common causes**:
- CSV operator SHA doesn't match snapshot operator SHA
- Other policy violations (unsigned images, missing metadata, etc.)

**Solutions**:
1. Check Enterprise Contract pipeline logs:
   ```bash
   oc get pipelinerun -n ocp-bpfman-tenant | grep enterprise-contract
   oc logs -n ocp-bpfman-tenant <pipeline-name> --all-containers
   ```
2. If operator SHA mismatch, the snapshot is invalid - find a different snapshot
3. For other policy violations, consult the Enterprise Contract documentation

## Verification

After completing the release:

### 1. Verify Component Release

Check that all components are published to registry.redhat.io:

```bash
# Bundle
skopeo inspect docker://registry.redhat.io/bpfman/bpfman-operator-bundle:v0.5.9

# Operator
skopeo inspect docker://registry.redhat.io/bpfman/bpfman-rhel9-operator:v0.5.9

# Daemon
skopeo inspect docker://registry.redhat.io/bpfman/bpfman-daemon:v0.5.9

# Agent
skopeo inspect docker://registry.redhat.io/bpfman/bpfman-agent:v0.5.9
```

### 2. Verify Bundle Contents Match Snapshot

Extract and verify the published bundle contains the correct component references:

```bash
# Create a temporary container
CONTAINER_ID=$(podman create registry.redhat.io/bpfman/bpfman-operator-bundle:v0.5.9)

# Extract CSV
podman cp $CONTAINER_ID:/manifests/bpfman-operator.clusterserviceversion.yaml /tmp/csv.yaml

# Extract ConfigMap
podman cp $CONTAINER_ID:/manifests/bpfman-config_v1_configmap.yaml /tmp/configmap.yaml

# Clean up
podman rm $CONTAINER_ID

# Verify operator SHA in CSV
grep "bpfman-rhel9-operator@sha256:" /tmp/csv.yaml

# Verify agent and daemon SHAs in ConfigMap
grep "bpfman.agent.image" /tmp/configmap.yaml
grep "bpfman.image" /tmp/configmap.yaml
```

These SHAs should match your validated snapshot exactly.

### 3. Verify Catalogue Published

Check that the catalogue is available in the OpenShift marketplace:

```bash
# In a connected OpenShift 4.20 cluster
oc get catalogsource -n openshift-marketplace

# Check the bpfman-operator package
oc get packagemanifest bpfman-operator -o yaml
```

### 4. Test Installation (Recommended)

In a test OpenShift 4.20 cluster, verify the operator can be installed from the catalogue:

```bash
# Create a subscription
cat <<EOF | oc apply -f -
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: bpfman-operator
  namespace: openshift-operators
spec:
  channel: stable
  name: bpfman-operator
  source: <catalogue-source-name>
  sourceNamespace: openshift-marketplace
  installPlanApproval: Automatic
  startingCSV: bpfman-operator.v0.5.9
EOF

# Verify installation
oc get csv -n openshift-operators | grep bpfman
```

### 5. Verify Running Pods Use Correct Images

After the operator is installed and running, verify the pod images match the release:

```bash
# Get all bpfman pod images
oc get pods -n bpfman -o json | jq -r '.items[].spec.containers[].image' | sort -u

# These should match the snapshot component SHAs
# Registry names may differ (quay.io vs registry.redhat.io) due to ImageDigestMirrorSet
# but the SHA256 digests must match exactly
```

## Troubleshooting the Nudge System and Race Conditions

### Understanding the Problem

The race condition occurs due to the "nudge" system in Konflux:

1. **Component builds complete independently**:
   - `bpfman-daemon` finishes (e.g., at T+24s)
   - `bpfman-agent` finishes (e.g., at T+24s, within the same second)
   - `bpfman-operator` finishes at various times

2. **Each triggers a nudge PR**:
   - Daemon completion → nudge PR updates `hack/konflux/images/bpfman.txt`
   - Agent completion → nudge PR updates `hack/konflux/images/bpfman-agent.txt`
   - Operator completion → nudge PR updates `hack/konflux/images/bpfman-operator.txt`

3. **Nudge PRs trigger operator rebuilds**:
   - When daemon.txt changes → operator rebuilds → bundle rebuilds
   - When agent.txt changes → operator rebuilds → bundle rebuilds
   - Multiple concurrent operator builds are triggered

4. **Bundle freezes component references**:
   - Each bundle build reads the .txt files at build time
   - Bundle embeds these SHAs in CSV (operator) and ConfigMap (agent, daemon)
   - These references become frozen in the bundle image

5. **Snapshot created after all builds**:
   - Snapshot creation selects latest available build of each component
   - Bundle in snapshot may reference different component versions
   - Only ~7% of snapshots happen to be self-consistent

### Why CEL Triggers Can't Fix This

Tekton CEL triggers can control when builds start, but cannot enforce ordering or prevent parallel execution. The underlying issue is temporal: the bundle's embedded references are determined at bundle build time, while the snapshot's component selection happens at snapshot creation time.

### Long-term Solutions

The following approaches could improve the situation, but require upstream changes:

1. **Batched nudge PRs**: Update all .txt files in a single PR instead of separate PRs per component
2. **Build coalescing**: Cancel in-flight builds when new nudge PRs arrive
3. **Release-time bundle transformation**: Rewrite bundle contents at release time to match snapshot
4. **Proper build ordering**: Ensure all components complete before bundle builds start

For now, **validation with `validate-snapshot.py` is mandatory** to ensure snapshot self-consistency.

## Summary Checklist

- [ ] Validate snapshot with `./validate-snapshot.py <snapshot-name>`
- [ ] Bump OPENSHIFT-VERSION to next version in both repositories
- [ ] Update z-stream template with next version entry
- [ ] Create component release manifest with validated snapshot
- [ ] Apply component release and verify completion
- [ ] Get released bundle SHA from registry.redhat.io
- [ ] Update z-stream template with released bundle SHA
- [ ] Regenerate catalogs and verify auto-generated/catalog/z-stream.yaml changed
- [ ] Create and merge PR for z-stream catalogue update (include auto-generated file)
- [ ] Wait for catalog-zstream snapshot
- [ ] Update released template with new version entry
- [ ] Regenerate catalogs and verify auto-generated/catalog/released.yaml changed
- [ ] Create and merge PR for released catalogue update (include auto-generated file)
- [ ] Wait for catalog-4-20 snapshot
- [ ] Create and apply catalog-4-20 release manifest
- [ ] Verify release completion
- [ ] Commit complete ledger to git
- [ ] Verify bundle contents match snapshot
- [ ] Test installation in OpenShift 4.20 cluster
