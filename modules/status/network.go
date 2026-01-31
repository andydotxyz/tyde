package status

import (
	"errors"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"fyshos.com/fynedesk"
	wmtheme "fyshos.com/fynedesk/theme"
)

var networkMeta = fynedesk.ModuleMetadata{
	Name:        "Network",
	NewInstance: NewNetwork,
}

const networkNameEthernet = "Ethernet"

type network struct {
	name *widget.Label
	icon *widget.Button

	wasBlocked bool
}

func (n *network) Destroy() {
}

func (n *network) wirelessName() (string, error) {
	net := ""
	iw, _ := exec.LookPath("iw")
	if iw == "" {
		iw, _ = exec.LookPath("/usr/sbin/iw")
	}
	if iw != "" {
		out, err := exec.Command("bash", []string{"-c", iw + " dev | grep ssid | cut -d ' ' -f2"}...).Output()
		if err != nil {
			log.Println("Error running iw", err)
			return "", err
		}
		net = strings.TrimSpace(string(out))
		if net == "" {
			return "", errors.New("no network connected")
		}
	} else {
		out, err := exec.Command("bash", []string{"-c", "/System/Library/PrivateFrameworks/Apple80211.framework/Resources/airport -I  | awk -F' SSID: '  '/ SSID: / {print $2}'"}...).Output()
		if err != nil {
			log.Println("Error getting network info from airport utility", err)
			return "", err
		}

		net = string(out)
	}
	return strings.TrimSpace(net), nil
}

func (n *network) isBlocked() (bool, error) {
	if ip, _ := exec.LookPath("rfkill"); ip != "" {
		out, err := exec.Command("bash", []string{"-c", "rfkill | grep \"wlan\""}...).Output()
		if err != nil {
			log.Println("Error running rfkill tool", err)
			return false, err
		}
		if strings.Contains(string(out), " blocked") {
			return true, nil
		}

		return false, nil
	}
	if ip, _ := exec.LookPath("networksetup"); ip != "" {
		out, err := exec.Command("bash", []string{"-c", "networksetup -setairportpower en0"}...).Output()
		if err != nil {
			log.Println("Error running networksetup tool", err)
			return false, err
		}
		if strings.TrimSpace(string(out)) == "Off" {
			return true, nil
		}

		return false, nil
	}

	return false, nil
}

func (n *network) isEthernetConnected() (bool, error) {
	if ip, _ := exec.LookPath("ip"); ip != "" {
		out, err := exec.Command("bash", []string{"-c", "ip link | grep \",UP,\" | grep -v LOOPBACK | grep -v \": wl\" | wc -l"}...).Output()
		if err != nil {
			log.Println("Error running ip tool", err)
			return false, err
		}
		if strings.TrimSpace(string(out)) == "0" {
			return false, nil
		}
	} else if scutil, _ := exec.LookPath("scutil"); scutil != "" {
		out, err := exec.Command("bash", []string{"-c", "scutil --nwi | grep address | wc -l"}...).Output()
		if err != nil {
			log.Println("Error running scutil tool", err)
			return false, err
		}
		if strings.TrimSpace(string(out)) == "0" {
			return false, nil
		}
	} else {
		out, err := exec.Command("ifconfig").Output()
		if err != nil {
			log.Println("Error running ifconfig tool", err)
			return false, err
		}
		re, err := regexp.Compile(`^[^\t:]+:([^\n]|\n\t)*status: active`)
		if err != nil {
			log.Println("Error compiling regular expression", err)
			return false, err
		}
		m := re.FindSubmatch(out)
		if len(m) < 1 {
			return false, nil
		}
		// IPv4
		if strings.Contains(string(m[0]), "broadcast") {
			return true, nil
		}
		// IPv6, non-link-local only
		if found, _ := regexp.MatchString(`\s+inet6\s+[[:xdigit:]:]+\s+prefixlen\s+`, m[0]); found {
			return true, nil
		}
	}
	return true, nil
}

func (n *network) networkName() string {
	name, _ := n.wirelessName()
	if name != "" {
		return name
	}

	ether, _ := n.isEthernetConnected()
	if ether {
		return networkNameEthernet
	}
	return ""
}

func (n *network) tick() {
	tick := time.NewTicker(time.Second * 10)
	go func() {
		for {
			fyne.Do(n.refreshContent)
			<-tick.C
		}
	}()
}

func (n *network) refreshContent() {
	val := n.networkName()
	blocked, _ := n.isBlocked()

	if val != n.name.Text || blocked != n.wasBlocked {
		n.wasBlocked = blocked
		n.name.SetText(val)

		if blocked {
			n.icon.SetIcon(wmtheme.AirplaneIcon)
		} else if val == "" {
			n.icon.SetIcon(wmtheme.WifiOffIcon)
		} else if val == networkNameEthernet {
			n.icon.SetIcon(wmtheme.EthernetIcon)
		} else {
			n.icon.SetIcon(wmtheme.WifiIcon)
		}
	}
}

func (n *network) StatusAreaWidget() fyne.CanvasObject {
	blocked := false
	if _, err := n.wirelessName(); err != nil {
		if blocked, err = n.isBlocked(); blocked || err != nil {
		}
		if _, err = n.isEthernetConnected(); err != nil && !blocked {
			return nil
		}
	}

	n.name = widget.NewLabel("")
	n.icon = &widget.Button{Icon: wmtheme.WifiOffIcon, Importance: widget.LowImportance, OnTapped: n.toggleFlightMode}
	if blocked {
		n.icon.Icon = wmtheme.AirplaneIcon
	}
	n.tick()

	return container.New(&handleNarrow{}, n.icon, n.name)
}

func (n *network) Metadata() fynedesk.ModuleMetadata {
	return networkMeta
}

func (n *network) setFlightMode(block bool) error {
	if ip, _ := exec.LookPath("rfkill"); ip != "" {
		out, err := exec.Command("bash", []string{"-c", "rfkill | grep \"wlan\""}...).Output()
		if err != nil {
			log.Println("Error running rfkill tool", err)
			return err
		}
		if len(out) < 3 {
			return errors.New("rfkill tool: rfkill output is too short")
		}

		id := strings.Split(strings.TrimSpace(string(out)), " ")[0]

		if id != "" {
			mode := "block"
			if !block {
				mode = "unblock"
			}
			cmd := exec.Command("bash", []string{"-c", "pkexec rfkill " + mode + " " + id}...)
			err = cmd.Start()
			if err != nil {
				log.Println("Error running rfkill tool", err)
				return err
			}

			go func() {
				cmd.Wait()
				fyne.Do(n.refreshContent)
			}()
		}

		return nil
	}

	if ip, _ := exec.LookPath("networksetup"); ip != "" {
		mode := "off"
		if !block {
			mode = "on"
		}
		err := exec.Command("bash", []string{"-c", "networksetup -setairportpower en0 " + mode + " "}...).Run()
		if err != nil {
			log.Println("Error running networksetup tool", err)
			return err
		}

		n.refreshContent()
	}

	return nil
}

func (n *network) toggleFlightMode() {
	blocked, err := n.isBlocked()
	if err != nil {
		fyne.LogError("blocking not supported", err)
		return
	}
	err = n.setFlightMode(!blocked)
	if err != nil {
		fyne.LogError("setting flight mode", err)
	}
}

// NewNetwork creates a new module that will show network information in the status area
func NewNetwork() fynedesk.Module {
	return &network{}
}
