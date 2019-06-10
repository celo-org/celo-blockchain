#!/usr/bin/env bash
make android

rm README.md
rm README.celo.md
VERSION=$(npm show @celo/client version)
a=( ${VERSION//./ } )
((a[2]++))

echo //registry.npmjs.org/:_authToken=$2 > ~/.npmrc
NEW_VERSION="${a[0]}.${a[1]}.${a[2]}"
npm -f --no-git-tag-version version $NEW_VERSION
PACKAGE=$(npm pack)
npm publish $PACKAGE --tag $1 --access public
npm dist-tag add @celo/client@$NEW_VERSION latest