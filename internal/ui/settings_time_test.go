package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCoord(t *testing.T) {
	// degrees/minutes form
	lat, lon, ok := parseCoord("+4230+00131")
	assert.True(t, ok)
	assert.InDelta(t, 42.5, lat, 0.01)
	assert.InDelta(t, 1.516, lon, 0.01)

	// degrees/minutes/seconds form, negative longitude
	lat, lon, ok = parseCoord("+404251-0740023")
	assert.True(t, ok)
	assert.InDelta(t, 40.714, lat, 0.01)
	assert.InDelta(t, -74.006, lon, 0.01)

	// southern hemisphere
	lat, _, ok = parseCoord("-3352+15113")
	assert.True(t, ok)
	assert.InDelta(t, -33.866, lat, 0.01)

	_, _, ok = parseCoord("")
	assert.False(t, ok)
}

func TestLoadZones(t *testing.T) {
	zones := loadZones()
	if len(zones) == 0 {
		t.Skip("no system zone table available")
	}
	byName := map[string]zoneInfo{}
	for _, z := range zones {
		byName[z.name] = z
	}
	berlin, ok := byName["Europe/Berlin"]
	assert.True(t, ok)
	assert.InDelta(t, 52.5, berlin.lat, 0.2)
	assert.InDelta(t, 13.4, berlin.lon, 0.2)
}
