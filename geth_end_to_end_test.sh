#!/usr/bin/env bash
set -euo pipefail

mkdir ~/.ssh/ && echo -e "Host github.com\n\tStrictHostKeyChecking no\n" > ~/.ssh/config

# For testing a particular commit hash of Celo monorepo repo (usually, on Circle CI)
# Usage: ci_test.sh checkout <commit_hash_of_celo_monorepo_to_test>
# For testing the local Celo monorepo dir (usually, for manual testing)
# Usage: ci_test.sh local <location_of_local_celo_monorepo_dir>

if [ "${1}" == "checkout" ]; then
    export CELO_MONOREPO_DIR="/tmp/celo"
    # Test master by default.
    COMMIT_HASH_TO_TEST=${2:-"master"}
    echo "Checking out celo monorepo at commit hash ${COMMIT_HASH_TO_TEST}..."
    # Clone only up to depth 100 to save time. If COMMIT_HASH_TO_TEST is not in the
    # last 100 commits then this will fail and that's fine since someone will be forced
    # to upgrade the test to use a newer version of celo-monorepo to test against.
    git clone --depth 100 git@github.com:celo-org/celo-monorepo.git ${CELO_MONOREPO_DIR} && cd ${CELO_MONOREPO_DIR} && git checkout ${COMMIT_HASH_TO_TEST} && cd -
elif [ "${1}" == "local" ]; then
    export CELO_MONOREPO_DIR="${2}"
    echo "Testing using local celo monorepo dir ${CELO_MONOREPO_DIR}..."
fi

GETH_DIR=${3:-"$PWD"}
cd ${CELO_MONOREPO_DIR}/packages/celotool
./ci_test.sh local ${GETH_DIR}