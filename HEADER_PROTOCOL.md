# Header protocol

Currently using custom protocol as the header format (a.k.a. one-liner).

## Algorithm
```
cat rawbytes | nc 29999
  -> proxy adds internal header "CP=29999\n"
  -> proxy2 parses 29999 as the CP, strips away "CP=29999\n" from stream
  -> connects to host port
  -> apply logic of stripping/prepending PP header to rawbytes.
```

* The internal header is required for direct connections to `32767`.
* The PP header is the proxy protocol header which haproxy will send, but a pod will likely not send.
