package rpc

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"

	"fyshos.com/tyde"
)

// LaunchInput passes the input string through each LaunchSuggestionModule
// and executes the first match. Search module is ignored in this context.
func LaunchInput(input string) (string, error) {
	desk := tyde.Instance()
	if desk == nil {
		return "", errors.New("desktop not running")
	}

	for _, m := range desk.Modules() {
		suggest, ok := m.(tyde.LaunchSuggestionModule)
		if !ok {
			continue
		}
		items := suggest.LaunchSuggestions(input)
		if len(items) == 0 {
			continue
		}

		// Search module is a catch-all fallbacks which we don't want.
		if strings.Contains(strings.ToLower(m.Metadata().Name), "search") {
			continue
		}

		item := items[0]
		done := make(chan struct{})
		fyne.Do(func() {
			item.Launch()
			close(done)
		})
		<-done
		return item.Title(), nil
	}

	return "", fmt.Errorf("no match for %q", input)
}

var rpcMeta = tyde.ModuleMetadata{
	Name:        "Remote Control",
	NewInstance: newRPC,
}

// SocketPath returns the Unix socket path used for RPC communication.
func SocketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir != "" {
		return filepath.Join(dir, "tyde.sock")
	}
	return "/tmp/tyde-" + strconv.Itoa(os.Getuid()) + ".sock"
}

type rpcModule struct {
	listener net.Listener
	commands map[string]func() error
}

func (r *rpcModule) Metadata() tyde.ModuleMetadata {
	return rpcMeta
}

func (r *rpcModule) Destroy() {
	if r.listener != nil {
		r.listener.Close()
	}
	os.Remove(SocketPath())
}

// Service is the RPC service exposed over the Unix socket.
type Service struct {
	module *rpcModule
}

// Launch passes the input string through the launch suggestion modules
// and executes the first match.
func (s *Service) Launch(input string, reply *string) error {
	if s.module != nil {
		if v, ok := s.module.commands[strings.ToLower(input)]; ok {
			return v()
		}
	}
	title, err := LaunchInput(input)
	if err != nil {
		return err
	}
	*reply = title
	return nil
}

// ListModules returns the names of loaded LaunchSuggestionModules.
func (s *Service) ListModules(_ string, reply *[]string) error {
	desk := tyde.Instance()
	if desk == nil {
		return errors.New("desktop not running")
	}

	var names []string
	for _, m := range desk.Modules() {
		if _, ok := m.(tyde.LaunchSuggestionModule); !ok {
			continue
		}
		name := m.Metadata().Name
		name = strings.TrimPrefix(name, "Launcher: ")
		names = append(names, name)
	}
	*reply = names
	return nil
}

// moduleAction runs a command that an optional module provides.
// The loaded modules are asked in turn and the first one offering the
// action performs it - or the user is told which module they need to enable.
func moduleAction(what string, find func(tyde.Module) func()) error {
	desk := tyde.Instance()
	if desk == nil {
		return errors.New("desktop not running")
	}

	for _, m := range desk.Modules() {
		if action := find(m); action != nil {
			fyne.Do(action)
			return nil
		}
	}
	return fmt.Errorf("the %s module is not enabled", what)
}

// showAI opens the AI assistant's chat window with no prompt set - what the
// "Fathom" launcher does.
func showAI() error {
	return moduleAction("AI assistant", func(m tyde.Module) func() {
		if chat, ok := m.(interface{ ShowChat() }); ok {
			return chat.ShowChat
		}
		return nil
	})
}

// showEmoji opens the emoji picker - what the "Emoji Picker" launcher does.
func showEmoji() error {
	return moduleAction("emoji picker", func(m tyde.Module) func() {
		if picker, ok := m.(interface{ ShowPicker() }); ok {
			return picker.ShowPicker
		}
		return nil
	})
}

func newRPC() tyde.Module {
	sock := SocketPath()
	os.Remove(sock) // clean up stale socket

	mod := &rpcModule{commands: map[string]func() error{
		"welcome": func() error {
			if welcomer, ok := tyde.Instance().(interface{ ShowWelcome() }); ok {
				fyne.Do(welcomer.ShowWelcome)
			} else {
				return errors.New("desktop does not support welcome")
			}
			return nil
		},
		"fathom": showAI,
		"emoji":  showEmoji,
		"restart": func() error {
			if os.Getenv("FYNE_DESK_RUNNER") != "" {
				os.Exit(5)
				return nil
			} else {
				return errors.New("tyde was not launched with tyde_runner, cannot restart")
			}
		},
	}}
	srv := rpc.NewServer()
	srv.Register(&Service{module: mod})

	ln, err := net.Listen("unix", sock)
	if err != nil {
		fyne.LogError("RPC: failed to listen on "+sock, err)
		return &rpcModule{}
	} else {
		log.Println("RPC: Listening on " + sock)
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go srv.ServeConn(conn)
		}
	}()

	mod.listener = ln
	return mod
}
