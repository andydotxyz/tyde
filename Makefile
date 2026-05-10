# If PREFIX isn't provided, we check for /usr/local and use that if it exists.
# Otherwice we fall back to using /usr.

LOCAL != test -d $(DESTDIR)/usr/local && echo -n "/local" || echo -n ""
LOCAL ?= $(shell test -d $(DESTDIR)/usr/local && echo "/local" || echo "")
PREFIX ?= /usr$(LOCAL)

build:
	go build ./cmd/tyde_runner
	go build ./cmd/tyde_ctl
	go build ./cmd/tyde

install:
	install -Dm00755 tyde_runner $(DESTDIR)$(PREFIX)/bin/tyde_runner
	install -Dm00755 tyde_runner $(DESTDIR)$(PREFIX)/bin/tyde_ctl
	install -Dm00755 tyde $(DESTDIR)$(PREFIX)/bin/tyde
	install -Dm00644 tyde.desktop $(DESTDIR)$(PREFIX)/share/xsessions/tyde.desktop

uninstall:
	-rm $(DESTDIR)$(PREFIX)/bin/tyde_runner
	-rm $(DESTDIR)$(PREFIX)/bin/tyde_ctl
	-rm $(DESTDIR)$(PREFIX)/bin/tyde
	-rm $(DESTDIR)$(PREFIX)/share/xsessions/tyde.desktop

embed:
	Xephyr :5 -screen 1280x720 &
	DISPLAY=:5 go run -tags migrated_fynedo ./cmd/tyde
