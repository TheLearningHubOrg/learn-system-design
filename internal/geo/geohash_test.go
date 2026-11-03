package geo

import (
	"strings"
	"testing"
)

func TestEncodeKnownPoint(t *testing.T) {
	// A widely used reference point near Aalborg, Denmark.
	if got := Encode(57.64911, 10.40744, 11); got != "u4pruydqqvj" {
		t.Fatalf("got %s", got)
	}
}

func TestNearbyPointsSharePrefix(t *testing.T) {
	a := Encode(40.7580, -73.9855, 7) // two points about 100 m apart in Manhattan
	b := Encode(40.7585, -73.9845, 7)
	if !strings.HasPrefix(b, a[:5]) {
		t.Fatalf("%s and %s do not share a 5-character prefix", a, b)
	}
	if far := Encode(51.5007, -0.1246, 7); far[:2] == a[:2] {
		t.Fatalf("London %s shares a prefix with New York %s", far, a)
	}
}
