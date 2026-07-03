package ui

import (
	"fmt"
	"os"
	"os/exec"
)

// Fingerprint authentication is wired into PAM through two dedicated service
// files. Keeping them separate from the primary "fin" / "display_manager"
// services means password login is never made to wait on a fingerprint swipe:
// the greeter and screensaver only consult the *-fingerprint service when the
// user actively chooses to swipe.
//
//   - fin-fingerprint          used by the fin login greeter (opens a session)
//   - display_manager-fingerprint  used by the screensaver unlock (auth only)
const (
	pamFinFingerprint = "/etc/pam.d/fin-fingerprint"
	pamDMFingerprint  = "/etc/pam.d/display_manager-fingerprint"
)

const pamFinFingerprintContent = `#%PAM-1.0

auth       required     pam_fprintd.so
account    include      system-local-login
session    include      system-local-login
password   include      system-local-login
`

const pamDMFingerprintContent = `#%PAM-1.0

auth       required     pam_fprintd.so
account    include      system-local-login
`

// fingerprintLoginEnabled reports whether the fingerprint PAM services are in
// place, i.e. whether login / unlock will accept a fingerprint.
func fingerprintLoginEnabled() bool {
	_, err1 := os.Stat(pamFinFingerprint)
	_, err2 := os.Stat(pamDMFingerprint)
	return err1 == nil && err2 == nil
}

// pamFprintdAvailable reports whether the pam_fprintd.so module is installed.
// Enabling fingerprint login is pointless without it, so the UI warns first.
func pamFprintdAvailable() bool {
	for _, dir := range []string{
		"/usr/lib/security", "/usr/lib64/security",
		"/lib/security", "/lib/x86_64-linux-gnu/security",
		"/usr/lib/x86_64-linux-gnu/security",
	} {
		if _, err := os.Stat(dir + "/pam_fprintd.so"); err == nil {
			return true
		}
	}
	return false
}

// setFingerprintLogin enables or disables fingerprint login/unlock by writing or
// removing the dedicated PAM service files. It elevates through pkexec, so the
// user is prompted for authorisation; the call blocks until pkexec exits and
// therefore must not run on the render thread.
func setFingerprintLogin(enable bool) error {
	var script string
	if enable {
		script = fmt.Sprintf(`set -e
cat > %s <<'EOF'
%sEOF
cat > %s <<'EOF'
%sEOF
chmod 644 %s %s`,
			pamFinFingerprint, pamFinFingerprintContent,
			pamDMFingerprint, pamDMFingerprintContent,
			pamFinFingerprint, pamDMFingerprint)
	} else {
		script = fmt.Sprintf("rm -f %s %s", pamFinFingerprint, pamDMFingerprint)
	}

	cmd := exec.Command("pkexec", "/bin/sh", "-c", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if len(out) > 0 {
			return fmt.Errorf("%s", out)
		}
		return err
	}
	return nil
}
