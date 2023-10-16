#!/bin/bash

export SLUG=ghcr.io/awakari/source-telegram
export VERSION=latest
docker tag awakari/source-telegram "${SLUG}":"${VERSION}"
docker push "${SLUG}":"${VERSION}"
