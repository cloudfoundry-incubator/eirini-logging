package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/SUSE/eirini-logging/loggregator"
	eirinix "github.com/SUSE/eirinix"
)

func startExtension() {
	var port int32
	ns := os.Getenv("EIRINI_EXTENSION_NAMESPACE")
	if len(ns) == 0 {
		ns = "default"
	}
	host := os.Getenv("EIRINI_EXTENSION_HOST")
	if len(host) == 0 {
		host = "10.0.2.2"
	}
	p := os.Getenv("EIRINI_EXTENSION_PORT")
	if len(p) == 0 {
		port = 3000
	} else {
		po, err := strconv.Atoi(p)
		if err != nil {
			panic(err)
		}
		port = int32(po)
	}

	fmt.Println("Listening on ", host, port)

	x := eirinix.NewManager(
		eirinix.ManagerOptions{
			Namespace:           ns,
			Host:                host,
			Port:                port,
			KubeConfig:          os.Getenv("KUBECONFIG"),
			FilterEiriniApps:    false,
			OperatorFingerprint: "eirini-app-logging",
		})

	x.AddExtension(&Extension{Namespace: ns})
	fmt.Println(x.Start())
}

func startLoggregator() {
	fmt.Println(loggregator.NewLoggregator().Run())
}

func main() {
	if len(os.Args) <= 1 {
		fmt.Println("Please specify a subcommand (either 'extension' or 'loggregator')")
		return
	}

	switch os.Args[1] {
	case "extension":
		startExtension()
	case "loggregator":
		startLoggregator()
	default:
		fmt.Println("Subcommand has to be either 'extension' or 'loggregator'")
	}
}
