package launcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalc_IsExpression(t *testing.T) {
	c := newCalcSuggest().(*calc)

	assert.True(t, c.isExpression("5*3"))
	assert.True(t, c.isExpression("5*(3+2)"))
	assert.True(t, c.isExpression("5x6"))
	assert.True(t, c.isExpression("5/5-1"))

	assert.False(t, c.isExpression("33"))
	assert.False(t, c.isExpression("5e"))
	assert.False(t, c.isExpression("2,1*4"))
	assert.False(t, c.isExpression("xterm"))
}

func TestCalc_Eval(t *testing.T) {
	c := newCalcSuggest().(*calc)

	res, err := c.eval("5*3")
	assert.NoError(t, err)
	assert.Equal(t, "15", res)

	res, err = c.eval("5x3")
	assert.NoError(t, err)
	assert.Equal(t, "15", res)

	res, err = c.eval("10/4")
	assert.NoError(t, err)
	assert.Equal(t, "2.5", res)

	_, err = c.eval("5*")
	assert.Error(t, err)
}

func TestCalc_LaunchSuggestions(t *testing.T) {
	c := newCalcSuggest().(*calc)

	assert.Nil(t, c.LaunchSuggestions("xterm"))
	assert.Nil(t, c.LaunchSuggestions("33"))

	res := c.LaunchSuggestions("5*3")
	if assert.Len(t, res, 1) {
		item := res[0].(*calcItem)
		assert.Equal(t, "5*3", item.sum)
		assert.Equal(t, "15", item.result)
		assert.Equal(t, "5*3 = 15", item.Title())
		assert.NotNil(t, item.Icon())
	}
}
