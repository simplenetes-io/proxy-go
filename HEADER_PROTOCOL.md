# Header protocol

Currently using proxy protocol (PP) as the header format (a.k.a. one-liner).

## cluster ports proxy
Example: port 29999

IN      | proxy                 | sendProxy* |   OUT             | host
---------------------------------------------------------------------------
header  | accepted connection   | N/A       |   nothing[1]       | connected
nothing | accepted connection   | N/A       |   dummy header[2]  | connected

* sendProxy was never available in cluster ports proxy part
[1] - header is expected to get to the next stage. That is, the request is ready to reach the host;
[2] - dummy header is provisioned by the proxy, if needed to be.

## local ports proxy
Example: port 32767

IN      | proxy                 | sendProxy |   OUT             | host
---------------------------------------------------------------------------
header  | accepted connection   | true      |   header          | connected
header  | accepted connection   | false     |   nothing         | connected

? ? ?
???nothing | accepted connection   | true      |   dummy header[1] | connected
???nothing | accepted connection   | false     |   nothing         | connected
? ? ?

[1] - dummy header is provisioned by the proxy, if needed to be.
