package rpc

import (
	"errors"
	"fmt"
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
	Name:        "RPC",
	NewInstance: newRPC,
}

// SocketPath returns the Unix socket path used for RPC communication.
func SocketPath() string {
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir != "" {
		return filepath.Join(dir, "fynedesk.sock")
	}
	return "/tmp/fynedesk-" + strconv.Itoa(os.Getuid()) + ".sock"
}

type rpcModule struct {
	listener net.Listener
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
type Service struct{}

// Launch passes the input string through the launch suggestion modules
// and executes the first match.
func (s *Service) Launch(input string, reply *string) error {
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

func newRPC() tyde.Module {
	sock := SocketPath()
	os.Remove(sock) // clean up stale socket

	srv := rpc.NewServer()
	srv.Register(&Service{})

	ln, err := net.Listen("unix", sock)
	if err != nil {
		fyne.LogError("RPC: failed to listen on "+sock, err)
		return &rpcModule{}
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

	return &rpcModule{listener: ln}
}
