package routers

import (
	"github.com/kerberos-io/opensource/machinery/src/routers/http"
)

func StartWebserver(name string, port string){
	http.StartServer(name, port)
}
