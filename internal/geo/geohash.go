// Package geo encodes coordinates as geohashes (M5). A geohash interleaves
// longitude and latitude bits and writes them in base32, so nearby points
// usually share a prefix and a prefix query finds a rectangular cell.
package geo

const base32 = "0123456789bcdefghjkmnpqrstuvwxyz"

// Encode returns the geohash of a point with the given number of
// characters. Each character adds 5 bits: 6 characters is a cell of about
// 1.2 km x 0.6 km at the equator, 7 is about 150 m x 150 m.
func Encode(lat, lng float64, precision int) string {
	latLo, latHi := -90.0, 90.0
	lngLo, lngHi := -180.0, 180.0
	out := make([]byte, 0, precision)
	bits, ch := 0, 0
	evenBit := true // even bits (counting from 0) encode longitude
	for len(out) < precision {
		if evenBit {
			mid := (lngLo + lngHi) / 2
			if lng >= mid {
				ch = ch<<1 | 1
				lngLo = mid
			} else {
				ch <<= 1
				lngHi = mid
			}
		} else {
			mid := (latLo + latHi) / 2
			if lat >= mid {
				ch = ch<<1 | 1
				latLo = mid
			} else {
				ch <<= 1
				latHi = mid
			}
		}
		evenBit = !evenBit
		if bits++; bits == 5 {
			out = append(out, base32[ch])
			bits, ch = 0, 0
		}
	}
	return string(out)
}
