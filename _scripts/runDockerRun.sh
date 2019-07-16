#!/usr/bin/env bash

set -euo pipefail

cd "${BASH_SOURCE%/*}/../"

if [ "${CI:-}" == "true" ] && [ "${TRAVIS_PULL_REQUEST_BRANCH:-}" != "" ] && [ "${CI_ONLY_RUN:-}" != "" ]
then
	if [[ ! "${VIM_FLAVOR}_${VIM_VERSION}_${GO_VERSION}" =~ $(echo ^\($(echo $CI_ONLY_RUN | sed 's/[[:blank:]]//g' | sed -e 's/^,*//' | sed -e 's/,*$//g' | sed -e 's/,/|/g')\)$) ]]
	then
		echo "Skipping build for ${VIM_FLAVOR}_${VIM_VERSION}_${GO_VERSION}"
		exit 0
	fi
fi

proxy=""

if [ "${CI:-}" != "true" ]
then
	proxy="-v $GOPATH/pkg/mod/cache/download:/cache -e GOPROXY=file:///cache"
fi

docker run $proxy --env-file ./_scripts/.docker_env_file -e "VIM_FLAVOR=${VIM_FLAVOR:-vim}" -v $PWD:/home/$USER/govim -w /home/$USER/govim --rm govim ./_scripts/dockerRun.sh
