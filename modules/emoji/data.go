package emoji

import (
	"sort"
	"strings"
	"sync"
)

//go:generate go run ./gen

// Emoji is one pickable character together with the name searches match
// against.
type Emoji struct {
	Character string
	Name      string
	Group     string
}

// Group is a named run of emoji, presented as one page of the picker.
type Group struct {
	Name  string
	Items []Emoji
}

var (
	groupsOnce sync.Once
	groups     []Group
)

// Groups returns the emoji split into their groups. The generated table is
// already in the Unicode Consortium's presentation order and its groups are
// contiguous, so this only has to find the boundaries - nothing is sorted, and
// nothing should be: that order is hand-curated per subgroup and no code point
// comparison reproduces it.
func Groups() []Group {
	groupsOnce.Do(func() {
		for _, e := range emojiTable {
			if len(groups) == 0 || groups[len(groups)-1].Name != e.Group {
				groups = append(groups, Group{Name: e.Group})
			}
			last := &groups[len(groups)-1]
			last.Items = append(last.Items, e)
		}
	})
	return groups
}

// All returns every pickable emoji in group order - the corpus searches run
// over.
func All() []Emoji {
	return emojiTable
}

// Search returns the emoji whose name matches every whitespace-separated term of
// the query, so "cat face" narrows rather than widens. Results keep the table's
// order, with exact and prefix matches promoted so "cat" leads with the cat
// rather than its wry-smiling neighbours.
func Search(query string) []Emoji {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}

	type scored struct {
		emoji Emoji
		rank  int
		index int
	}
	var found []scored

	for i, e := range All() {
		name := strings.ToLower(e.Name)

		matched := true
		for _, term := range terms {
			if !strings.Contains(name, term) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		found = append(found, scored{emoji: e, rank: matchRank(name, terms), index: i})
	}

	sort.SliceStable(found, func(i, j int) bool {
		if found[i].rank != found[j].rank {
			return found[i].rank < found[j].rank
		}
		return found[i].index < found[j].index
	})

	out := make([]Emoji, len(found))
	for i, f := range found {
		out[i] = f.emoji
	}
	return out
}

// matchRank scores how directly a name answers the query - lower is better. An
// exact name wins, then a name starting with the query, then anything else.
func matchRank(name string, terms []string) int {
	joined := strings.Join(terms, " ")
	switch {
	case name == joined:
		return 0
	case strings.HasPrefix(name, joined):
		return 1
	default:
		return 2
	}
}
