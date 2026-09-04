#!/bin/bash

i=$(gh api /repos/Velocidex/evtx/actions/runs?per_page=10  | jq 'limit(1; .workflow_runs[] | select (.name | contains("Windows")) | .id )') &&
    mkdir -p .ci &&
    cd .ci &&
    gh run download $i &&
    mv fixtures/* ../fixtures &&
    rm -rf .ci
