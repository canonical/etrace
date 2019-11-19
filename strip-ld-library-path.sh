#!/bin/sh -e

LD_LIBRARY_PATH=""
export LD_LIBRARY_PATH

exec "$@"
