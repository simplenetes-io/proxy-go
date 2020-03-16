#!/usr/bin/env sh

#
# Perform proxy validation tests
#
# Usage:
#   ./run_as_containers.sh
#   DOCKER_IP=192.168.99.100 ./run_as_containers.sh
#

# Options
set -u

# Check dependencies
if ! command -v curl >/dev/null; then
    printf "Unable to find external program: curl\n" >&2
    exit 1
fi
if ! command -v docker >/dev/null; then
    printf "Unable to find external program: docker\n" >&2
    exit 1
fi

# Environment variables
DOCKER_IP=${DOCKER_IP:-127.0.0.1}

# Data
golang_image="golang:1.14-alpine"
webserver_container_name="simplenetes_proxy_test_webserver"
webserver_port=9998
proxy_container_name="simplenetes_proxy_test_proxy"
proxy_port=8888

# Procedures
_test_reachable()
{
    port="$1"
    printf "Host: try to reach %s..." "${port}"

    # Verify success
    if ! _status="$(curl --silent -o /dev/null -w "%{http_code}" "${DOCKER_IP}:${port}")"; then
        printf " failed to reach %s. Status: %s\n" "${port}" "${_status}"
        exit 1
    fi

    # Validate response
    if [ "${_status}" -ne 200 ]; then
        printf " unexpected return status after trying to reach %s. Status: %s\n" "${port}" "${_status}"
        exit 1
    fi

    printf " OK\n"
}

_test_request()
{
    port="$1"
    printf "Host: send request to %s..." "${port}"

    # Verify status code
    if ! _status="$(curl --silent "${DOCKER_IP}:${port}")"; then
        printf " failed to send request to %s. Data: %s\n" "${port}" "${_status}"
        exit 1
    fi

    # Validate response
    if ! _output="$(printf "%s" "${_status}" | grep "^webserver [0-9]* (9998)$" 2>/dev/null)"; then
        printf " unexpected return after sending request to %s. Data: %s\n" "${port}" "${_status}"
        exit 1
    fi

    printf " %s. OK\n" "${_output}"
}

_test_request_failure()
{
    port="$1"

    printf "Host: send request to %s..." "${port}"

    if _status="$(curl --silent -o /dev/null -w "%{http_code}" "${DOCKER_IP}:${port}")"; then
        printf " expected request to previous port %s to fail. Data: %s\n" "${port}" "${_status}"
        exit 1
    fi

    printf " returned status %s. OK\n" "${_status}"
}

# Initialize the test webserver
printf "======\n[Docker]\n"
printf "Docker: removing existing %s container...\n" "${webserver_container_name}"
docker stop "${webserver_container_name}" && docker rm "${webserver_container_name}"
printf "Docker: running webserver on port %s..." "${webserver_port}"
docker run --name "${webserver_container_name}" --network="host" -v "$PWD:/${webserver_container_name}" --workdir=/${webserver_container_name} "${golang_image}" nohup sh -c "go run ./test/webserver.go ${webserver_port}" &
sleep 3
printf " OK\n"

# Check test webserver is reachable,
# then try to send request and validate output
printf "======\n[Webserver]\n"
_test_reachable "${webserver_port}"
_test_request "${webserver_port}"

# Initialize the proxy server
# Configuration
printf "======\n[Docker]\n"
printf "8888:[1001,2001,3001,9998,9999]" > ./test/ports.cfg
# Container
printf "Docker: removing existing %s container...\n" "${proxy_container_name}"
docker stop "${proxy_container_name}" && docker rm "${proxy_container_name}"
printf "Docker: running proxy on port %s..." "${proxy_port}"
docker run --name "${proxy_container_name}" --network="host" -v "$PWD:/${proxy_container_name}" --workdir=/${proxy_container_name} "${golang_image}" nohup sh -c "go run ./proxy.go ${DOCKER_IP}" &
sleep 3
printf " OK\n"

# Check proxy is reachable,
# then send request and validate response
printf "======\n[Proxy]\n"
_test_reachable "${proxy_port}"
_test_request "${proxy_port}"

# Send SIGHUP to proxym
printf "======\n[Docker]\n"
printf "Docker: sending HUP signal to proxy..."
docker exec -it "${proxy_container_name}" sh -c "kill -s HUP \$(pgrep exe/proxy)"
printf " OK\n"

# Repeat sending request and validating response
printf "======\n[Proxy]\n"
_test_request "${proxy_port}"

# Push configuration changes
printf "======\n[Docker]\n"
printf "7777:[1001,2001,3001,9998,9999]" > ./test/ports.cfg
# Send SIGHUP to proxy
printf "Docker: sending HUP signal to proxy..."
docker exec -it "${proxy_container_name}" sh -c "kill -s HUP \$(pgrep exe/proxy)"
printf " OK\n"

# Send request to previous proxy port,
# then point to the new port and repeat request
printf "======\n[Proxy]\n"
_test_request_failure "${proxy_port}"
proxy_port=7777
_test_request "${proxy_port}"

# Restore proxy port to initial state
printf "======\n[Docker]\n"
printf "8888:[1001,2001,3001,9998,9999]" > ./test/ports.cfg
printf "Docker: sending HUP signal to proxy..."
docker exec -it "${proxy_container_name}" sh -c "kill -s HUP \$(pgrep exe/proxy)"
printf " OK\n"

# Send request to previous proxy port,
# then point to the new port and repeat request
printf "======\n[Proxy]\n"
_test_request_failure "${proxy_port}"
proxy_port=8888
_test_request "${proxy_port}"

# Clean up
docker stop "${webserver_container_name}" && docker rm "${webserver_container_name}"
docker stop "${proxy_container_name}" && docker rm "${proxy_container_name}"

printf "Tests completed!\n"
