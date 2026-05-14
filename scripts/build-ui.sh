#!/usr/bin/env bash

set -e

cd internal/server/ui
yarn install --silent
yarn build
