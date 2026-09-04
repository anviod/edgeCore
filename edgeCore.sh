#!/bin/bash
# edgeCore startup helper script
set -e

edgeCore_HOME="/usr/local/bin/edgeCore"
cd "$edgeCore_HOME"

exec ./edgeCore "$@"
