#!/usr/bin/env bash
#
# Usage: ./scripts/publish-mobile-client.sh $COMMIT_SHA $NPM_TOKEN

set -euo pipefail

# Process args
commit_sha_short="${1:0:7}"
npm_token="${2:-}"

package_name="$(node -p "require('./package.json').name")"

version=$(npm show "$package_name" version)
# Bump minor version
a=( ${version//./ } )
((a[2]++))
new_version="${a[0]}.${a[1]}.${a[2]}"

# Add npm token if provided
if [ -n "$npm_token" ]; then
  echo "//registry.npmjs.org/:_authToken=$npm_token" > ~/.npmrc
fi

# TODO: Create an appropriate README for NPM
rm README.md

npm -f --no-git-tag-version version "$new_version"
npm publish --tag "$commit_sha_short" --access public -timeout=9999999
npm dist-tag add "$package_name@$new_version" latest
