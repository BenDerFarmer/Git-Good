#!/bin/env bash

mkdir -p ./keys
ssh-keygen -t ed25519 -f ./keys/hostkey
