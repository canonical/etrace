#!/bin/sh -e

LD_DEBUG=libs

export LD_DEBUG

exec "$@"
