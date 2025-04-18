package main

import (
	"log"
	"net/http"
	"os"

	_ "net/http/pprof"

	_ "go.uber.org/automaxprocs"

	"github.com/GoPlugin/web3rpcproxy/internal/app"
)

func main() {
	if os.Getenv("PPROF") == "true" {
		go func() {
			log.Println("listen and serve pprof: http://0.0.0.0:6060")
			log.Println(http.ListenAndServe("0.0.0.0:6060", nil))
		}()
	}

	app.StartCluster()
}
