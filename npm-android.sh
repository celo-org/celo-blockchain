#!/usr/bin/env bash
make android

rm README.md
rm README.celo.md
VERSION=$(npm show @celo/client version)
a=( ${VERSION//./ } )
((a[2]++))

echo //registry.npmjs.org/:_authToken=$2 > ~/.npmrc
npm -f --no-git-tag-version version "${a[0]}.${a[1]}.${a[2]}"
PACKAGE=$(npm pack)
npm publish $PACKAGE --tag $1