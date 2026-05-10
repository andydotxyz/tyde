package main

import (
	"fmt"
	"net/rpc"
	"os"
	"strings"

	frpc "fyshos.com/tyde/modules/rpc"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] == "-h" || os.Args[1] == "--help" || os.Args[1] == "help" {
		printHelp()
		return
	}

	client, err := rpc.Dial("unix", frpc.SocketPath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to connect to fynedesk:", err)
		os.Exit(1)
	}
	defer client.Close()

	if os.Args[1] == "list" {
		var modules []string
		if err := client.Call("Service.ListModules", "", &modules); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("Available suggestion modules:")
		for _, m := range modules {
			fmt.Println("  -", m)
		}
		return
	}

	input := strings.Join(os.Args[1:], " ")
	var reply string
	if err := client.Call("Service.Launch", input, &reply); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(reply)
}

func printHelp() {
	fmt.Print(`fynedesk_ctl - command line interface for FyneDesk

Usage:
  fynedesk_ctl <command> [args...]
  fynedesk_ctl list
  fynedesk_ctl help

Commands are passed to FyneDesk's launch suggestion modules, the same
way text typed into the app launcher is processed.

Examples:
  fynedesk_ctl brightness up       Increase screen brightness
  fynedesk_ctl brightness 50       Set brightness to 50%
  fynedesk_ctl big Hello World     Show "Hello World" in large type
  fynedesk_ctl 2+2                 Evaluate expression

Built-in commands:
  list    Show loaded suggestion modules
  help    Show this help message
`)

	client, err := rpc.Dial("unix", frpc.SocketPath())
	if err != nil {
		return
	}
	defer client.Close()

	var modules []string
	if err := client.Call("Service.ListModules", "", &modules); err != nil {
		return
	}
	fmt.Println("Loaded modules:")
	for _, m := range modules {
		fmt.Println("  -", m)
	}
}
