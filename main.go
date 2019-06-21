package main

import (
	"fmt"
	"os"
	"strconv"

	eirinix "github.com/SUSE/eirinix"
)

func main() {
	var port int32
	ns := os.Getenv("NAMESPACE")
	if len(ns) == 0 {
		ns = "default"
	}
	host := os.Getenv("HOST")
	if len(host) == 0 {
		host = "10.0.2.2"
	}
	p := os.Getenv("PORT")
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
