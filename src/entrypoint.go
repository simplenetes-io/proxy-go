// Simplenetes Proxy
// Keywords: TCP, node, host, relay, proxy, forward

package main

import (
    "bufio"
    "bytes"
    "fmt"
    "io"
    "log"
    "net"
    "os"
    "os/signal"
    "regexp"
    "strconv"
    "strings"
    "syscall"
    "time"
)


// Data
type ConfigurationMap map[int][]int;

type PortsConfigurationData struct {
    hostPort int;
    maxConnections int;
    sendProxyFlag bool;
}

type PortsConfigurationMap map[int][]PortsConfigurationData;


/*============================
 loadConfiguration

 This procedure takes a configuration file path, opens the file, then
 extracts the file contents and store them into a key-value map.

 Configuration file format:
    inPort1:[outPort1,outPort2,...,outPortN]
    inPort2:[outPortA,outPortB,...,outPortM]
    inPort3:[outPortU,outPortV,...,outPortZ]
    [...]

 Parameters:
    cfgFilePath: local path to configuration file

 Returns:
    Loaded configuration map
============================*/
func loadConfiguration(cfgFilePath string) (ConfigurationMap) {
    // Open configuration file
    var cfgFile = func(filePath string) (*os.File) {
        file, err := os.Open(filePath);
        if(err != nil) {
            log.Printf("Error opening file %s for reading: %v", filePath, err);
            os.Exit(1);
        }
        return file;
    } (cfgFilePath);
    defer cfgFile.Close();

    // Extract data from configuration file
    var portsConfiguration = func(file *os.File) (ConfigurationMap) {
        var data = make(ConfigurationMap);
        var scanner *bufio.Scanner = bufio.NewScanner(file);

        // Try to iterate over all file contents
        for scanner.Scan() {
            // Data
            var err error = nil;
            var inPort int;
            var outPorts []int;

            // Read line
            var line string;
            var lineSplit []string;
            line = scanner.Text();
            lineSplit = strings.Split(line, ":");
            if(len(lineSplit) != 2) {
                log.Printf("Error while reading configuration line: %s. Expected format: inPort1:[outPort1,outPort2,...,outPortN]", line);
                os.Exit(1);
            }
            log.Printf("Configuration line: %s\n", line);

            //
            // First half: read inPort
            inPort, err = strconv.Atoi(lineSplit[0]);
            if(err != nil) {
                log.Printf("Error converting data: %s. Message: %v", lineSplit[0], err);
                os.Exit(1);
            }

            //
            // Second half: read outPorts

            // Extract ports from brackets
            var regex = regexp.MustCompile(`\[(.*?)\]`);
            var regexFindInLine = regex.FindStringSubmatch(lineSplit[1]);
            if(len(regexFindInLine) != 2) {
                log.Printf("Error finding outPort submatch for configuration line: %s. Expected format: inPort1:[outPort1,outPort2,...,outPortN]", line);
                os.Exit(1);
            }

            // Extract only the regex capture part
            var regexResult = regexFindInLine[1];

            // Extract individual ports
            var portsStr = strings.Split(regexResult, ",");
            if(len(portsStr) < 1) {
                log.Printf("Error finding listed outPorts in configuration line: %s. Expected format: inPort1:[outPort1,outPort2,...,outPortN]", line);
                os.Exit(1);
            }

            // Copy individual ports to array
            var portsIndex int;
            var outPortsLen int;
            outPortsLen = len(portsStr);
            outPorts = make([]int, outPortsLen, 1024);
            for portsIndex=0; portsIndex < outPortsLen; portsIndex++ {
                var data = portsStr[portsIndex];
                outPorts[portsIndex], err = strconv.Atoi(data);
                if(err != nil) {
                    log.Printf("Error converting data: %s. Message: %v", data, err);
                    os.Exit(1);
                }
            }

            // Store current entry
            // TODO: FIXME: flag duplicated entries and overwrites
            data[inPort] = outPorts;
        }

        // In case of error during Scan(), expect to catch error here
        var err error = nil;
        err = scanner.Err();
        if(err != nil) {
            log.Printf("Error reading from file: %v", err);
            os.Exit(1);
        }

        // Otherwise, assume data is in good condition
        return data;
    } (cfgFile);

    // Return loaded configuration data
    return portsConfiguration;
}

