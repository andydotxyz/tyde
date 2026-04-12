package launcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLargeType_LaunchSuggestions_NoMatch(t *testing.T) {
	l := newLargeType().(*largeType)

	assert.Nil(t, l.LaunchSuggestions("hello world"))
	assert.Nil(t, l.LaunchSuggestions("xyz"))
}

func TestLargeType_LaunchSuggestions_AliasPrefix(t *testing.T) {
	l := newLargeType().(*largeType)

	for _, alias := range largeTypeAliases {
		res := l.LaunchSuggestions(alias)
		if assert.Len(t, res, 1) {
			item := res[0].(*largeTypeItem)
			assert.Equal(t, "", item.text)
		}
	}

	// partial prefix still matches
	res := l.LaunchSuggestions("lar")
	if assert.Len(t, res, 1) {
		assert.Equal(t, "", res[0].(*largeTypeItem).text)
	}
}

func TestLargeType_LaunchSuggestions_WithText(t *testing.T) {
	l := newLargeType().(*largeType)

	res := l.LaunchSuggestions("largetype Hello World")
	if assert.Len(t, res, 1) {
		item := res[0].(*largeTypeItem)
		assert.Equal(t, "Hello World", item.text)
		assert.Equal(t, "Large Type: Hello World", item.Title())
	}

	res = l.LaunchSuggestions("BIG Fyne")
	if assert.Len(t, res, 1) {
		assert.Equal(t, "Fyne", res[0].(*largeTypeItem).text)
	}
}
