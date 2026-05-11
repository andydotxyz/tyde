package status

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBrightItem_Title(t *testing.T) {
	i := &brightItem{input: "50"}
	assert.Equal(t, "Brightness 50%", i.Title())

	i = &brightItem{input: "down"}
	assert.Equal(t, "Brightness down", i.Title())

	i = &brightItem{input: "up"}
	assert.Equal(t, "Brightness up", i.Title())

	i = &brightItem{input: ""}
	assert.Equal(t, "", i.Title())
}

func TestBrightItem_Icon(t *testing.T) {
	i := &brightItem{input: "50"}
	assert.NotNil(t, i.Icon())
}
