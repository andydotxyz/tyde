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
		fmt.Fprintln(os.Stderr, "failed to connect to tyde:", err)
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
	fmt.Print(`tyde_ctl - command line interface for Tyde

Usage:
  tyde_ctl <command> [args...]
  tyde_ctl list
  tyde_ctl help

Commands are passed to Tyde's launch suggestion modules, the same
way text typed into the app launcher is processed.

Examples:
  tyde_ctl brightness up       Increase screen brightness
  tyde_ctl brightness 50       Set brightness to 50%
  tyde_ctl big Hello World     Show "Hello World" in large type
  tyde_ctl 2+2                 Evaluate expression

Built-in commands:
  list    Show loaded suggestion modules
  help    Show this help message
`)
}
