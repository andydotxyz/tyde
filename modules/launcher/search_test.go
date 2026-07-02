package launcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearch_LaunchSuggestions_Empty(t *testing.T) {
	s := newSearchSuggest().(*search)

	assert.Nil(t, s.LaunchSuggestions(""))
}

func TestSearch_LaunchSuggestions(t *testing.T) {
	s := newSearchSuggest().(*search)

	res := s.LaunchSuggestions("fyne toolkit")
	if assert.Len(t, res, 1) {
		item := res[0].(*searchItem)
		assert.Equal(t, "fyne toolkit", item.text)
		assert.Equal(t, "Search Web: fyne toolkit", item.Title())
		assert.NotNil(t, item.Icon())
	}
}
