#!/bin/bash -e

BRANCH=${1:-master}
SCRIPTS_DIR=`dirname $0`
cd $SCRIPTS_DIR/..
rm -r ./crypto/bls/bls-zexe
git subtree add --prefix crypto/bls/bls-zexe https://github.com/celo-org/bls-zexe $BRANCH --squash
