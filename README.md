# Simplenetes proxy
A single process composed of two parts: cluster ports proxy and local ports proxy.

The cluster ports proxy goal is to listen to a range of ports, referred to as cluster ports, and to proxy connections to the respective cluster hosts.

The local ports proxy serves to proxy traffic coming in from a single port to local ports, in sequenced order, exiting upon first success or on failure at the end of the configured sequence.

### Run
Serve on all local interfaces:
```sh
go run src/entrypoint.go
```

Serve on specific host IP:
```sh
go run src/entrypoint.go 192.168.99.100
```

### Tests

Run all proxy verification tests inside a container:  
```sh
./test/run_as_containers.sh
```

If _Docker_ is not running on `localhost`, then set the _Docker IP_ to the respective _VM_ address by redefining the `DOCKER_IP` environment variable:  
```sh
DOCKER_IP=192.168.99.100 ./test/run_as_containers.sh
```