/*============================
 loadPortsConfiguration

 This procedure takes the local proxy ports configuration file path, opens the file,
 then extracts the file contents and store them into a key-value map.

 Base configuration entry format:
    clusterPort:hostPort:maxConnections:sendProxyFlag

 Configuration example:
    #clusterPortA:hostPort1:100:true clusterPortA:hostPort2:100:false
    clusterPortB:hostPort10:100:false clusterPortB:hostPort11:100:false
    [...]
    ### EOF

 Parameters:
    cfgFilePath: local path to configuration file

 Returns:
    Loaded ports configuration map
============================*/
func loadPortsConfiguration(cfgFilePath string) (PortsConfigurationMap) {
    // Open configuration file
    var cfgFile = func(filePath string) (*os.File) {
        file, err := os.Open(filePath);
        if(err != nil) {
            log.Printf("Error opening file %s for reading: %v", filePath, err);
            os.Exit(1);
        }
        return file;
    } (cfgFilePath);
    defer cfgFile.Close();

    // Extract data from configuration file
    var portsConfiguration = func(file *os.File) (PortsConfigurationMap) {
        var data = make(PortsConfigurationMap);
        var scanner *bufio.Scanner = bufio.NewScanner(file);

        // Try to iterate over all file contents
        for scanner.Scan() {
            // Data
            //var err error = nil;

            // Read line
            var line string;
            var lineSplit []string;
            var lineSplitLen int;
            line = scanner.Text();
            log.Printf("Configuration line: %s\n", line);

            // Skip commented out line
            if(line[0] == '#') {
                continue;
            }

            // Split by space-separated entries,
            // then iterate over all entries
            var lineSplitIndex int;
            lineSplit = strings.Split(line, " ");
            lineSplitLen = len(lineSplit);
            var portsDataList []PortsConfigurationData;
            portsDataList = make([]PortsConfigurationData, lineSplitLen);

            var clusterPort int;
            for lineSplitIndex=0; lineSplitIndex < lineSplitLen; lineSplitIndex++ {
                var currentEntry string = lineSplit[lineSplitIndex];
                var currentEntryValues []string = strings.Split(currentEntry, ":");
                var currentEntryValuesLen int = len(currentEntryValues);
                if(currentEntryValuesLen != 4) {
                    log.Printf("Error while reading configuration line: %s. Expected format: clusterPort:hostPort:maxConnections:sendProxyFlag. Values: %s. Length: %d", currentEntry, currentEntryValues, currentEntryValuesLen);
                    os.Exit(1);
                } else {
                    log.Printf("Parsing configuration entry: %s\n", currentEntry);
                    var err error;
                    var portsData PortsConfigurationData;
                    portsData.hostPort, err = strconv.Atoi(currentEntryValues[1]);
                    if(err != nil) {
                        log.Printf("Error converting hostPort: %s. Message: %v", currentEntryValues[1], err);
                        os.Exit(1);
                    }
                    portsData.maxConnections, err = strconv.Atoi(currentEntryValues[2]);
                    if(err != nil) {
                        log.Printf("Error converting maxConnections: %s. Message: %v", currentEntryValues[2], err);
                        os.Exit(1);
                    }
                    portsData.sendProxyFlag, err = strconv.ParseBool(currentEntryValues[3]);
                    if(err != nil) {
                        log.Printf("Error converting sendProxyFlag: %s. Message: %v", currentEntryValues[3], err);
                        os.Exit(1);
                    }

                    clusterPort, err = strconv.Atoi(currentEntryValues[0]);
                    if(err != nil) {
                        log.Printf("Error converting clusterPort: %s. Message: %v", currentEntryValues[1], err);
                        os.Exit(1);
                    }
                    portsDataList[lineSplitIndex] = portsData;
                }
            }

            data[clusterPort] = portsDataList;
        }

        // In case of error during Scan(), expect to catch error here
        var err error = nil;
        err = scanner.Err();
        if(err != nil) {
            log.Printf("Error reading from file: %v", err);
            os.Exit(1);
        }

        // Otherwise, assume data is in good condition
        return data;
    } (cfgFile);

    // Return loaded configuration data
    return portsConfiguration;
}
func loadListener(networkMode string, clusterAddress string, previousPortsConfiguration ConfigurationMap, newPortsConfiguration ConfigurationMap, currentListeners *map[int]net.Listener) () {
    var port int;
    // close all open ports which are no longer part of configuration
    for port = range (*currentListeners) {
        if(newPortsConfiguration[port] == nil) {
            log.Printf("Listener port changed. Shutting down previous listener on %d...\n", port);
            (*currentListeners)[port].Close();
            delete(*currentListeners, port);
        } else {
            var portMappingChanged bool = false;
            log.Printf("Comparing previous and new port mappings: %v:%v\n", previousPortsConfiguration[port], newPortsConfiguration[port]);
            if(len(previousPortsConfiguration[port]) != len(newPortsConfiguration[port])) {
                portMappingChanged = true;
            } else {
                for _, mappedPort := range previousPortsConfiguration[port] {
                    log.Printf("Looking for %d in %v", mappedPort, newPortsConfiguration[port]);
                    var foundValue = false;
                    for _, value := range newPortsConfiguration[port] {
                        if(mappedPort == value) {
                            log.Printf("Found %d in %v", mappedPort, newPortsConfiguration[port]);
                            foundValue = true;
                            break;
                        }
                    }
                    if(!foundValue) {
                        log.Printf("Unable to find %d in %v. Flagging port mapping as changed", mappedPort, newPortsConfiguration[port]);
                        portMappingChanged = true;
                        break;
                    }
                }
            }

            if(portMappingChanged) {
                log.Printf("Port mapping changed. Shutting down previous listener on %d...\n", port);
                (*currentListeners)[port].Close();
                delete(*currentListeners, port);
            }
        }
    }

    // Handle reloading
    for port = range newPortsConfiguration {
        log.Printf("Listener port: %v\n", (*currentListeners)[port]);
        if((*currentListeners)[port] == nil) {
            // Input : announce and listen to incoming connections
            (*currentListeners)[port] = func(mode string, address string) (net.Listener) {
                listen, err := net.Listen(mode, address);
                if(err != nil) {
                    log.Printf("Error listening to %s in mode %s: %v", address, mode, err);
                    os.Exit(1);
                }
                log.Printf("Listening to %s", address);
                return listen;
            } (networkMode, clusterAddress + ":" + strconv.Itoa(port));
        }
    }
}

