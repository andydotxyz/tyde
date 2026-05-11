package launcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnyts_IsConversion(t *testing.T) {
	u := newUnyts().(*unyts)

	assert.True(t, u.isConversion("100km in mi"))
	assert.True(t, u.isConversion("10kg to lb"))
	assert.True(t, u.isConversion("5m as ft"))
	assert.True(t, u.isConversion("1km IN mi"))

	assert.False(t, u.isConversion(""))
	assert.False(t, u.isConversion("100km"))
	assert.False(t, u.isConversion("100km in"))
	assert.False(t, u.isConversion("100km into mi"))
	assert.False(t, u.isConversion("100 km in mi"))
	assert.False(t, u.isConversion("notanumber in mi"))
}

func TestUnyts_LaunchSuggestions(t *testing.T) {
	u := newUnyts().(*unyts)

	assert.Nil(t, u.LaunchSuggestions(""))
	assert.Nil(t, u.LaunchSuggestions("hello world"))

	res := u.LaunchSuggestions("100km in mi")
	if assert.Len(t, res, 1) {
		item := res[0].(*unytResult)
		assert.Equal(t, "100km in mi", item.conversion)
	}
}

func TestUnytResult_Title(t *testing.T) {
	r := &unytResult{conversion: "100km in mi"}
	assert.Equal(t, "100km in mi = 62.1", r.Title())

	r = &unytResult{conversion: "10kg to lb"}
	assert.Equal(t, "10kg to lb = 22.0", r.Title())

	r = &unytResult{conversion: "100km in xyz"}
	assert.Equal(t, "Cannot convert km to xyz", r.Title())
}

func TestUnytResult_Icon(t *testing.T) {
	r := &unytResult{conversion: "100km in mi"}
	assert.Equal(t, resourceUnyts, r.Icon())
}
