package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/hashicorp/consul/command/flags"
	"github.com/hashicorp/consul/logger"
	"github.com/mitchellh/cli"
)

func main() {
	// Handle parsing the CLI flags.
	var configFiles flags.AppendSliceValue

	f := flag.NewFlagSet("", flag.ContinueOnError)
	f.Var(&configFiles, "config-file", "A config file to use. Can be either .hcl or .json "+
		"format. Can be specified multiple times.")
	f.Var(&configFiles, "config-dir", "A directory to look for .hcl or .json config files in. "+
		"Can be specified multiple times.")

	f.Usage = func() {
		fmt.Print(flags.Usage(usage, f))
	}

	err := f.Parse(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Parse and merge the config.
	config := DefaultConfig()
	err = MergeConfigPaths(config, []string(configFiles))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Set up logging.
	logConfig := &logger.Config{
		LogLevel:       config.LogLevel,
		EnableSyslog:   config.EnableSyslog,
		SyslogFacility: config.SyslogFacility,
	}
	ui := &cli.BasicUi{Writer: os.Stdout, ErrorWriter: os.Stderr}
	_, gatedWriter, _, logOutput, ok := logger.Setup(logConfig, ui)
	if !ok {
		os.Exit(1)
	}
	logger := log.New(logOutput, "", log.LstdFlags)

	a, err := NewAgent(config, logger)
	if err != nil {
		panic(err)
	}

	// Set up shutdown and signal handling.
	shutdownCh := make(chan struct{})
	signalCh := make(chan os.Signal, 10)
	signal.Notify(signalCh)
	go handleSignals(a.logger, signalCh, shutdownCh)

	ui.Output("Consul ESM running!")
	if config.Datacenter == "" {
		ui.Info(fmt.Sprintf("            Datacenter: (default)"))
	} else {
		ui.Info(fmt.Sprintf("            Datacenter: %q", config.Datacenter))
	}
	ui.Info(fmt.Sprintf("               Service: %q", config.Service))
	ui.Info(fmt.Sprintf("            Leader Key: %q", config.LeaderKey))
	ui.Info(fmt.Sprintf("Node Reconnect Timeout: %q", config.NodeReconnectTimeout.String()))

	ui.Info("")
	ui.Output("Log data will now stream in as it occurs:\n")
	gatedWriter.Flush()

	// Run the agent!
	if err := a.Run(shutdownCh); err != nil {
		panic(err)
	}
}

func handleSignals(logger *log.Logger, signalCh chan os.Signal, shutdownCh chan struct{}) {
	shutdown := false
	for sig := range signalCh {
		switch sig {
		case os.Interrupt:
			logger.Printf("[INFO] got signal, shutting down...")
			if !shutdown {
				close(shutdownCh)
				shutdown = true
			}
		}
	}
}

const usage = `
Usage: consul-esm [options]

  A config file is optional, and can be either HCL or JSON format.
`
