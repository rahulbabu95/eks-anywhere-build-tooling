#!/bin/bash
# test-fix-patches.sh
PR=$1
PROJECT=$2

git fetch origin pull/$PR/head:test-pr-$PR
git checkout test-pr-$PR
SKIP_VALIDATION=true /Users/rahulgab/Desktop/work/1-30/eks-anywhere-build-tooling/bin/version-tracker fix-patches --project $PROJECT --pr $PR --max-attempts 3 --verbosity 6