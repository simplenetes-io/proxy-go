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
    "sync/atomic"
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

type ProgramSettings struct {
    configurationFile string;
    portsConfigurationFile string;
    hostsConfigurationFile string;
    listenerHost string;
    listenerPort int;
    clusterPortsRangeMin int;
    clusterPortsRangeMax int;
}

type PortsConfigurationMap map[int][]PortsConfigurationData;
type HostsConfigurationMap map[string]int;

type ClientWriter struct {
    io.Writer
}

type SignalReader struct {
    io.Reader
}

func (writer ClientWriter) Write(data []byte) (int, error) {
    fmt.Printf("Yet to write: %s\n", string(data));
    if(strings.Contains(string(data), "go away") || strings.Contains(string(data), "go ahead")) {
        if(len(data) > 9 && (strings.HasPrefix(string(data), "go away\n") || strings.HasPrefix(string(data), "go ahead\n"))) {
            fmt.Printf("Got go ahead/away prefix\n");
            buf := make([]byte, len(data));
            buf = data[9:];
            fmt.Printf("Buffer: %s\n", string(buf));
            n, err := writer.Writer.Write(buf);
            fmt.Printf("wrote %d bytes\n", n);
            return n, err;
        } else {
            fmt.Printf("wrote %d bytes\n", len(string(data)));
            return len(string(data)), nil;
        }
    } else {
        n, err := writer.Writer.Write(data);
        fmt.Printf("wrote %d bytes\n", n);
        return n, err;
    }
}

