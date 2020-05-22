#!/usr/bin/env sh

#
# Perform test:
#   Single proxy
#   Custom webserver client and server connections
#
# Usage:
#   ./single_proxy_custom_server.sh
#

# Options
set -u

# Check dependencies
if ! command -v docker >/dev/null; then
    printf "Unable to find external program: docker\n" >&2
    exit 1
fi

# Data
golang_image="golang:1.14-alpine"
proxy_container_name="single_proxy_custom_server-1"
proxy_container_ip="172.17.0.2"
proxy_cluster_port=29999
proxy_host_port=30998
client_container_name="single_proxy_custom_server-client"

# Initialize the proxy server
# Configuration
printf "======\n[Docker]\n"
# Proxy
printf "Docker: removing existing %s container...\n" "${proxy_container_name}"
docker stop "${proxy_container_name}" && docker rm "${proxy_container_name}"
printf "Docker: running proxy on port %s..." "${proxy_cluster_port}"
docker run --name "${proxy_container_name}" -v "$PWD:/${proxy_container_name}" --workdir=/${proxy_container_name} "${golang_image}" nohup sh -c "go run ./src/entrypoint.go ${proxy_container_ip} > /dev/null 2>&1" &
sleep 2
printf " OK\n"

printf "======\n[Docker]\n"
printf "Docker: initializing custom server on %s port %s...\n" "${proxy_container_name}" "${proxy_host_port}"
docker exec -t "${proxy_container_name}" nohup sh -c "go run test/webserver.go ${proxy_host_port}" &
printf " OK\n"

# Client
printf "Docker: removing existing %s container...\n" "${client_container_name}"
docker stop "${client_container_name}" && docker rm "${client_container_name}"
printf "Docker: running client...\n"
docker run --name "${client_container_name}" -v "$PWD:/${client_container_name}" --workdir=/${client_container_name} "${golang_image}" sh -c 'apk add curl && echo "Sending requests to '${proxy_container_ip}':'${proxy_cluster_port}' over the next 60 seconds"; _counter=0; while [ "${_counter}" -lt 60 ]; do if ! curl -s '${proxy_container_ip}':'${proxy_cluster_port}' > /dev/null; then printf "\nFailure: $(date)\n"; exit 1; else printf "."; _counter=$((_counter+1)); sleep 1; fi; done; printf " OK!\n"'


# Clean up
docker stop "${client_container_name}" && docker rm "${client_container_name}"
docker stop "${proxy_container_name}" && docker rm "${proxy_container_name}"

printf "Tests completed!\n"
