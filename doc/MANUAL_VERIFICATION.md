# Manual local ports proxy verification

1. Set `clusterPort` `29999` configuration
```
29999:30000:100:false 29999:30001:100:false 29999:30002:100:false 29999:30998:100:false 29999:30003:100:false
```

2. Turn host on port `30998` on
```
nc -l -p 30998
```

3. Connect to `proxyIp=29999` via `32767`
```
cat <(printf "PROXY TCP4 192.168.0.1 192.168.0.11 56324 29999\r\n") - | nc 127.0.0.1 32767
go ahead
```

Proxy logs (observed output):
```
Accepted connection from 127.0.0.1:32767 (via 127.0.0.1:55954)
Handling remote connection: 127.0.0.1:55954
Reading back proxy protocol line. inet: tcp | Remote clientip: 192.168.0.1, clientport 56324 | Proxy proxyip: 192.168.0.11, proxyport: 29999
Current number of configured host ports: 5
Error connecting to localhost:30000 in mode tcp. Message: dial tcp [::1]:30000: connect: connection refused
Error connecting to localhost:30001 in mode tcp. Message: dial tcp [::1]:30001: connect: connection refused
Error connecting to localhost:30002 in mode tcp. Message: dial tcp [::1]:30002: connect: connection refused
Connected to localhost:30998
```

Client log:
```
go ahead
Ground control to Major Tom
Cleared for take off, straight out departure
```

Host log:
```
Ground control to Major Tom
Cleared for take off, straight out departure
```

4. Replace the netcat server with a webserver instead.

Run a new host:
```
go run test/webserver.go 30998
```

5. Client connects the same way, but we now pass in the _HTTP_ request in the interactive session (lines 2 and 3)
```
cat <(printf "PROXY TCP4 192.168.0.1 192.168.0.11 56324 29999\r\n") - | nc 127.0.0.1 32767
go ahead
GET / HTTP/1.0 
Host: localhost:30998

HTTP/1.0 200 OK
Date: Fri, 03 Apr 2020 13:38:07 GMT
Content-Length: 38
Content-Type: text/plain; charset=utf-8

webserver 3503776738523685870 (30998)
```
