#!/usr/bin/env python3
"""
Validate that a Konflux snapshot is self-consistent for OLM bundle releases.

Checks that all component SHAs referenced inside the bundle (CSV + ConfigMap)
match the component SHAs actually present in the snapshot.
"""

import argparse
import subprocess
import sys
import tempfile
import json
import re


def detect_stream(snapshot_name):
    """Detect stream (ystream/zstream) from snapshot name."""
    if "ystream" in snapshot_name:
        return "ystream"
    elif "zstream" in snapshot_name:
        return "zstream"
    else:
        raise ValueError(f"Cannot detect stream from snapshot name: {snapshot_name}")


def get_snapshot_components(snapshot_name, namespace):
    """Get component SHAs from snapshot."""
    cmd = ["oc", "get", "snapshot", "-n", namespace, snapshot_name, "-o", "json"]
    result = subprocess.run(cmd, capture_output=True, text=True, check=True)
    snapshot = json.loads(result.stdout)

    stream = detect_stream(snapshot_name)
    components = {}
    for comp in snapshot["spec"]["components"]:
        name = comp["name"]
        image = comp["containerImage"]
        sha = image.split("@")[1]
        components[name] = sha

    return components, stream


def extract_bundle_refs(bundle_sha, stream):
    """Extract component SHAs referenced in bundle CSV and ConfigMap."""
    bundle_image = f"quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-{stream}@{bundle_sha}"

    # Create temporary container
    result = subprocess.run(
        ["podman", "create", bundle_image], capture_output=True, text=True, check=True
    )
    container_id = result.stdout.strip()

    try:
        # Extract CSV
        with tempfile.NamedTemporaryFile(mode="w+", suffix=".yaml") as csv_file:
            subprocess.run(
                [
                    "podman",
                    "cp",
                    f"{container_id}:/manifests/bpfman-operator.clusterserviceversion.yaml",
                    csv_file.name,
                ],
                check=True,
            )
            csv_content = open(csv_file.name).read()

        # Extract ConfigMap
        with tempfile.NamedTemporaryFile(mode="w+", suffix=".yaml") as cm_file:
            subprocess.run(
                [
                    "podman",
                    "cp",
                    f"{container_id}:/manifests/bpfman-config_v1_configmap.yaml",
                    cm_file.name,
                ],
                check=True,
            )
            cm_content = open(cm_file.name).read()
    finally:
        # Clean up container
        subprocess.run(["podman", "rm", container_id], check=True, capture_output=True)

    # Parse operator SHA from CSV
    operator_match = re.search(
        r"registry\.redhat\.io/bpfman/bpfman-rhel9-operator@(sha256:[a-f0-9]+)",
        csv_content,
    )
    operator_sha = operator_match.group(1) if operator_match else None

    # Parse agent/daemon SHAs from ConfigMap
    agent_match = re.search(r"bpfman\.agent\.image:.*@(sha256:[a-f0-9]+)", cm_content)
    agent_sha = agent_match.group(1) if agent_match else None

    daemon_match = re.search(r"bpfman\.image:.*@(sha256:[a-f0-9]+)", cm_content)
    daemon_sha = daemon_match.group(1) if daemon_match else None

    return {"operator": operator_sha, "agent": agent_sha, "daemon": daemon_sha}


def validate_snapshot(snapshot_name, namespace="ocp-bpfman-tenant"):
    """Validate snapshot self-consistency."""
    print(f"=== Validating Snapshot: {snapshot_name} ===\n")

    # Get snapshot components
    components, stream = get_snapshot_components(snapshot_name, namespace)

    print("Snapshot contains:")
    print(f"  Operator: {components[f'bpfman-operator-{stream}']}")
    print(f"  Agent:    {components[f'bpfman-agent-{stream}']}")
    print(f"  Daemon:   {components[f'bpfman-daemon-{stream}']}")
    print(f"  Bundle:   {components[f'bpfman-operator-bundle-{stream}']}")
    print()

    # Extract bundle references
    bundle_sha = components[f"bpfman-operator-bundle-{stream}"]
    refs = extract_bundle_refs(bundle_sha, stream)

    print("Bundle references:")
    print(f"  CSV Operator:     {refs['operator']}")
    print(f"  ConfigMap Agent:  {refs['agent']}")
    print(f"  ConfigMap Daemon: {refs['daemon']}")
    print()

    # Compare
    valid = True

    if refs["operator"] != components[f"bpfman-operator-{stream}"]:
        print(f"❌ MISMATCH: CSV operator != Snapshot operator")
        print(f"   Bundle wants:  {refs['operator']}")
        print(f"   Snapshot has:  {components[f'bpfman-operator-{stream}']}")
        valid = False
    else:
        print("✅ CSV operator matches snapshot")

    if refs["agent"] != components[f"bpfman-agent-{stream}"]:
        print(f"❌ MISMATCH: ConfigMap agent != Snapshot agent")
        print(f"   Bundle wants:  {refs['agent']}")
        print(f"   Snapshot has:  {components[f'bpfman-agent-{stream}']}")
        valid = False
    else:
        print("✅ ConfigMap agent matches snapshot")

    if refs["daemon"] != components[f"bpfman-daemon-{stream}"]:
        print(f"❌ MISMATCH: ConfigMap daemon != Snapshot daemon")
        print(f"   Bundle wants:  {refs['daemon']}")
        print(f"   Snapshot has:  {components[f'bpfman-daemon-{stream}']}")
        valid = False
    else:
        print("✅ ConfigMap daemon matches snapshot")

    print()
    if valid:
        print("✅ VALID: This snapshot is self-consistent and safe to release")
        return 0
    else:
        print("❌ INVALID: This snapshot has mismatches")
        print("   - Will fail Enterprise Contract if operator mismatched")
        print("   - Will fail in production if agent/daemon mismatched")
        return 1


def main():
    parser = argparse.ArgumentParser(
        description="Validate Konflux snapshot self-consistency"
    )
    parser.add_argument("snapshot", help="Snapshot name to validate")
    parser.add_argument(
        "--namespace",
        "-n",
        default="ocp-bpfman-tenant",
        help="Namespace (default: ocp-bpfman-tenant)",
    )

    args = parser.parse_args()

    try:
        sys.exit(validate_snapshot(args.snapshot, args.namespace))
    except subprocess.CalledProcessError as e:
        print(f"\nError: {e}", file=sys.stderr)
        sys.exit(2)
    except Exception as e:
        print(f"\nUnexpected error: {e}", file=sys.stderr)
        sys.exit(2)


if __name__ == "__main__":
    main()
