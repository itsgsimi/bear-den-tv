// Generated DEMO posters for the fake Plex Media Server's photo transcoder:
// a colour gradient per item, a simple hill silhouette, and the word DEMO
// with the item number in a built-in 5×7 bitmap font. Never real artwork.

package plexfake

import (
	"bytes"
	"crypto/sha256"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"strconv"
	"strings"
)

// glyphs is a 5×7 bitmap font for the characters posters use.
var glyphs = map[rune][7]string{
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'O': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'#': {"01010", "11111", "01010", "01010", "01010", "11111", "01010"},
	'0': {"01110", "10011", "10101", "10101", "10101", "11001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"01110", "10001", "00001", "00110", "01000", "10000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	'5': {"11111", "10000", "11110", "00001", "00001", "10001", "01110"},
	'6': {"00110", "01000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00010", "01100"},
}

// Poster renders the DEMO poster for ratingKey at w×h as PNG bytes.
func Poster(ratingKey string, w, h int) []byte {
	if w <= 0 || w > 1000 {
		w = 240
	}
	if h <= 0 || h > 1500 {
		h = 360
	}
	seed := sha256.Sum256([]byte("plexfake-poster\x00" + ratingKey))
	top := color.RGBA{40 + seed[0]%120, 40 + seed[1]%120, 60 + seed[2]%140, 255}
	bottom := color.RGBA{top.R / 4, top.G / 4, top.B / 3, 255}
	hill := color.RGBA{top.R / 2, top.G/2 + 20, top.B / 3, 255}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	phase := float64(seed[3]) / 255
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		row := color.RGBA{lerp(top.R, bottom.R, t), lerp(top.G, bottom.G, t), lerp(top.B, bottom.B, t), 255}
		for x := 0; x < w; x++ {
			fx := float64(x) / float64(w)
			ridge := 0.62 + 0.08*tri(fx*2+phase)
			if t > ridge {
				img.SetRGBA(x, y, hill)
				continue
			}
			img.SetRGBA(x, y, row)
		}
	}
	// A pale sun.
	cx, cy, r := w*(30+int(seed[4]%40))/100, h/4, w/9
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r && x >= 0 && y >= 0 && x < w && y < h {
				img.SetRGBA(x, y, color.RGBA{250, 236, 200, 255})
			}
		}
	}
	white := color.RGBA{255, 255, 255, 255}
	label := "DEMO"
	scale := w / 30
	if scale < 1 {
		scale = 1
	}
	drawText(img, label, (w-textWidth(label, scale))/2, h*2/5, scale, white)
	num := "#" + digits(ratingKey)
	small := scale * 2 / 3
	if small < 1 {
		small = 1
	}
	drawText(img, num, (w-textWidth(num, small))/2, h*2/5+9*scale, small, white)
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func (f *Fake) poster(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	thumb := q.Get("url")
	if !strings.HasPrefix(thumb, "/library/metadata/") {
		http.NotFound(w, r)
		return
	}
	key := strings.SplitN(strings.TrimPrefix(thumb, "/library/metadata/"), "/", 2)[0]
	width, _ := strconv.Atoi(q.Get("width"))
	height, _ := strconv.Atoi(q.Get("height"))
	// Keep the poster 2:3 inside the requested box, like the transcoder.
	if width > 0 && height > 0 && width*3 > height*2 {
		width = height * 2 / 3
	} else if width > 0 {
		height = width * 3 / 2
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(Poster(key, width, height))
}

func digits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func textWidth(s string, scale int) int { return len([]rune(s))*6*scale - scale }

func drawText(img *image.RGBA, s string, x0, y0, scale int, c color.RGBA) {
	for i, r := range s {
		g, ok := glyphs[r]
		if !ok {
			continue
		}
		for gy, row := range g {
			for gx, bit := range row {
				if bit != '1' {
					continue
				}
				for dy := 0; dy < scale; dy++ {
					for dx := 0; dx < scale; dx++ {
						x := x0 + (i*6+gx)*scale + dx
						y := y0 + gy*scale + dy
						if image.Pt(x, y).In(img.Rect) {
							img.SetRGBA(x, y, c)
						}
					}
				}
			}
		}
	}
}

func lerp(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }

// tri is a triangle wave in [-1, 1] with period 1.
func tri(x float64) float64 {
	x -= float64(int(x))
	if x < 0.5 {
		return 4*x - 1
	}
	return 3 - 4*x
}