func handlePorts(networkMode string, hostAddress string, portsConfiguration ConfigurationMap, listeners *map[int]net.Listener) {
    var listenerPort int;
    var listener net.Listener;
    log.Printf("Listeners: %v\n", *listeners);
    for listenerPort, listener = range (*listeners) {
        log.Printf("Current listener (%d): %v\n", listener, listenerPort);
            var hostPorts []int;
            hostPorts = portsConfiguration[listenerPort];

            log.Printf("%s is waiting for connection...\n", listener.Addr());
            go func(listener net.Listener) {
            // Transform: forward all connections to handler
                for {
                    // Take the connection
                    var connection = func(listen net.Listener) (net.Conn) {
                        conn, err := listen.Accept();
                        if(err != nil) {
                            log.Printf("Error accepting connection: %v", err);
                        } else {
                            log.Printf("Accepted connection from %s (via %s)", conn.LocalAddr(), conn.RemoteAddr());
                        }
                        return conn;
                    } (listener);

                    // Pass the connection to handler
                    if(connection != nil) {
                        defer connection.Close();
                        go func(mode string, address string, ports []int, conn net.Conn) {
                            var hostConnection net.Conn;
                            var err error;

                            // Iterate over all host ports trying to connect to host
                            var hostPortsIndex int;
                            var hostPortsLen = len(ports);
                            log.Printf("Current number of configured host ports: %d", hostPortsLen);
                            for hostPortsIndex=0; hostPortsIndex < hostPortsLen; hostPortsIndex++ {
                                var host = address + ":" + strconv.Itoa(ports[hostPortsIndex]);
                                hostConnection, err = net.Dial(mode, host);
                                if(err == nil) {
                                    log.Printf("Connected to %s", host);

                                    // Input: Send data from received connection to host
                                    go func() {
                                        var err error;
                                        _, err = io.Copy(conn, hostConnection);
                                        if(err != nil) {
                                            log.Printf("Error copying data from cluster to host: %v", err);
                                            hostConnection.Close();
                                            return;
                                        }
                                    }();

                                    // Output: send data from host back to the original connection
                                    go func() {
                                        var err error;
                                        _, err = io.Copy(hostConnection, conn);
                                        if(err != nil) {
                                            log.Printf("Error copying data from host to cluster: %v", err);
                                            hostConnection.Close();
                                            return;
                                        }
                                    }();

                                    // End host ports loop
                                    break;
                                } else {
                                    log.Printf("Error connecting to %s in mode %s. Message: %v", host, mode, err);
                                    // TODO: FIXME: send error reply when unable to find any match
                                }
                            }
                        } (networkMode, hostAddress, hostPorts, connection);
                    } else {
                        log.Printf("Ports routine for %s, over and out!\n", listener.Addr());
                        return;
                    }
                }
            } (listener);
    }
}

