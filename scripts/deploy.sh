#!/usr/bin/env bash

set -eu
set -o pipefail

bitrise run test
bitrise run audit-this-step
bitrise run share-this-step
