#!/usr/bin/env bash

set -euo pipefail

source "${BASH_SOURCE%/*}/gen_maxVersions_genconfig.bash"

cd "${BASH_SOURCE%/*}"

# Usage; either:
#
#   buildGovimImage.sh
#   buildGovimImage.sh VIM_FLAVOR VIM_VERSION GO_VERSION
#
# Note that VIM_FLAVOR can be one of vim, gvim or neovim and
# VIM_VERSION is a version pertaining to any of them.

if [ "${CI:-}" == "true" ] && [ "${TRAVIS_PULL_REQUEST_BRANCH:-}" != "" ] && [ "${CI_ONLY_RUN:-}" != "" ]
then
	if [[ ! "${VIM_FLAVOR}_${VIM_VERSION}_${GO_VERSION}" =~ $(echo ^\($(echo $CI_ONLY_RUN | sed 's/[[:blank:]]//g' | sed -e 's/^,*//' | sed -e 's/,*$//g' | sed -e 's/,/|/g')\)$) ]]
	then
		echo "Skipping build for ${VIM_FLAVOR}_${VIM_VERSION}_${GO_VERSION}"
		exit 0
	fi
fi

if [ "$#" -eq 3 ]
then
	VIM_FLAVOR="$1"
	VIM_VERSION="$2"
	GO_VERSION="$3"
else
	# If not provided we default to testing against vim.
	VIM_FLAVOR="${VIM_FLAVOR:-vim}"
	if [ "${VIM_VERSION:-}" == "" ]
	then
		eval "VIM_VERSION=\"\$MAX_${VIM_FLAVOR^^}_VERSION\""
	fi
	if [ "${GO_VERSION:-}" == "" ]
	then
		GO_VERSION="$MAX_GO_VERSION"
	fi
fi

cat Dockerfile.user \
	| GO_VERSION=$GO_VERSION VIM_FLAVOR=$VIM_FLAVOR VIM_VERSION=$VIM_VERSION envsubst '$GO_VERSION,$VIM_FLAVOR,$VIM_VERSION' \
	| docker build -t govim --build-arg USER=$USER --build-arg UID=$UID --build-arg GID=$(id -g $USER) -