func main() {
    // Arguments
    var host string;
    if(len(os.Args) > 1) {
        host = os.Args[1];
    } else {
        host = "localhost";
    }

    // Program settings
    //TODO: FIXME: enable customizable configuration path
    //const configurationFile string = "cfg/ports.cfg";
    const configurationFile string = "test/ports.cfg";
    const portsConfigurationFile string = "test/ports.conf";

    // Network settings
    const networkMode string = "tcp";
    const listenerHost string = "";
    const listenerPort int = 32767;

    // Cluster settings (in)
    // Host settings (out)
    var clusterAddress string = host;
    var hostAddress string = host;
    log.Printf("Loading configuration...\n");
    var portsConfiguration ConfigurationMap = loadConfiguration(configurationFile);
    var listeners map[int]net.Listener;
    listeners = make(map[int]net.Listener);

    var newPortsConfiguration PortsConfigurationMap = loadPortsConfiguration(portsConfigurationFile);
    log.Printf("Ports configuration: %v\n", newPortsConfiguration);

    // Start listener
    // Input : announce and listen to incoming connections
    var listener net.Listener = func(mode string, address string) (net.Listener) {
        listen, err := net.Listen(mode, address);
        if(err != nil) {
            log.Printf("Error listening to %s in mode %s: %v", address, mode, err);
            os.Exit(1);
        }
        log.Printf("Listening to %s", address);
        return listen;
    } (networkMode, listenerHost + ":" + strconv.Itoa(listenerPort));

    // Handle listener connections
    go func(listener net.Listener) {
        // Transform: forward all connections to handler
        for {
            // Take a new connection
            var connection net.Conn;
            var err error;
            connection, err = listener.Accept();
            if(err != nil) {
                log.Printf("Error accepting connection: %v", err);
                return;
            } else {
                log.Printf("Accepted connection from %s (via %s)", connection.LocalAddr(), connection.RemoteAddr());
            }

            // Transform: forward connection to handler
            go func(conn net.Conn) {
                log.Printf("Handling remote connection: %s\n", connection.RemoteAddr());

                // Mark connection to be closed on exit
                defer func() {
                    log.Printf("Closing remote connection: %s\n", connection.RemoteAddr());
                    err = connection.Close();
                    if(err != nil) {
                        log.Printf("Error closing connection: %s. Error: %v", connection.RemoteAddr(), err);
                    }
                }();

                // Check presence of proxy protocol
                var clientIp, proxyIp, clientPort, proxyPort = func() (string, string, int, int) {
                    var err error;
                    var connectionReader *bufio.Reader;
                    connectionReader = bufio.NewReader(connection);
                    var connectionReaderBufferCount int;
                    var connectionReaderBuffer []byte;

                    // Check proxy protocol header
                    const proxyProtocolHeaderString string = "PROXY ";
                    const proxyProtocolHeaderStringLen int = len(proxyProtocolHeaderString);
                    connectionReaderBuffer = make([]byte, proxyProtocolHeaderStringLen);
                    // Read initial header
                    connectionReaderBufferCount, err = connectionReader.Read(connectionReaderBuffer);
                    // Check header buffer is valid, count matches expected length and buffer match expected content
                    if(err != nil || connectionReaderBufferCount != proxyProtocolHeaderStringLen ||
                        !bytes.Equal(connectionReaderBuffer, []byte(proxyProtocolHeaderString))) {
                        log.Printf("Error parsing proxy protocol header prefix: %s. Error: %v", connectionReaderBuffer, err);
                        return "", "", 0, 0;
                    }

                    // Check case of unknown proxy protocol
                    const proxyProtocolUnknownString string = "UNKNOWN\r\n";
                    var proxyProtocolUnknownStringLen int = len(proxyProtocolUnknownString);
                    connectionReaderBuffer, err = connectionReader.Peek(proxyProtocolUnknownStringLen);
                    // Check unknwon buffer is valid and data matches expected content
                    if(err != nil || bytes.Equal(connectionReaderBuffer, []byte(proxyProtocolUnknownString))) {
                        log.Printf("Error parsing proxy protocol unknown: %v", err);
                        return "", "", 0, 0;
                    }

                    // Check TCP4 proxy protocol case
                    // Reference: "PROXY TCP4 255.255.255.255 255.255.255.255 65535 65535\r\n"
                    // TODO: add support to TCP6
                    const proxyProtocolTCP4String string = "TCP4 ";
                    const proxyProtocolTCP4StringLen int = len(proxyProtocolTCP4String);
                    connectionReaderBuffer = make([]byte, proxyProtocolTCP4StringLen);
                    connectionReaderBufferCount, err = connectionReader.Read(connectionReaderBuffer)
                    // Check buffer is valid, count matches expected length and buffer matches expected content
                    if(err != nil || connectionReaderBufferCount != proxyProtocolTCP4StringLen ||
                        !bytes.Equal(connectionReaderBuffer, []byte(proxyProtocolTCP4String))) {
                        log.Printf("Error parsing proxy protocol inet protocol: %s. Error: ", connectionReaderBuffer, err);
                        return "", "", 0, 0;
                    }

                    // Read client IP address
                    var proxyProtocolClientIpString string;
                    proxyProtocolClientIpString, err = connectionReader.ReadString(' ');
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol client IP: %v", err);
                        return "", "", 0, 0;
                    }
                    // Adjust string
                    var proxyProtocolClientIpStringLen int;
                    proxyProtocolClientIpStringLen = len(proxyProtocolClientIpString);
                    proxyProtocolClientIpString = proxyProtocolClientIpString[:proxyProtocolClientIpStringLen-1];
                    // Parse IP
                    var proxyProtocolClientIp net.IP;
                    proxyProtocolClientIp = net.ParseIP(proxyProtocolClientIpString);
                    if(proxyProtocolClientIp == nil) {
                        log.Printf("Error parsing client IP: %s", proxyProtocolClientIpString);
                        return "", "", 0, 0;
                    }

                    // Read proxy IP address
                    var proxyProtocolProxyIpString string;
                    proxyProtocolProxyIpString, err = connectionReader.ReadString(' ');
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol proxy IP: %v", err);
                        return "", "", 0, 0;
                    }
                    // Adjust string
                    var proxyProtocolProxyIpStringLen int;
                    proxyProtocolProxyIpStringLen = len(proxyProtocolProxyIpString);
                    proxyProtocolProxyIpString = proxyProtocolProxyIpString[:proxyProtocolProxyIpStringLen-1];
                    // Parse IP
                    var proxyProtocolProxyIp net.IP;
                    proxyProtocolProxyIp = net.ParseIP(proxyProtocolProxyIpString);
                    if(proxyProtocolProxyIp == nil) {
                        log.Printf("Error parsing proxy IP: %s", proxyProtocolClientIpString);
                        return "", "", 0, 0;
                    }

                    // Read client port number
                    var proxyProtocolClientPortString string;
                    proxyProtocolClientPortString, err = connectionReader.ReadString(' ');
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol client port: %v", err);
                        return "", "", 0, 0;
                    }
                    // Adjust number
                    var proxyProtocolClientPortStringLen int;
                    proxyProtocolClientPortStringLen = len(proxyProtocolClientPortString);
                    proxyProtocolClientPortString = proxyProtocolClientPortString[:proxyProtocolClientPortStringLen-1];
                    // Parse port
                    var proxyProtocolClientPort int;
                    proxyProtocolClientPort, err = strconv.Atoi(proxyProtocolClientPortString);
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol client port: %v", err);
                        return "", "", 0, 0;
                    }

                    // Read proxy port number
                    var proxyProtocolProxyPortString string;
                    proxyProtocolProxyPortString, err = connectionReader.ReadString('\r');
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol proxy port: %v", err);
                        return "", "", 0, 0;
                    }
                    // Adjust number
                    var proxyProtocolProxyPortStringLen int;
                    proxyProtocolProxyPortStringLen = len(proxyProtocolProxyPortString);
                    proxyProtocolProxyPortString = proxyProtocolProxyPortString[:proxyProtocolProxyPortStringLen-1];
                    // Parse port
                    var proxyProtocolProxyPort int;
                    proxyProtocolProxyPort, err = strconv.Atoi(proxyProtocolProxyPortString);
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol proxy port: %v", err);
                        return "", "", 0, 0;
                    }

                    // Read trailing characters
                    var proxyProtocolTrailingByte byte;
                    proxyProtocolTrailingByte, err = connectionReader.ReadByte();
                    if(err != nil || proxyProtocolTrailingByte != '\n') {
                        log.Printf("Error parsing proxy protocol trailing byte: %v", err);
                        return "", "", 0, 0;
                    }

                    return proxyProtocolClientIpString, proxyProtocolProxyIpString, proxyProtocolClientPort, proxyProtocolProxyPort;
                } ();

                // Reply port mapping status
                const responseMappingActive string = "go ahead\n"
                const responseMappingInactive string = "go away\n"
                log.Printf("Reading back proxy protocol line. inet: tcp | Remote clientip: %s, clientport %d | Proxy proxyip: %s, proxyport: %d\n", clientIp, clientPort, proxyIp, proxyPort);
                if(proxyPort == 0) {
                    log.Printf("Error reading back from proxy protocol line. Proxy port: %d", proxyPort);
                    return;
                } else {
                    if(newPortsConfiguration[proxyPort] != nil) {
                        fmt.Fprintf(connection, responseMappingActive);
                    } else {
                        fmt.Fprintf(connection, responseMappingInactive);
                    }
                }
            } (connection);
        }
    } (listener);

    // Install watcher for ports configuration file changes
    var watchForFileChanges = func(filePath string, channel chan bool) {
        var fileStatBase os.FileInfo;
        var err error;

        // Mark initial file state
        fileStatBase, err = os.Stat(filePath);
        if(err != nil) {
            log.Printf("Error trying to stat file %s: %v", fileStatBase, err);
            channel <- false;
            return;
        }

        // Watch for changes by comparing the modification time and file size
        for {
            var fileStatNow os.FileInfo;
            fileStatNow, err = os.Stat(filePath);
            if(err != nil) {
                log.Printf("Error trying to stat file %s: %v", fileStatNow, err);
                channel <- false;
                return;
            }

            if(fileStatNow.ModTime() != fileStatBase.ModTime() || fileStatNow.Size() != fileStatBase.Size()) {
                break;
            }

            time.Sleep(2 * time.Second);
        }

        // Return
        if(err != nil) {
            log.Printf("Error while watching for file %s: %v", filePath, err);
            channel <- false;
        } else {
            channel <- true;
        }
    }
    var watchChannel chan bool;
    go func() {
        var fileHasChanged bool;
        for {
            watchChannel = make(chan bool);
            go watchForFileChanges(portsConfigurationFile, watchChannel);
            fileHasChanged = <-watchChannel;
            if(fileHasChanged) {
                log.Printf("Ports configuration file has changed: %s. Reloading...\n", portsConfigurationFile);
                newPortsConfiguration = loadPortsConfiguration(portsConfigurationFile);
                log.Printf("Ports configuration: %v\n", newPortsConfiguration);
            }
        }
    } ();

    // Install SIGHUP for handling reload
    var signalChannel chan os.Signal = make(chan os.Signal, 1);
    go func() {
        var signal os.Signal;
        for signal = range signalChannel {
            switch signal {
                case syscall.SIGHUP:
                    log.Printf("Reloading configuration file...\n");
                    var previousPortsConfiguration = portsConfiguration;
                    portsConfiguration = loadConfiguration(configurationFile);
                    loadListener(networkMode, clusterAddress, previousPortsConfiguration, portsConfiguration, &listeners);
                    go handlePorts(networkMode, hostAddress, portsConfiguration, &listeners);
            }
        }
    } ();
    signal.Notify(signalChannel, syscall.SIGHUP);

    var previousPortsConfiguration = portsConfiguration;
    loadListener(networkMode, clusterAddress, previousPortsConfiguration, portsConfiguration, &listeners);
    go handlePorts(networkMode, hostAddress, portsConfiguration, &listeners);

    // TODO: consider readding sync.WaitGroup -> Wait instead of infinite loop
    //for {}
    <-watchChannel;
}
