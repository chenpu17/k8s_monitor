package ui

import (
	"sort"
)

// renderDetailContent renders the content of the current detail view.
func (m *Model) renderDetailContent() string {
	switch m.currentView {
	case ViewNodeDetail:
		return m.renderNodeDetail()
	case ViewPodDetail:
		return m.renderPodDetail()
	case ViewEventDetail:
		return m.renderEventDetail()
	case ViewJobDetail:
		return m.renderJobDetail()
	case ViewServiceDetail:
		return m.renderServiceDetail()
	case ViewDeploymentDetail:
		return m.renderDeploymentDetail()
	case ViewStatefulSetDetail:
		return m.renderStatefulSetDetail()
	case ViewDaemonSetDetail:
		return m.renderDaemonSetDetail()
	case ViewCronJobDetail:
		return m.renderCronJobDetail()
	case ViewPVDetail:
		return m.renderPVDetail()
	case ViewPVCDetail:
		return m.renderPVCDetail()
	case ViewVolcanoJobDetail:
		return m.renderVolcanoJobDetail()
	case ViewQueueDetail:
		return m.renderQueueDetail()
	case ViewTopologyDetail:
		return m.renderSuperPodDetail()
	default:
		return ""
	}
}

// renderDetailAt renders the content of the current detail view at the given
// scroll offset, leaving the model's own offset untouched.
func (m *Model) renderDetailAt(offset int) string {
	saved := m.detailScrollOffset
	m.detailScrollOffset = offset
	content := m.renderDetailContent()
	m.detailScrollOffset = saved
	return content
}

// sortedKeys returns the keys of a map in a stable order. Detail renderers that
// walk a map directly produce a different line order on every render (Go
// randomises map iteration), which makes the labels/annotations block flicker on
// every refresh and leaves the scroll clamp without a deterministic view to
// compare against.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// detailViewMoves reports whether the detail renderer produces different content
// for the given offset and the next one, i.e. whether the view still travels
// between the two. The renderers clamp the offset they are handed to their own
// maximum, so the bottom page and everything past it render the same window and
// this returns false from the bottom page onwards.
//
// The two windows are compared as rendered, line by line. That is sound only
// because a renderer builds the same window for the same offset every time: the
// renderers that display maps sort the keys they walk (sortedKeys), and
// TestDetailScrollMapOrderedContent re-renders every offset of those views to
// pin that determinism. An offset past the end would otherwise look like
// movement, which is what would let the scroll offset run away past the last
// line.
func (m *Model) detailViewMoves(offset int) bool {
	if offset < 0 {
		return false
	}
	return m.renderDetailAt(offset) != m.renderDetailAt(offset+1)
}

// detailOffsetValid reports whether the detail renderer honours the given
// offset, i.e. whether that offset is a scroll position inside the content. The
// offset is valid when the view travelled on the way to it (or when it is the
// first line). detailViewMoves alone cannot be used as the validity test: it is
// false at the bottom page itself (the renderer clamps the next offset back onto
// the same window), which would put the bottom page - and with it the last line
// of the content - out of reach.
func (m *Model) detailOffsetValid(offset int) bool {
	if offset <= 0 {
		return true
	}
	return m.detailViewMoves(offset - 1)
}

// detailScrollTarget returns the largest valid offset in [from, to], where from
// is an offset known to be valid. Valid offsets form a prefix of the range, so
// the boundary can be found by bisection.
func (m *Model) detailScrollTarget(from, to int) int {
	if from < 0 {
		from = 0
	}
	if to <= from {
		return from
	}
	if m.detailOffsetValid(to) {
		return to
	}

	// The content ends somewhere between from and to; find where.
	lo, hi := from, to
	for lo+1 < hi {
		mid := lo + (hi-lo)/2
		if m.detailOffsetValid(mid) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return lo
}

// detailScrollLimit returns the offset the detail renderer actually displays for
// the given offset. The model's offset can sit past the end of the content when
// the content shrinks underneath it (a refresh can shorten a detail view), and
// the renderer then shows the last page for any of those offsets.
func (m *Model) detailScrollLimit(offset int) int {
	if offset <= 0 {
		return 0
	}
	if m.detailOffsetValid(offset) {
		return offset // still inside the content
	}
	// The offset is past the end: the renderer shows the last page.
	return m.detailScrollTarget(0, offset)
}

// detailScrollBy moves the detail view scroll offset by delta lines without ever
// stepping past the end of the content. Scrolling down is bounded by probing the
// renderer for the last valid offset; scrolling up starts from the offset that is
// actually on screen, so a single ↑ always moves the view exactly one line - also
// after the offset had been left dangling past the end by a content refresh.
func (m *Model) detailScrollBy(delta int) {
	if delta < 0 {
		offset := m.detailScrollLimit(m.detailScrollOffset) + delta
		if offset < 0 {
			offset = 0
		}
		m.detailScrollOffset = offset
		return
	}
	// A dangling offset means the view already shows the last page, so scrolling
	// down is a no-op until ↑ normalizes the offset.
	m.detailScrollOffset = m.detailScrollTarget(m.detailScrollOffset, m.detailScrollOffset+delta)
}
