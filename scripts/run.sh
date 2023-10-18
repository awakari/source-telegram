#!/bin/sh

mkfifo pipe0
/bin/source-telegram < pipe0
