package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
)

func renderIcon(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx := float64(size) / 2.0
	cy := float64(size) / 2.0
	fSize := float64(size)

	// Squircle corner radius
	cornerR := fSize * 0.22

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx := float64(x) + 0.5
			fy := float64(y) + 0.5

			// Check rounded rectangle bounds
			dx := math.Max(0, math.Abs(fx-cx)-(cx-cornerR))
			dy := math.Max(0, math.Abs(fy-cy)-(cy-cornerR))
			distCorner := math.Sqrt(dx*dx + dy*dy)

			if distCorner > cornerR {
				// Outside squircle: transparent
				img.SetRGBA(x, y, color.RGBA{0, 0, 0, 0})
				continue
			}

			// Distance from center
			distFromCenter := math.Hypot(fx-cx, fy-cy)
			normDist := distFromCenter / (fSize / 2.0)

			// Base dark background: #0c0e14 -> #141824 gradient
			bgGrad := 14.0 + 10.0*(1.0-normDist)
			r := uint8(math.Min(255, bgGrad*0.9))
			g := uint8(math.Min(255, bgGrad))
			b := uint8(math.Min(255, bgGrad*1.4))

			// Outer neon cyan-blue ring: radius ~ 0.68 of half-size
			r1 := 0.68 * (fSize / 2.0)
			d1 := math.Abs(distFromCenter - r1)
			w1 := fSize * 0.045
			if d1 < w1 {
				alpha := math.Cos((d1 / w1) * (math.Pi / 2.0))
				r = uint8(math.Min(255, float64(r)+0.0*alpha))
				g = uint8(math.Min(255, float64(g)+113.0*alpha))
				b = uint8(math.Min(255, float64(b)+227.0*alpha))
			}

			// Mid glowing blue ring: radius ~ 0.44 of half-size
			r2 := 0.44 * (fSize / 2.0)
			d2 := math.Abs(distFromCenter - r2)
			w2 := fSize * 0.04
			if d2 < w2 {
				alpha := math.Cos((d2 / w2) * (math.Pi / 2.0))
				r = uint8(math.Min(255, float64(r)+41.0*alpha))
				g = uint8(math.Min(255, float64(g)+151.0*alpha))
				b = uint8(math.Min(255, float64(b)+255.0*alpha))
			}

			// Core pulse center: radius ~ 0.22 of half-size
			r3 := 0.22 * (fSize / 2.0)
			if distFromCenter < r3 {
				alpha := math.Cos((distFromCenter / r3) * (math.Pi / 2.0))
				r = uint8(math.Min(255, float64(r)+48.0*alpha))
				g = uint8(math.Min(255, float64(g)+209.0*alpha))
				b = uint8(math.Min(255, float64(b)+88.0*alpha))
			}

			// Border highlight around squircle edge
			if distCorner > cornerR-1.5 {
				edgeAlpha := (distCorner - (cornerR - 1.5)) / 1.5
				r = uint8(float64(r)*(1.0-edgeAlpha) + 60.0*edgeAlpha)
				g = uint8(float64(g)*(1.0-edgeAlpha) + 70.0*edgeAlpha)
				b = uint8(float64(b)*(1.0-edgeAlpha) + 90.0*edgeAlpha)
			}

			img.SetRGBA(x, y, color.RGBA{r, g, b, 255})
		}
	}
	return img
}

func main() {
	sizes := []int{192, 512}
	for _, sz := range sizes {
		img := renderIcon(sz)
		fname := "modules/dashboard/icon-192.png"
		if sz == 512 {
			fname = "modules/dashboard/icon-512.png"
		}
		f, err := os.Create(fname)
		if err != nil {
			panic(err)
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			panic(err)
		}
		f.Close()
	}

	svgContent := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
  <defs>
    <linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0%" stop-color="#141824"/>
      <stop offset="100%" stop-color="#0c0e14"/>
    </linearGradient>
    <radialGradient id="coreGlow" cx="50%" cy="50%" r="50%">
      <stop offset="0%" stop-color="#30d158" stop-opacity="0.9"/>
      <stop offset="60%" stop-color="#2997ff" stop-opacity="0.4"/>
      <stop offset="100%" stop-color="#0071e3" stop-opacity="0"/>
    </radialGradient>
    <filter id="glow" x="-20%" y="-20%" width="140%" height="140%">
      <feGaussianBlur stdDeviation="8" result="blur"/>
      <feMerge>
        <feMergeNode in="blur"/>
        <feMergeNode in="SourceGraphic"/>
      </feMerge>
    </filter>
  </defs>
  <rect width="512" height="512" rx="112" fill="url(#bg)"/>
  <rect width="508" height="508" x="2" y="2" rx="110" fill="none" stroke="#2c2c2e" stroke-width="2"/>
  <circle cx="256" cy="256" r="160" fill="none" stroke="#0071e3" stroke-width="8" stroke-dasharray="14 10" opacity="0.6"/>
  <circle cx="256" cy="256" r="110" fill="none" stroke="#2997ff" stroke-width="10" filter="url(#glow)" opacity="0.9"/>
  <circle cx="256" cy="256" r="60" fill="url(#coreGlow)"/>
  <circle cx="256" cy="256" r="32" fill="#30d158" filter="url(#glow)"/>
</svg>`
	if err := os.WriteFile("modules/dashboard/icon.svg", []byte(svgContent), 0644); err != nil {
		panic(err)
	}
}
