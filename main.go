package main

import (
	"github.com/fkautz/casserole/cmd"
	"log"
)

func main() {
	if err := cmd.RootCmd.Execute(); err != nil {
		log.Panicln(err)
	}
}
