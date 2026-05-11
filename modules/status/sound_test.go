package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStartsWith(t *testing.T) {
	assert.True(t, startsWith("vol", "volume"))
	assert.True(t, startsWith("volume", "volume"))
	assert.True(t, startsWith("volume up", "volume"))

	assert.False(t, startsWith("", "volume"))
	assert.False(t, startsWith("voo", "volume"))
}

func TestVolItem_Title(t *testing.T) {
	i := &volItem{input: "50"}
	assert.Equal(t, "Volume 50%", i.Title())

	i = &volItem{input: "mute"}
	assert.Equal(t, "Mute volume", i.Title())

	i = &volItem{input: "unmute"}
	assert.Equal(t, "Unmute volume", i.Title())

	i = &volItem{input: "up"}
	assert.Equal(t, "Volume up", i.Title())

	i = &volItem{input: "down"}
	assert.Equal(t, "Volume down", i.Title())

	i = &volItem{input: ""}
	assert.Equal(t, "", i.Title())
}

func TestVolItem_Icon(t *testing.T) {
	i := &volItem{input: "50"}
	assert.NotNil(t, i.Icon())
}
