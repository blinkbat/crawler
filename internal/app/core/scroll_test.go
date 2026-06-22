package core

import "testing"

func absF(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestScrollMaxOffset(t *testing.T) {
	cases := []struct {
		view, content, want float32
	}{
		{100, 80, 0},  // fits
		{100, 100, 0}, // exact fit
		{100, 250, 150},
	}
	for _, c := range cases {
		if got := ScrollMaxOffset(c.view, c.content); got != c.want {
			t.Errorf("ScrollMaxOffset(view=%v, content=%v) = %v, want %v", c.view, c.content, got, c.want)
		}
	}
}

func TestScrollThumbExtent(t *testing.T) {
	if got := ScrollThumbExtent(200, 100, 80, 24); got != 200 {
		t.Errorf("fits: got %v, want 200", got)
	}
	if got := ScrollThumbExtent(200, 100, 400, 24); got != 50 {
		t.Errorf("proportional: got %v, want 50", got)
	}
	if got := ScrollThumbExtent(200, 10, 4000, 24); got != 24 {
		t.Errorf("min floor: got %v, want 24", got)
	}
	if got := ScrollThumbExtent(0, 10, 4000, 24); got != 0 {
		t.Errorf("zero track: got %v, want 0", got)
	}
}

func TestScrollThumbPosEndpoints(t *testing.T) {
	track, view, content, min := float32(200), float32(100), float32(400), float32(24)
	maxOff := ScrollMaxOffset(view, content)
	travel := track - ScrollThumbExtent(track, view, content, min)

	if got := ScrollThumbPos(track, view, content, min, 0); got != 0 {
		t.Errorf("offset 0: got %v, want 0", got)
	}
	if got := ScrollThumbPos(track, view, content, min, maxOff); got != travel {
		t.Errorf("offset max: got %v, want %v", got, travel)
	}
	if got := ScrollThumbPos(track, view, content, min, maxOff/2); absF(got-travel/2) > 0.001 {
		t.Errorf("offset mid: got %v, want %v", got, travel/2)
	}
	if got := ScrollThumbPos(track, view, content, min, maxOff+999); got != travel {
		t.Errorf("over-range offset: got %v, want %v", got, travel)
	}
	// Content that fits parks the thumb at the start.
	if got := ScrollThumbPos(200, 100, 80, 24, 50); got != 0 {
		t.Errorf("fits: got %v, want 0", got)
	}
}

func TestScrollThumbRoundTrip(t *testing.T) {
	track, view, content, min := float32(180), float32(120), float32(640), float32(20)
	maxOff := ScrollMaxOffset(view, content)
	for _, off := range []float32{0, 37, 200, 519, maxOff} {
		pos := ScrollThumbPos(track, view, content, min, off)
		back := ScrollOffsetForThumbPos(track, view, content, min, pos)
		if absF(back-off) > 0.01 {
			t.Errorf("round trip offset %v -> pos %v -> %v (drift %v)", off, pos, back, absF(back-off))
		}
	}
	if got := ScrollOffsetForThumbPos(track, view, content, min, 99999); absF(got-maxOff) > 0.001 {
		t.Errorf("over-range thumb pos: got %v, want %v", got, maxOff)
	}
}

func TestClampPanAxis(t *testing.T) {
	const over = float32(48)

	// Content FITS (content 80, view 200 at 10): base 70, hi 130, lo 10.
	base := float32(70)
	if got := ClampPanAxis(0, base, 10, 200, 80, over); got != 0 {
		t.Errorf("fit centered: pan 0 should stay 0, got %v", got)
	}
	if got := ClampPanAxis(1000, base, 10, 200, 80, over); got != 60 {
		t.Errorf("fit shove right: got %v, want 60", got)
	}
	if got := ClampPanAxis(-1000, base, 10, 200, 80, over); got != -60 {
		t.Errorf("fit shove left: got %v, want -60", got)
	}

	// Content OVERFLOWS (content 400, view 200 at 10): base -90, hi 58, lo -238.
	base = -90
	if got := ClampPanAxis(0, base, 10, 200, 400, over); got != 0 {
		t.Errorf("overflow centered: pan 0 should stay 0, got %v", got)
	}
	if got := ClampPanAxis(1000, base, 10, 200, 400, over); got != 148 {
		t.Errorf("overflow shove right: got %v, want 148", got)
	}
	if got := ClampPanAxis(-1000, base, 10, 200, 400, over); got != -148 {
		t.Errorf("overflow shove left: got %v, want -148", got)
	}
}
