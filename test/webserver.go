//
// This is a reference webserver used for testing.
//
// The server defines a randomly generated number to act as identifier during initialization,
// then replies to requests to the root path echoing back the identification together
// with the port where it is listening to.
//
// Usage:
//   go run webserver.go 8080
//

package main

import (
    "math/rand"
    "net/http"
    "os"
    "strconv"
    "time"
)

func main() {
    // Data
    rand.Seed(time.Now().UnixNano());
    var id string = strconv.Itoa(rand.Int());
    var port string = os.Args[1];

    // Define server response
    http.HandleFunc("/", func(response http.ResponseWriter, request *http.Request) {
        response.Write([]byte("webserver " + id + " (" + port + ")\n"));
    });

    // Start server
    var err error = nil;
    err = http.ListenAndServe(":" + port, nil);
    if(err != nil) {
        panic(err);
    }
}
