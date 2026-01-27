package main

import (
	"fmt"
	"log"
	"os"
	"tunnel_pls/internal/bootstrap"
	"tunnel_pls/internal/config"
	"tunnel_pls/internal/port"
	"tunnel_pls/internal/version"
)

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(version.GetVersion())
		os.Exit(0)
	}

	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Starting %s", version.GetVersion())

	conf, err := config.MustLoad()
	if err != nil {
		log.Fatalf("Config load error: %v", err)
	}

	boot, err := bootstrap.New(conf, port.New())
	if err != nil {
		log.Fatalf("Startup error: %v", err)
	}

	if err = boot.Run(); err != nil {
		log.Fatalf("Application error: %v", err)
	}
}
