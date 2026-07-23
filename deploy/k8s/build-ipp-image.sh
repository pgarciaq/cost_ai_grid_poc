#!/usr/bin/env bash
# Build and push a custom IPP (ai-gateway-payload-processing) image to quay.io.
#
# Usage:
#   bash deploy/k8s/build-ipp-image.sh [PR_NUMBER] [IMAGE_TAG]
#
# Defaults:
#   PR_NUMBER  — branch already checked out (skips fetch if omitted)
#   IMAGE_TAG  — "latest" (also pushed as :pr-<PR_NUMBER> if PR_NUMBER given)
#
# Examples:
#   bash deploy/k8s/build-ipp-image.sh              # build current checkout
#   bash deploy/k8s/build-ipp-image.sh 386          # fetch & build PR #386
#
# Prerequisites:
#   - docker login quay.io
#   - Go toolchain (GOTOOLCHAIN=auto will download if needed)
#   - Quay repo exists: quay.io/martin_povolny/ipp-metering

set -euo pipefail

QUAY_IMAGE="quay.io/martin_povolny/ipp-metering"
IPP_REPO_DIR="${IPP_REPO_DIR:-${HOME}/Projects/koku/ai-gateway-payload-processing}"
IPP_UPSTREAM="https://github.com/martinpovolny/ai-gateway-payload-processing"
PR_NUMBER="${1:-}"
IMAGE_TAG="${2:-latest}"
BINARY="/tmp/ipp-bbr"

# ── Clone if needed ──
if [ ! -d "$IPP_REPO_DIR/.git" ]; then
  echo "--- Cloning IPP repo ---"
  git clone "$IPP_UPSTREAM" "$IPP_REPO_DIR"
fi

cd "$IPP_REPO_DIR"

# ── Checkout PR branch if specified ──
if [ -n "$PR_NUMBER" ]; then
  echo "--- Fetching PR #${PR_NUMBER} ---"
  git fetch origin "pull/${PR_NUMBER}/head:pr-${PR_NUMBER}"
  git checkout "pr-${PR_NUMBER}"
fi

echo "--- Building binary (branch: $(git rev-parse --abbrev-ref HEAD), commit: $(git rev-parse --short HEAD)) ---"
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 GOTOOLCHAIN=auto go build -o "$BINARY" ./cmd

echo "--- Building image ---"
docker build \
  -t "${QUAY_IMAGE}:${IMAGE_TAG}" \
  ${PR_NUMBER:+-t "${QUAY_IMAGE}:pr-${PR_NUMBER}"} \
  -f - /tmp <<'EOF'
FROM registry.access.redhat.com/ubi9-minimal:latest
COPY ipp-bbr /bbr
USER 1001
ENTRYPOINT ["/bbr"]
EOF

echo "--- Pushing image ---"
docker push "${QUAY_IMAGE}:${IMAGE_TAG}"
if [ -n "$PR_NUMBER" ]; then
  docker push "${QUAY_IMAGE}:pr-${PR_NUMBER}"
fi

echo ""
echo "Done! Image pushed:"
echo "  ${QUAY_IMAGE}:${IMAGE_TAG}"
[ -n "$PR_NUMBER" ] && echo "  ${QUAY_IMAGE}:pr-${PR_NUMBER}"
echo ""
echo "To deploy to CRC, update deploy/k8s/osac-oidc-fixed.yaml or the relevant"
echo "IPP Helm values to use this image."
