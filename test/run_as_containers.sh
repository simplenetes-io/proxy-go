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
if ! command -v go >/dev/null; then
    printf "Unable to find external program: go\n" >&2
    exit 1
fi
if ! command -v netcat >/dev/null; then
    printf "Unable to find external program: netcat\n" >&2
    exit 1
fi

# Environment variables
DOCKER_IP=${DOCKER_IP:-127.0.0.1}

# Data
golang_image="golang:1.14-alpine"
webserver_container_name="simplenetes_proxy_test_webserver"
webserver_port=30998
proxy_container_name="simplenetes_proxy_test_proxy"
proxy_port=8888
nginx_image="nginx:alpine"
nginx_container_name="proxy_test_nginx"
haproxy_image="haproxy:2.1-alpine"
haproxy_conf_path="./test/haproxy.cfg"
haproxy_container_name="proxy_test_haproxy"
haproxy_port_regular=8000
haproxy_port_proxyprotocol_unmapped=8001
haproxy_port_proxyprotocol_mapped=29999

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
    if ! _output="$(printf "%s" "${_status}" | grep "^webserver [0-9]* (30998)$" 2>/dev/null)"; then
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

# TODO: FIXME:
#_test_request_proxy_protocol_mapping_inactive()

_test_request_proxy_protocol_mapping_unmapped()
{
    host="$1"
    port="${proxy_port}"
    clusterPort="$2"
    printf "Host: send PROXY PROTOCOL request to %s (unmapped)..." "${port}"

    # Verify status code
    if ! _status="$(printf "PROXY TCP4 192.168.0.1 192.168.0.11 56324 %s\r\n" "${clusterPort}" | nc "${host}" "${port}")"; then
        printf " failed to send request to %s. Data: %s\n" "${port}" "${_status}"
        exit 1
    fi

    # Validate response
    if ! _output="$(printf "%s" "${_status}" | grep -E "^go away$" 2>/dev/null)"; then
        printf " unexpected return after sending request to %s. Data: %s\n" "${port}" "${_status}"
        exit 1
    fi

    printf " %s. OK\n" "${_output}"
}

