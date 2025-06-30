#!/bin/bash
set -euo pipefail

# Script to fix template files by converting from the original catalog format
echo "Fixing template files..."

cd "$(dirname "$0")"

# Recreate templates with correct format
for stream in y-stream z-stream released; do
    template_file="templates/${stream}.yaml"
    echo "Creating ${template_file}..."
    
    case "$stream" in
        "y-stream")
            dev_image="quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-ystream:latest"
            ;;
        "z-stream")
            dev_image="quay.io/redhat-user-workloads/ocp-bpfman-tenant/bpfman-operator-bundle-zstream:latest"
            ;;
        "released")
            dev_image="registry.redhat.io/bpfman/bpfman-operator-bundle@sha256:7f701331ca76d520f9b7ea68a17259b9e5f5ac5fd9ca97fa4b13fd7159ece8fd"
            ;;
    esac
    
    # Create proper template format
    {
        echo "schema: olm.template.basic"
        echo "entries:"
        cat ../catalog/index.yaml | \
        sed -e "s|registry\\.redhat\\.io/bpfman/bpfman-operator-bundle@sha256:[a-f0-9]*|${dev_image}|g" | \
        yq eval '. as $item ireduce ({}; . + {"entries": (.entries // []) + [$item]})' | \
        yq eval '.entries[]' | \
        sed 's/^/  /'
    } > "$template_file"
    
    echo "Created $template_file"
done

echo "Template files fixed!"