var foundValidHost int32;
func (reader SignalReader) Read(dst []byte) (int, error) {
//    emptySet := make([]byte, 32768)
    n, err := reader.Reader.Read(dst);
    if(err == io.EOF) {
        fmt.Printf("EOF\n");
    } else {
        fmt.Printf("read %d bytes: %v\n", n, string(dst));
//        if(strings.Contains(string(dst), "go away") || bytes.Contains(emptySet, dst)) {
        if(strings.Contains(string(dst), "go away")) {
            // TODO: FIXME: consider returning 0 read ?
            //              ... but then the Read could not roll back...
            return n, fmt.Errorf("Not a valid host: %v. Skipping...\n", string(dst));
        } else {
            fmt.Printf("Making as valid host: %v (%d)\n", string(dst), len(string(dst)));
            atomic.StoreInt32(&foundValidHost, 1);
        }
    }
    fmt.Printf("done\n");
    return n, err;
}

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
            log.Printf("Error opening file %s for reading\n%v", filePath, err);
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
            log.Printf("Error opening file %s for reading\n%v", filePath, err);
            os.Exit(1);
        }
        return file;
    } (cfgFilePath);
    defer cfgFile.Close();

    // EOF line: check the file is ready for reading
    var err error = nil;
    const eofLine string = "### EOF\n"
    var eofLineLength int = len(eofLine);
    var cfgFileStat os.FileInfo;
    var cfgFileStatSize int64;
    var cfgFileLastLine []byte;
    var cfgFileLastLineStr string;
    var cfgFileLastLineOffset int64;
    var cfgFileLastLineBytesRead int;
    cfgFileStat, err = cfgFile.Stat();
    if(err != nil) {
        log.Printf("Error stating file %s: %v", cfgFilePath, err);
        return nil;
    }
    cfgFileStatSize = cfgFileStat.Size();
    cfgFileLastLine =  make([]byte, eofLineLength)
    cfgFileLastLineOffset = cfgFileStatSize - int64(eofLineLength);
    cfgFileLastLineBytesRead, err = cfgFile.ReadAt(cfgFileLastLine, cfgFileLastLineOffset);
    if(cfgFileLastLineBytesRead != eofLineLength) {
        log.Printf("Error reading file %s last line. Expected bytes read (%d) to be the same length as EOF line: %d", cfgFilePath, cfgFileLastLineBytesRead, eofLineLength);
        return nil;
    }
    if(err != nil) {
        log.Printf("Error reading file %s last line: %v", cfgFilePath, err);
        return nil;
    }
    cfgFileLastLine = cfgFileLastLine[:cfgFileLastLineBytesRead]
    cfgFileLastLineStr = string(cfgFileLastLine);
    if(cfgFileLastLineStr != eofLine) {
        log.Printf("Ports configuration file is still being written to. Skipping ports configuration reload...");
        return nil;
    }

    // Extract data from configuration file
    var portsConfiguration = func(file *os.File) (PortsConfigurationMap) {
        var data = make(PortsConfigurationMap);
        var scanner *bufio.Scanner = bufio.NewScanner(file);

        // Try to iterate over all file contents
        for scanner.Scan() {
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

/*============================
 loadHostsConfiguration

 This procedure takes the cluster ports proxy configuration file path, opens the file,
 then extracts the file contents and store them into a key-value map.

 Base configuration entry format:
    ipA:32767
    ipB:32767

 Configuration example:
    192.168.10.20:32767
    192.168.10.30:32767
    192.168.10.40:32767

 Parameters:
    cfgFilePath: local path to configuration file

 Returns:
    Loaded hosts configuration map
============================*/
func loadHostsConfiguration(cfgFilePath string) (HostsConfigurationMap) {
    // Open configuration file
    var cfgFile = func(filePath string) (*os.File) {
        file, err := os.Open(filePath);
        if(err != nil) {
            log.Printf("Error opening file %s for reading\n%v", filePath, err);
            os.Exit(1);
        }
        return file;
    } (cfgFilePath);
    defer cfgFile.Close();

    // Extract data from configuration file
    var hostsConfiguration = func(file *os.File) (HostsConfigurationMap) {
        var data = make(HostsConfigurationMap);
        var scanner *bufio.Scanner = bufio.NewScanner(file);

        // Try to iterate over all file contents
        for scanner.Scan() {
            // Read line
            var line string;
            line = scanner.Text();
            log.Printf("Configuration line: %s\n", line);

            // Iterate over all entries
            var currentEntry string = line;
            var currentEntryValues []string = strings.Split(currentEntry, ":");
            var currentEntryValuesLen int = len(currentEntryValues);
            if(currentEntryValuesLen != 2) {
                log.Printf("Error while reading configuration line: %s. Expected format: ip:port. Values: %s. Length: %d", currentEntry, currentEntryValues,  currentEntryValuesLen);
                os.Exit(1);
            } else {
                log.Printf("Parsing configuration entry: %s\n", currentEntry);
                var err error;
                var hostIp string;
                var hostPort int;
                hostIp = currentEntryValues[0];
                hostPort, err = strconv.Atoi(currentEntryValues[1]);
                if(err != nil) {
                    log.Printf("Error converting hostPort: %s. Message: %v", currentEntryValues[1], err);
                    os.Exit(1);
                }
                data[hostIp] = hostPort;
            }

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
    return hostsConfiguration;
}

func loadProgramSettings(cfgFilePath string) (ProgramSettings) {
    // Open configuration file
    var cfgFile = func(filePath string) (*os.File) {
        file, err := os.Open(filePath);
        if(err != nil) {
            log.Printf("Error opening file %s for reading\n%v", filePath, err);
            os.Exit(1);
        }
        return file;
    } (cfgFilePath);
    defer cfgFile.Close();

    // Extract data from configuration file
    var programSettings = func(file *os.File) (ProgramSettings) {
        var data ProgramSettings;
        var scanner *bufio.Scanner = bufio.NewScanner(file);

        // Try to iterate over all file contents
        for scanner.Scan() {
            // Read line
            var line string;
            line = scanner.Text();
            log.Printf("Program settings line: %s\n", line);

            // Iterate over all entries
            var currentEntry string = line;
            var currentEntryValues []string = strings.Split(currentEntry, "=");
            var currentEntryValuesLen int = len(currentEntryValues);
            if(currentEntryValuesLen != 2) {
                log.Printf("Error while reading program settings line: %s. Expected format: setting:value. Values: %s. Length: %d", currentEntry, currentEntryValues, currentEntryValuesLen);
                os.Exit(1);
            } else {
                log.Printf("Parsing program settings entry: %s\n", currentEntry);
                var err error;
                var setting string;
                var value string;
                setting = currentEntryValues[0];
                value = strings.Trim(currentEntryValues[1], "\"");
                switch setting {
                    case "configurationFile":
                        data.configurationFile = value;
                    case "portsConfigurationFile":
                        data.portsConfigurationFile = value;
                    case "hostsConfigurationFile":
                        data.hostsConfigurationFile = value;
                    case "listenerHost":
                        data.listenerHost = value;
                    case "listenerPort":
                        data.listenerPort, err = strconv.Atoi(value);
                        if(err != nil) {
                            log.Printf("Error converting listenerPort: %s. Message: %v", value, err);
                            os.Exit(1);
                        }
                    case "clusterPortsRangeMin":
                        data.clusterPortsRangeMin, err = strconv.Atoi(value);
                        if(err != nil) {
                            log.Printf("Error converting clusterPortsRangeMin: %s. Message: %v", value, err);
                            os.Exit(1);
                        }
                    case "clusterPortsRangeMax":
                        data.clusterPortsRangeMax, err = strconv.Atoi(value);
                        if(err != nil) {
                            log.Printf("Error converting clusterPortsRangeMax: %s. Message: %v", value, err);
                            os.Exit(1);
                        }
                    default:
                        log.Printf("Skipping unknown entry: %s", value);
                }

            }

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
    return programSettings;
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
//TODO: FIXME: remove this procedure and all its references
return;
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
                                            //conn.Close(); ? Leak ?
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
                                            //conn.Close(); ? Leak ?
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
    const programSettingsFile string = "config/settings/settings.conf";
    var programSettings ProgramSettings =  loadProgramSettings(programSettingsFile);
    log.Printf("Program settings: %v\n", programSettings);

    // Configuration settings
    var configurationFile string = programSettings.configurationFile;
    var portsConfigurationFile string = programSettings.portsConfigurationFile;
    var hostsConfigurationFile string = programSettings.hostsConfigurationFile;

    // Network settings
    const networkMode string = "tcp";
    var listenerHost string = programSettings.listenerHost;
    var listenerPort int = programSettings.listenerPort;

    // Cluster settings (in)
    // Host settings (out)
    var clusterAddress string = host;
    var hostAddress string = host;
    log.Printf("Loading configuration...\n");
    var portsConfiguration ConfigurationMap = loadConfiguration(configurationFile);
    var listeners map[int]net.Listener;
    listeners = make(map[int]net.Listener);

    var newPortsConfiguration PortsConfigurationMap = loadPortsConfiguration(portsConfigurationFile);
    if(newPortsConfiguration == nil) {
        log.Printf("Error reading ports configuration file. Expected initial configuration to be valid");
        os.Exit(1);
    }
    log.Printf("Ports configuration: %v\n", newPortsConfiguration);

    var hostsConfiguration HostsConfigurationMap = loadHostsConfiguration(hostsConfigurationFile);
    if(hostsConfiguration == nil) {
        log.Printf("Error reading hosts configuration file. Expected initial configuration to be valid");
        os.Exit(1);
    }
    log.Printf("Hosts configuration: %v\n", hostsConfiguration);

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

    // Set up max connections data
    var currentHostPortsMaxConnections map[int]int;
    currentHostPortsMaxConnections = make(map[int]int);

    // Handle listeners for range of cluster ports
    var clusterPortsRangeMin int = programSettings.clusterPortsRangeMin;
    var clusterPortsRangeMax int = programSettings.clusterPortsRangeMax;
    for clusterPort := clusterPortsRangeMin; clusterPort <= clusterPortsRangeMax; clusterPort++ {
        var listener net.Listener = func(mode string, address string) (net.Listener) {
            listen, err := net.Listen(mode, address);
            if(err != nil) {
                log.Printf("Error listening to %s in mode %s: %v", address, mode, err);
                os.Exit(1);
            }
            //log.Printf("Listening to %s", address);
            return listen;
        } (networkMode, listenerHost + ":" + strconv.Itoa(clusterPort));
        go func(listener net.Listener, currentClusterPort int) {
            // Transform: forward all connections to handler
            for {
                // Take a new connection
                var connection net.Conn;
                var err error;
                connection, err = listener.Accept();
                if(err != nil) {
                    log.Printf("[host] Error accepting connection: %v", err);
                    return;
                } else {
                    log.Printf("[host] Accepted connection from %s (via %s)", connection.LocalAddr(), connection.RemoteAddr());
                }

                // Iterate over list of hosts
                // TODO: handle one at a time iteration
                //         -> procedures can only be started the previous one signals with OK or FAILURE

                atomic.StoreInt32(&foundValidHost, 0);
                signalDone := make(chan struct{});
                var signalDoneMutex int32 = 0;
                var hostsConfigurationLen int32 = (int32)(len(hostsConfiguration));
                var hostsConfigurationCounter int32 = 0;
                var signalNextMutex int32 = 0;
                // Check presence of proxy protocol
                var connectionReader *bufio.Reader;
                var clientIp, proxyIp, clientPort, proxyPort, data = func() (string, string, int, int, []byte) {
                    var err error;
                    connectionReader = bufio.NewReader(connection);
                    var connectionReaderBufferCount int;
                    var connectionReaderBuffer []byte;

                    var data []byte;
                    data = []byte("");

                    // Check proxy protocol header
                    const proxyProtocolHeaderString string = "PROXY ";
                    const proxyProtocolHeaderStringLen int = len(proxyProtocolHeaderString);
                    // TODO: FIXME: this could lead to bugs depending on the initial data burst
                    //connectionReaderBuffer = make([]byte, proxyProtocolHeaderStringLen);
                    connectionReaderBuffer = make([]byte, 1024);
                    // Read initial header
                    connectionReaderBufferCount, err = connectionReader.Read(connectionReaderBuffer);
                    // Check header buffer is valid, count matches expected length and buffer match expected content
                    if(err != nil || connectionReaderBufferCount != proxyProtocolHeaderStringLen ||
                            !bytes.Equal(connectionReaderBuffer, []byte(proxyProtocolHeaderString))) {
                        log.Printf("Error parsing proxy protocol header prefix: %s. Error: %v", connectionReaderBuffer, err);
                        data = connectionReaderBuffer;
                        return "", "", 0, 0, data;
                    }

                    // Check case of unknown proxy protocol
                    const proxyProtocolUnknownString string = "UNKNOWN\r\n";
                    var proxyProtocolUnknownStringLen int = len(proxyProtocolUnknownString);
                    connectionReaderBuffer, err = connectionReader.Peek(proxyProtocolUnknownStringLen);
                    // Check unknwon buffer is valid and data matches expected content
                    if(err != nil || bytes.Equal(connectionReaderBuffer, []byte(proxyProtocolUnknownString))) {
                        log.Printf("Error parsing proxy protocol unknown: %v", err);
                        return "", "", 0, 0, data;
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
                            log.Printf("Error parsing proxy protocol inet protocol: %s. Error: %v", connectionReaderBuffer, err);
                            return "", "", 0, 0, data;
                        }

                    // Read client IP address
                    var proxyProtocolClientIpString string;
                    proxyProtocolClientIpString, err = connectionReader.ReadString(' ');
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol client IP: %v", err);
                        return "", "", 0, 0, data;
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
                        return "", "", 0, 0, data;
                    }

                    // Read proxy IP address
                    var proxyProtocolProxyIpString string;
                    proxyProtocolProxyIpString, err = connectionReader.ReadString(' ');
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol proxy IP: %v", err);
                        return "", "", 0, 0, data;
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
                        return "", "", 0, 0, data;
                    }

                    // Read client port number
                    var proxyProtocolClientPortString string;
                    proxyProtocolClientPortString, err = connectionReader.ReadString(' ');
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol client port: %v", err);
                        return "", "", 0, 0, data;
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
                        return "", "", 0, 0, data;
                    }

                    // Read proxy port number
                    var proxyProtocolProxyPortString string;
                    proxyProtocolProxyPortString, err = connectionReader.ReadString('\r');
                    if(err != nil) {
                        log.Printf("Error parsing proxy protocol proxy port: %v", err);
                        return "", "", 0, 0, data;
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
                        return "", "", 0, 0, data;
                    }

                    // Read trailing characters
                    var proxyProtocolTrailingByte byte;
                    proxyProtocolTrailingByte, err = connectionReader.ReadByte();
                    if(err != nil || proxyProtocolTrailingByte != '\n') {
                        log.Printf("Error parsing proxy protocol trailing byte: %v", err);
                        return "", "", 0, 0, data;
                    }

                    return proxyProtocolClientIpString, proxyProtocolProxyIpString, proxyProtocolClientPort, proxyProtocolProxyPort, data;
                } ();

                log.Printf("[host] Iterating over hosts configuration: %v", hostsConfiguration);
                for ip, port := range hostsConfiguration {
                    signalNext := make(chan struct{});
                    atomic.StoreInt32(&signalNextMutex, 0);


                    atomic.AddInt32(&hostsConfigurationCounter, 1);
                    if(atomic.LoadInt32(&foundValidHost) == 1) {
                        break;
                    }
                    log.Printf("[host] Trying to connect to host: %s:%d", ip, port);
                    // Transform: forward connection to handler
                    mode:= networkMode;
                    address:= ip;
                    log.Printf("[host] Handling remote connection: %s\n", connection.RemoteAddr());

                    // Pass the connection to handler
                    if(connection != nil) {
                        var hostConnection net.Conn;
                        var err error;

                        // Try to connect to host
                        var currentHostPort = port;
                        var host = address + ":" + strconv.Itoa(currentHostPort);

                        // TODO: FIXME: expose timeout
                        var hostConnectionTimeout time.Duration;
                        // TODO: FIXME: error checking
                        hostConnectionTimeout, _ = time.ParseDuration("1s");
                        hostConnection, err = net.DialTimeout(mode, host, hostConnectionTimeout);
                        if(err == nil) {
                            log.Printf("[host] Connected to %s", host);

                            // Handle internal header communication
                            log.Printf("[host] Reading back proxy protocol line. inet: tcp | Remote clientip: %s, clientport %d | Proxy proxyip: %s, proxyport: %d\n", clientIp, clientPort, proxyIp, proxyPort);
                            var proxyLine string;
                            if(proxyPort == 0) {
                                proxyLine = fmt.Sprintf("PROXY TCP4 127.0.0.1 127.0.0.1 %d %d\r\n%s", currentClusterPort, currentClusterPort, data);
                            } else {
                                proxyLine = fmt.Sprintf("PROXY TCP4 %s %s %d %d\r\n", clientIp, proxyIp, clientPort, proxyPort);
                            }
                            log.Printf("[host] sending header line: %s", proxyLine);
                            fmt.Fprintf(hostConnection, proxyLine);

                            // TODO: FIXME: Remove
                            fmt.Fprintf(connection, ""); // FLUSH
                            // TODO: FIXME: Remove
                            fmt.Fprintf(hostConnection, ""); // FLUSH

                            var isConnected int32;
                            atomic.StoreInt32(&isConnected, 1);


                            // Input: Send data from received connection to host
                            go func(conn net.Conn) {
                                defer func() {
                                    log.Printf("[host] Closing input host connection...");
                                    if(atomic.LoadInt32(&isConnected) == 1) {
                                        atomic.StoreInt32(&isConnected, 0);
                                        log.Printf("[host] Closing host connection: %s", hostConnection.RemoteAddr());
                                        // TODO: FIXME: verify if the right place to call this. Toggle ?
                                        hostConnection.Close();
                                        //conn.Close();
                                        if(atomic.LoadInt32(&signalNextMutex) == 0) {
                                            atomic.StoreInt32(&signalNextMutex, 1);
                                            log.Printf("[host] SIGNAL: next entry");
                                            close(signalNext);
                                        }

                                        if(atomic.LoadInt32(&signalDoneMutex) == 0) {
                                            atomic.StoreInt32(&signalDoneMutex, 1);
                                            log.Printf("[host] SIGNAL: hosts loop is done");
                                            close(signalDone);
                                        }
                                    }
                                } ();

                                var err error;
                                clientConnectionWriter := ClientWriter{conn};
                                signalConnectionReader := SignalReader{hostConnection};
                                log.Printf("[host] Copying to connection %s and host %s", conn.RemoteAddr(), hostConnection.RemoteAddr());
                                // TODO: FIXME: Remove
                                fmt.Fprintf(conn, ""); // FLUSH
                                // TODO: FIXME: Remove
                                fmt.Fprintf(hostConnection, ""); // FLUSH

                                _, err = io.Copy(clientConnectionWriter, signalConnectionReader);
                                if(err != nil) {
                                    log.Printf("[host] Error copying data from cluster to host: %v", err);
                                    /*if(atomic.LoadInt32(&foundValidHost) == 1) {
                                        fmt.Fprintf(conn, "debug1");
                                    }*/

                                    // TODO: FIXME: Missing call. Leak !? Toggle ?
                                    //hostConnection.Close();
/*                                        if(atomic.LoadInt32(&signalDoneMutex) == 0) {
                                            atomic.StoreInt32(&signalDoneMutex, 1);
                                            close(signalDone);
                                        }*/

                                    return;
                                }
                            }(connection);

                            // Output: send data from host back to the original connection
                            go func(conn net.Conn) {
                                defer func() {
                                    log.Printf("[host] Closing output host connection...");
                                    if(atomic.LoadInt32(&isConnected) == 1) {
                                        atomic.StoreInt32(&isConnected, 0);
                                        log.Printf("[host] Closing host connection: %s", hostConnection.RemoteAddr());
                                        // TODO: FIXME: verify if the right place to call this. Toggle ?
                                        hostConnection.Close();
                                        //conn.Close();
                                        if(atomic.LoadInt32(&signalNextMutex) == 0) {
                                            atomic.StoreInt32(&signalNextMutex, 1);
                                            log.Printf("[host] SIGNAL: next entry");
                                            close(signalNext);
                                        }

                                        if(atomic.LoadInt32(&signalDoneMutex) == 0) {
                                            atomic.StoreInt32(&signalDoneMutex, 1);
                                            log.Printf("[host] SIGNAL: hosts loop is done");
                                            close(signalDone);
                                        }
                                    }
                                } ();

                                var err error;

                                // TODO: FIXME: Remove
                                fmt.Fprintf(conn, ""); // FLUSH
                                // TODO: FIXME: Remove
                                fmt.Fprintf(hostConnection, ""); // FLUSH
                                log.Printf("[host] Copying to hostConnection %s and conn %s", hostConnection.RemoteAddr(), conn.RemoteAddr());
                                _, err = io.Copy(hostConnection, connectionReader);
                                if(err != nil) {
                                    /*if(atomic.LoadInt32(&foundValidHost) == 1) {
                                        fmt.Fprintf(conn, "debug2");
                                    }*/
                                    log.Printf("[host] Error copying data from host to cluster: %v", err);
                                    // TODO: FIXME: verify if the right place to call this. Toggle ?
                                    //hostConnection.Close();
                                    /*if(atomic.LoadInt32(&signalDoneMutex) == 0) {
                                        atomic.StoreInt32(&signalDoneMutex, 1);
                                        close(signalDone);
                                    }*/

                                    return;
                                }
                            }(connection);
                        } else {
                            log.Printf("[host] Error connecting to %s in mode %s. Message: %v", host, mode, err);
                            //err = connection.Close();
                            if(err != nil) {
                                log.Printf("[host] Error closing connection: %s. Error: %v", connection.RemoteAddr(), err);
                            }
                            if(atomic.LoadInt32(&signalNextMutex) == 0) {
                                log.Printf("[host] Proceeding to next entry");
                                atomic.StoreInt32(&signalNextMutex, 1);
                                close(signalNext);
                            }

                            var currentHostsConfigurationCounter int32;
                            currentHostsConfigurationCounter = atomic.LoadInt32(&hostsConfigurationCounter);
                            if(currentHostsConfigurationCounter >= hostsConfigurationLen) {
                                log.Printf("No available hosts");
                                connection.Close();
                                if(atomic.LoadInt32(&signalDoneMutex) == 0) {
                                    atomic.StoreInt32(&signalDoneMutex, 1);
                                    log.Printf("[host] SIGNAL: hosts loop is done");
                                    close(signalDone);
                                }
                            }
                        }
                    } else {
                        log.Printf("[host] Hosts routine for %s, over and out!\n", listener.Addr());
                        //err = connection.Close();
                        if(err != nil) {
                            log.Printf("[host] Error closing connection: %s. Error: %v", connection.RemoteAddr(), err);
                        }
                        if(atomic.LoadInt32(&signalNextMutex) == 0) {
                            atomic.StoreInt32(&signalNextMutex, 1);
                            log.Printf("[host] SIGNAL: next entry");
                            close(signalNext);
                        }
                        return;
                    }
                    log.Printf("[host] Waiting on signal next. Current host: %v, %v", ip, port);
                    <-signalNext;
                    log.Printf("[host] Proceeding to the next host");
                }
                log.Printf("[host] Waiting on signal done");
                <-signalDone;
                log.Printf("[host] Exhausted all hosts");
                connection.Close();
            }
        } (listener, clusterPort);
    }

    // Handle listener connections
    go func(listener net.Listener) {
        // Transform: forward all connections to handler
        for {
            // Take a new connection
            var connection net.Conn;
            var err error;
            log.Printf("Ready for new connection");
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

                // Check presence of proxy protocol
                var connectionReader *bufio.Reader;
                var clientIp, proxyIp, clientPort, proxyPort = func() (string, string, int, int) {
                    var err error;
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
                        log.Printf("Error parsing proxy protocol inet protocol: %s. Error: %v", connectionReaderBuffer, err);
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
                    err = connection.Close();
                    if(err != nil) {
                        log.Printf("Error closing connection: %s. Error: %v", connection.RemoteAddr(), err);
                    }
                    return;
                } else {
                    if(newPortsConfiguration[proxyPort] != nil) {

                        var hostPorts []PortsConfigurationData;
                        hostPorts = newPortsConfiguration[proxyPort];

                        // Pass the connection to handler
                        if(connection != nil) {
                            go func(mode string, address string, ports []PortsConfigurationData, conn net.Conn) {
                                var hostConnection net.Conn;
                                var err error;

                                // Iterate over all host ports trying to connect to host
                                var hostPortsIndex int;
                                var hostPortsLen = len(ports);
                                var attempts int = 0;
                                log.Printf("Current number of configured host ports: %d", hostPortsLen);
                                for hostPortsIndex=0; hostPortsIndex < hostPortsLen; hostPortsIndex++ {
                                    var currentHostPort = ports[hostPortsIndex].hostPort;
                                    var host = address + ":" + strconv.Itoa(currentHostPort);

                                    // Limit max connections
                                    var currentHostMaxConnections = ports[hostPortsIndex].maxConnections;
                                    if(currentHostPortsMaxConnections[currentHostPort] >= currentHostMaxConnections) {
                                        log.Printf("Error connecting to %s in mode %s. Reached maximum number of active connections (%d)", host, mode, currentHostMaxConnections);
                                        if(hostPortsIndex == (hostPortsLen-1)) {
                                            fmt.Fprintf(connection, responseMappingInactive);
                                            log.Printf("Closing connection: %s", connection.RemoteAddr());
                                            err = connection.Close();
                                            if(err != nil) {
                                                log.Printf("Error closing connection: %s. Error: %v", connection.RemoteAddr(), err);
                                            }
                                            return;
                                        } else {
                                            attempts++;
                                            continue;
                                        }
                                    }

                                    hostConnection, err = net.Dial(mode, host);
                                    if(err == nil) {
                                        fmt.Fprintf(connection, responseMappingActive);
                                        log.Printf("Connected to %s", host);

                                        var currentSendProxyFlag = ports[hostPortsIndex].sendProxyFlag;
                                        if(currentSendProxyFlag) {
                                            var proxyLine = "PROXY TCP4 " + clientIp + " " + proxyIp + " " + strconv.Itoa(clientPort) + " " + strconv.Itoa(proxyPort) + "\r\n";
                                            fmt.Fprintf(hostConnection, proxyLine);
                                            log.Printf("sendProxy is set");
                                        }

                                        currentHostPortsMaxConnections[currentHostPort]++; // TODO: FIXME: CMPXCHG
                                        log.Printf("Current connections on port %d: %d (%d)", currentHostPort, currentHostPortsMaxConnections[currentHostPort], currentHostMaxConnections);
                                        var isConnected = true; // TODO: FIXME: atomic

                                        // Input: Send data from received connection to host
                                        go func() {
                                            defer func() {
                                                log.Printf("Closing input host connection...");
                                                if(isConnected) { // TODO: FIXME: atomic
                                                    currentHostPortsMaxConnections[currentHostPort]--;
                                                    isConnected = false;
                                                }
                                            } ();

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
                                            defer func() {
                                                log.Printf("Closing output host connection...");
                                                if(isConnected) { // TODO: FIXME: atomic
                                                    currentHostPortsMaxConnections[currentHostPort]--;
                                                    isConnected = false;
                                                }
                                            } ();

                                            var err error;
                                            log.Printf("Copying to hostConnection %s and conn %s", hostConnection.RemoteAddr(), conn.RemoteAddr());
                                            _, err = io.Copy(hostConnection, connectionReader);
                                            if(err != nil) {
                                                log.Printf("Error copying data from host to cluster: %v", err);
                                                hostConnection.Close();
                                                return;
                                            }
                                        }();

                                        // End host ports loop
                                        break;
                                    } else {
                                        attempts++;
                                        log.Printf("Error connecting to %s in mode %s. Message: %v", host, mode, err);
                                        if(attempts >= hostPortsLen) {
                                            fmt.Fprintf(connection, responseMappingInactive);
                                            log.Printf("Closing connection: %s", connection.RemoteAddr());
                                            err = connection.Close();
                                            if(err != nil) {
                                                log.Printf("Error closing connection: %s. Error: %v", connection.RemoteAddr(), err);
                                            }
                                            return;
                                        }
                                    }
                                }
                            } (networkMode, hostAddress, hostPorts, connection);
                        } else {
                            log.Printf("Ports routine for %s, over and out!\n", listener.Addr());
                            err = connection.Close();
                            if(err != nil) {
                                log.Printf("Error closing connection: %s. Error: %v", connection.RemoteAddr(), err);
                            }
                            return;
                        }
                    } else {
                        fmt.Fprintf(connection, responseMappingInactive);
                        log.Printf("Closing connection: %s", connection.RemoteAddr());
                        err = connection.Close();
                        if(err != nil) {
                            log.Printf("Error closing connection: %s. Error: %v", connection.RemoteAddr(), err);
                        }
                        return;
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
                var configuration = loadPortsConfiguration(portsConfigurationFile);
                if(configuration != nil) {
                    log.Printf("Ports configuration: %v\n", newPortsConfiguration);
                    // TODO: FIXME: newPortsConfiguration should drop all connections that were removed in the reload process (diff)
                    newPortsConfiguration = configuration;
                }
            }
        }
    } ();

    var watchChannelHosts chan bool;
    go func() {
        var fileHasChanged bool;
        for {
            watchChannelHosts = make(chan bool);
            go watchForFileChanges(hostsConfigurationFile, watchChannelHosts);
            fileHasChanged = <-watchChannelHosts;
            if(fileHasChanged) {
                log.Printf("Hosts configuration file has changed: %s. Reloading...\n", hostsConfigurationFile);
                var configuration = loadHostsConfiguration(hostsConfigurationFile);
                if(configuration != nil) {
                    log.Printf("Hosts configuration: %v\n", hostsConfiguration);
                    // TODO: FIXME: hostsConfiguration should drop all connections that were removed in the reload process (diff)
                    hostsConfiguration = configuration;
                }
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

    // Block indefinitely
    select {
    }
}
