package ui

import (
	"errors"
	"fmt"
	"os/user"
	"time"

	"github.com/godbus/dbus/v5"
)

// This file is a small client for fprintd (net.reactivated.Fprint), the system
// fingerprint daemon. It is used by the Account settings tab to let the user
// enrol and manage the fingerprints used to unlock the screensaver and log in.
//
// Everything here talks to the system bus and is called off the render thread.

const (
	fprintService    = "net.reactivated.Fprint"
	fprintManagerObj = "/net/reactivated/Fprint/Manager"
	fprintManagerIf  = "net.reactivated.Fprint.Manager"
	fprintDeviceIf   = "net.reactivated.Fprint.Device"
)

// fingerNames are the fingers fprintd understands, in the order we present them.
var fingerNames = []string{
	"right-index-finger", "right-thumb", "right-middle-finger",
	"right-ring-finger", "right-little-finger",
	"left-index-finger", "left-thumb", "left-middle-finger",
	"left-ring-finger", "left-little-finger",
}

// fingerLabel gives a human-friendly name for an fprintd finger identifier.
func fingerLabel(finger string) string {
	switch finger {
	case "right-index-finger":
		return "Right index finger"
	case "right-thumb":
		return "Right thumb"
	case "right-middle-finger":
		return "Right middle finger"
	case "right-ring-finger":
		return "Right ring finger"
	case "right-little-finger":
		return "Right little finger"
	case "left-index-finger":
		return "Left index finger"
	case "left-thumb":
		return "Left thumb"
	case "left-middle-finger":
		return "Left middle finger"
	case "left-ring-finger":
		return "Left ring finger"
	case "left-little-finger":
		return "Left little finger"
	}
	return finger
}

// fprintClient wraps a system-bus connection and the default fingerprint device.
type fprintClient struct {
	conn   *dbus.Conn
	device dbus.BusObject
	path   dbus.ObjectPath
}

// newFprintClient connects to fprintd and resolves the default sensor. It
// returns an error (without logging) when no daemon or no device is present, so
// callers can quietly hide the fingerprint UI on machines without a sensor.
func newFprintClient() (*fprintClient, error) {
	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}

	mgr := conn.Object(fprintService, fprintManagerObj)
	var devicePath dbus.ObjectPath
	if err := mgr.Call(fprintManagerIf+".GetDefaultDevice", 0).Store(&devicePath); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &fprintClient{
		conn:   conn,
		device: conn.Object(fprintService, devicePath),
		path:   devicePath,
	}, nil
}

// close releases the bus connection. The caller must not reuse the client after.
func (c *fprintClient) close() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// currentUsername resolves the current user's login name for fprintd calls.
func currentUsername() string {
	if u, err := user.Current(); err == nil {
		return u.Username
	}
	return ""
}

// numEnrollStages reports how many scans this device needs to enrol a finger,
// used to render enrollment progress. It falls back to a sensible default.
func (c *fprintClient) numEnrollStages() int {
	v, err := c.device.GetProperty(fprintDeviceIf + ".num-enroll-stages")
	if err != nil {
		return 5
	}
	if n, ok := v.Value().(int32); ok && n > 0 {
		return int(n)
	}
	return 5
}

// listEnrolled returns the fingers already enrolled for the current user.
func (c *fprintClient) listEnrolled() ([]string, error) {
	var fingers []string
	err := c.device.Call(fprintDeviceIf+".ListEnrolledFingers", 0, currentUsername()).Store(&fingers)
	if err != nil {
		// fprintd raises an error rather than returning an empty list when a
		// user has no prints; treat that as simply "none enrolled".
		return nil, err
	}
	return fingers, nil
}

// deleteEnrolled removes every enrolled finger for the current user.
func (c *fprintClient) deleteEnrolled() error {
	user := currentUsername()
	if call := c.device.Call(fprintDeviceIf+".Claim", 0, user); call.Err != nil {
		return call.Err
	}
	defer c.device.Call(fprintDeviceIf+".Release", 0)

	return c.device.Call(fprintDeviceIf+".DeleteEnrolledFingers", 0, user).Err
}

// enroll drives an interactive enrollment of the given finger. progress is
// called (off the UI thread) after each scan with a human-readable status and
// the fraction complete in [0,1]. It blocks until enrollment completes, fails,
// or the timeout elapses.
func (c *fprintClient) enroll(finger string, progress func(msg string, done float64)) error {
	user := currentUsername()
	if call := c.device.Call(fprintDeviceIf+".Claim", 0, user); call.Err != nil {
		return fmt.Errorf("could not access the sensor: %w", call.Err)
	}
	defer c.device.Call(fprintDeviceIf+".Release", 0)

	// Subscribe to EnrollStatus before starting so no early scan is missed.
	if err := c.conn.AddMatchSignal(
		dbus.WithMatchObjectPath(c.path),
		dbus.WithMatchInterface(fprintDeviceIf),
		dbus.WithMatchMember("EnrollStatus"),
	); err != nil {
		return err
	}
	defer c.conn.RemoveMatchSignal(
		dbus.WithMatchObjectPath(c.path),
		dbus.WithMatchInterface(fprintDeviceIf),
		dbus.WithMatchMember("EnrollStatus"),
	)

	signals := make(chan *dbus.Signal, 16)
	c.conn.Signal(signals)
	defer c.conn.RemoveSignal(signals)

	if call := c.device.Call(fprintDeviceIf+".EnrollStart", 0, finger); call.Err != nil {
		return fmt.Errorf("could not start enrollment: %w", call.Err)
	}
	defer c.device.Call(fprintDeviceIf+".EnrollStop", 0)

	stages := c.numEnrollStages()
	passed := 0
	timeout := time.NewTimer(2 * time.Minute)
	defer timeout.Stop()

	for {
		select {
		case sig := <-signals:
			if sig == nil || sig.Name != fprintDeviceIf+".EnrollStatus" || len(sig.Body) < 2 {
				continue
			}
			result, _ := sig.Body[0].(string)
			done, _ := sig.Body[1].(bool)

			switch result {
			case "enroll-completed":
				progress("Fingerprint saved.", 1)
				return nil
			case "enroll-failed":
				return errors.New("enrollment failed, please try again")
			case "enroll-stage-passed":
				passed++
				frac := float64(passed) / float64(stages)
				if frac > 1 {
					frac = 1
				}
				progress("Scan received, lift and touch again...", frac)
			default:
				// Retry hints such as enroll-retry-scan / enroll-swipe-too-short.
				progress(enrollHint(result), float64(passed)/float64(stages))
			}
			if done && result != "enroll-completed" {
				return errors.New(enrollHint(result))
			}
		case <-timeout.C:
			return errors.New("enrollment timed out")
		}
	}
}

// enrollHint turns an fprintd retry/failure code into a user-facing message.
func enrollHint(result string) string {
	switch result {
	case "enroll-retry-scan":
		return "Scan was not clear, please try again..."
	case "enroll-swipe-too-short":
		return "Swipe was too short, please try again..."
	case "enroll-finger-not-centered":
		return "Center your finger on the sensor and try again..."
	case "enroll-remove-and-retry":
		return "Remove your finger and touch the sensor again..."
	case "enroll-data-full":
		return "The sensor storage is full."
	case "enroll-disconnected":
		return "The fingerprint sensor was disconnected."
	}
	return "Please try again..."
}