_test_request_proxy_protocol_mapping_mapped()
{
    host="$1"
    port="$2"
    printf "Host: send PROXY PROTOCOL request to %s (mapped)..." "${port}"

    # Run request in background
    printf "starting in background...\n"
    printf "PROXY TCP4 192.168.0.1 192.168.0.11 56324 ${haproxy_port_proxyprotocol_mapped}\r\n" | nc "${host}" "${port}" > "./tmp_output.txt" &
    _pid="$!"

    sleep 3
    _status=$(cat "./tmp_output.txt")
    kill "${_pid}"

    # Validate response
    if ! _output="$(printf "%s" "${_status}" | grep -E "^go ahead$" 2>/dev/null)"; then
        printf " unexpected return after sending request to %s. Data: %s\n" "${port}" "${_status}"
        exit 1
    fi

    printf " %s. OK\n" "${_output}"
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
printf "8888:[1001,2001,3001,30998,9999]" > ./test/ports.cfg
# Container
printf "Docker: removing existing %s container...\n" "${proxy_container_name}"
docker stop "${proxy_container_name}" && docker rm "${proxy_container_name}"
printf "Docker: running proxy on port %s..." "${proxy_port}"
docker run --name "${proxy_container_name}" --network="host" -v "$PWD:/${proxy_container_name}" --workdir=/${proxy_container_name} "${golang_image}" nohup sh -c "go run ./src/entrypoint.go ${DOCKER_IP}" &
sleep 3
printf " OK\n"

# Initialize nginx server
# Container
printf "======\n[Docker]\n"
printf "Docker: removing existing %s container\n" "${nginx_container_name}"
docker stop "${nginx_container_name}" && docker rm "${nginx_container_name}"
printf "Docker: setting up new container\n"
docker run -d --name "${nginx_container_name}" --network="host" -v $PWD/test/nginx.conf:/etc/nginx/nginx.conf:ro "${nginx_image}"
sleep 3
printf " OK\n"

# Initialize haproxy
# Container
printf "======\n[Docker]\n"
printf "Docker: removing existing %s container\n" "${haproxy_container_name}"
docker stop "${haproxy_container_name}" && docker rm "${haproxy_container_name}"
printf "Docker: setting up new container\n"
docker run --name "${haproxy_container_name}" --network="host" -v "$PWD:/${haproxy_container_name}" --workdir "/${haproxy_container_name}" "${haproxy_image}" nohup sh -c "haproxy -f ${haproxy_conf_path}" &
sleep 3
printf " OK\n"

# Check proxy is reachable,
# then send request and validate response
printf "======\n[Proxy]\n"
_test_reachable "${proxy_port}"
_test_request "${proxy_port}"

# Send SIGHUP to proxy
printf "======\n[Docker]\n"
printf "Docker: sending HUP signal to proxy..."
docker exec -it "${proxy_container_name}" sh -c "kill -s HUP \$(pgrep exe/entrypoint)"
printf " OK\n"

# Repeat sending request and validating response
printf "======\n[Proxy]\n"
_test_request "${proxy_port}"

# Push configuration changes
printf "======\n[Docker]\n"
printf "7777:[1001,2001,3001,30998,9999]" > ./test/ports.cfg
# Send SIGHUP to proxy
printf "Docker: sending HUP signal to proxy..."
docker exec -it "${proxy_container_name}" sh -c "kill -s HUP \$(pgrep exe/entrypoint)"
printf " OK\n"

# Send request to previous proxy port,
# then point to the new port and repeat request
printf "======\n[Proxy]\n"
_test_request_failure "${proxy_port}"
proxy_port=7777
_test_request "${proxy_port}"

# Restore proxy port to initial state
printf "======\n[Docker]\n"
printf "8888:[1001,2001,3001,30998,9999]" > ./test/ports.cfg
printf "Docker: sending HUP signal to proxy..."
docker exec -it "${proxy_container_name}" sh -c "kill -s HUP \$(pgrep exe/entrypoint)"
printf " OK\n"

# Send request to previous proxy port,
# then point to the new port and repeat request
printf "======\n[Proxy]\n"
_test_request_failure "${proxy_port}"
proxy_port=8888
_test_request "${proxy_port}"

# Send request to new proxy
printf "======\n[New proxy]\n"
proxy_port=32767
# Cannot send regular request to 32767
_test_request_failure "${proxy_port}"
# Send PROXY PROTOCOL request
_test_request_proxy_protocol_mapping_unmapped "${DOCKER_IP}" "${proxy_port}"

# Send request to new proxy via Haproxy
# Regular proxy backend is expected to fail
_test_request_failure "${haproxy_port_regular}"
# Proxy protocol request via send-proxy
# Unmapped
_test_request_proxy_protocol_mapping_unmapped "${DOCKER_IP}" "${haproxy_port_proxyprotocol_unmapped}"
# TODO: FIXME: mapped but inactive ?
# Mapped
_test_request_proxy_protocol_mapping_mapped "${DOCKER_IP}" "${haproxy_port_proxyprotocol_mapped}"

# sendProxy flag
printf "======\n[New proxy - sendProxy]\n"
_proxyrequest_file="./proxyrequest"
_proxyoutput_file="./proxyoutput"
_netcatpid_file="./netcatpid"
rm "${_proxyrequest_file}"
mkfifo "${_proxyrequest_file}"
tail -f "${_proxyrequest_file}" | netcat ${DOCKER_IP} ${proxy_port} > "${_proxyoutput_file}" &
echo $! > "${_netcatpid_file}"
printf "PROXY TCP4 1.2.3.4 5.6.7.8 56324 9999\r\n" > "${_proxyrequest_file}"
sleep 2
_check_go_ahead=$(cat "${_proxyoutput_file}")
if [ "${_check_go_ahead}" != "go ahead" ]; then
    printf " expected go ahead return from request to sendProxy host\n"
    exit 1
fi

printf "GET / HTTP/1.0\r\nHost: test\r\n\n\n" > "${_proxyrequest_file}"
_expected_return="go ahead
HTTP/1.1 200 OK
Server: nginx/1.17.10
Content-Type: text/plain
Content-Length: 20
Connection: close

Hello world: 1.2.3.4"
_check_return="$(cat "${_proxyoutput_file}" | grep -v "Date:" | tr -d '\r')"
if [ "${_check_return}" != "${_expected_return}" ]; then
    printf " expected return data from request to sendProxy host to match. Got: %s. Expected: %s\n" "${_check_return}" "${_expected_return}"
    exit 1
fi
sleep 2
printf "%s\n" "$(cat "${_proxyoutput_file}")"
kill $(cat "${_netcatpid_file}")
rm "${_netcatpid_file}"
rm "${_proxyrequest_file}"
rm "${_proxyoutput_file}"

# cluster ports proxy
printf "======\n[Cluster ports proxy]\n"
go run ./src/entrypoint.go "127.0.0.1" &
sleep 3
_local_execution_pid="$!"
_test_request_proxy_protocol_mapping_unmapped "127.0.0.1" "${haproxy_port_proxyprotocol_mapped}"
sleep 3
kill "${_local_execution_pid}"


# TODO: FIXME: find a way to automate call and responses between client and hosts (see doc/MANUAL_VERIFICATION.md)


# Clean up
docker stop "${webserver_container_name}" && docker rm "${webserver_container_name}"
docker stop "${proxy_container_name}" && docker rm "${proxy_container_name}"
docker stop "${nginx_container_name}" && docker rm "${nginx_container_name}"
docker stop "${haproxy_container_name}" && docker rm "${haproxy_container_name}"

printf "Tests completed!\n"
