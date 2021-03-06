package main

import (
	_ "expvar" // For HTTP server registration
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"time"

	"github.com/golang/glog"
	"github.com/google/trillian"
	"github.com/google/trillian/examples/ct"
	"google.golang.org/grpc"
)

// Global flags that affect all log instances.
var serverPortFlag = flag.Int("port", 6962, "Port to serve CT log requests on")
var rpcBackendFlag = flag.String("log_rpc_server", "localhost:8090", "Backend Log RPC server to use")
var rpcDeadlineFlag = flag.Duration("rpc_deadline", time.Second*10, "Deadline for backend RPC requests")
var logConfigFlag = flag.String("log_config", "", "File holding log config in JSON")

func awaitSignal() {
	// Arrange notification for the standard set of signals used to terminate a server
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Now block main and wait for a signal
	sig := <-sigs
	glog.Warningf("Signal received: %v", sig)
	glog.Flush()

	// Terminate the process
	os.Exit(1)
}

func main() {
	flag.Parse()
	// Get log config from file before we start.
	cfg, err := ct.LogConfigFromFile(*logConfigFlag)
	if err != nil {
		glog.Fatalf("Failed to read log config: %v", err)
	}

	glog.CopyStandardLogTo("WARNING")
	glog.Info("**** CT HTTP Server Starting ****")

	// TODO(Martin2112): Support TLS and other stuff for RPC client and http server, this is just to
	// get started. Uses a blocking connection so we don't start serving before we're connected
	// to backend.
	conn, err := grpc.Dial(*rpcBackendFlag, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		glog.Fatalf("Could not connect to rpc server: %v", err)
	}
	defer conn.Close()
	client := trillian.NewTrillianLogClient(conn)

	for _, c := range cfg {
		if err := c.SetUpInstance(client, *rpcDeadlineFlag); err != nil {
			glog.Fatalf("Failed to set up log instance for %+v: %v", cfg, err)
		}
	}

	// Bring up the HTTP server and serve until we get a signal not to.
	go awaitSignal()
	server := http.Server{Addr: fmt.Sprintf("localhost:%d", *serverPortFlag), Handler: nil}
	err = server.ListenAndServe()
	glog.Warningf("Server exited: %v", err)
	glog.Flush()
}
