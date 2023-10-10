#!/bin/bash

export SLUG=ghcr.io/awakari/producer-telegram
export VERSION=latest
docker tag awakari/producer-telegram "${SLUG}":"${VERSION}"
docker push "${SLUG}":"${VERSION}"
