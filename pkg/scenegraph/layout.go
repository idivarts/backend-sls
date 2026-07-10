package scenegraph

// Box is a pixel-space rectangle.
type Box struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// NormBox is a rectangle in normalized [0,1] frame coordinates. This is the
// hit-box the frontend overlays on the rendered PNG so users can tap an element
// to edit its text or pin a comment — no client-side scene engine required.
type NormBox struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// LayoutBox is a placed element.
type LayoutBox struct {
	ID   string      `json:"id"`
	Type ElementType `json:"type"`
	Zone Zone        `json:"zone"`
	Px   Box         `json:"px"`
	Norm NormBox     `json:"norm"`
}

// Layout is the computed placement of every element in a document, in source
// order. Callers persist Layout.Boxes alongside the rendered image so the app
// can build tap regions.
type Layout struct {
	Size  Size        `json:"size"`
	Boxes []LayoutBox `json:"boxes"`
}

const (
	marginFrac  = 0.06 // frame margin as a fraction of each axis
	logoMaxFrac = 0.18 // logo max size as a fraction of frame width
	elemGapFrac = 0.02 // vertical gap between stacked elements in a zone
)

// ComputeLayout deterministically maps a document's semantic zones to pixel
// boxes. Elements never carry x/y; this is the single place geometry is
// derived, so preview, hit-boxes and render all agree.
func ComputeLayout(d *Document) Layout {
	l := Layout{Size: d.Size}
	if d.Size.W <= 0 || d.Size.H <= 0 {
		return l
	}
	fw, fh := float64(d.Size.W), float64(d.Size.H)
	mx, my := marginFrac*fw, marginFrac*fh
	contentTop, contentBottom := my, fh-my
	contentH := contentBottom - contentTop

	// Group elements by zone, preserving order.
	byZone := map[Zone][]int{}
	var order []Zone
	for i, el := range d.Elements {
		z := el.Zone
		if z == "" {
			z = ZoneCenter
		}
		if _, ok := byZone[z]; !ok {
			order = append(order, z)
		}
		byZone[z] = append(byZone[z], i)
	}

	band := func(z Zone) (top, height float64) {
		switch z {
		case ZoneFull:
			return 0, fh
		case ZoneTop:
			return contentTop, contentH / 3
		case ZoneCenter:
			return contentTop + contentH/3, contentH / 3
		case ZoneBottom:
			return contentTop + 2*contentH/3, contentH / 3
		default:
			return contentTop + contentH/3, contentH / 3
		}
	}

	for _, z := range order {
		idxs := byZone[z]
		top, bandH := band(z)
		n := float64(len(idxs))
		gap := elemGapFrac * fh
		slice := (bandH - gap*(n-1)) / n
		for k, ei := range idxs {
			el := d.Elements[ei]
			y := top + float64(k)*(slice+gap)
			bx := Box{X: mx, Y: y, W: fw - 2*mx, H: slice}

			if el.Type == ElLogo {
				// Logos are small squares, aligned by HAlign, pinned to the
				// top of their slice.
				size := logoMaxFrac * fw
				if size > slice {
					size = slice
				}
				bx = Box{X: alignX(el.HAlign, mx, fw-mx, size), Y: y, W: size, H: size}
			} else if z == ZoneFull && el.Type == ElImage {
				bx = Box{X: 0, Y: 0, W: fw, H: fh}
			}

			l.Boxes = append(l.Boxes, LayoutBox{
				ID:   el.ID,
				Type: el.Type,
				Zone: z,
				Px:   bx,
				Norm: NormBox{X: bx.X / fw, Y: bx.Y / fh, W: bx.W / fw, H: bx.H / fh},
			})
		}
	}
	return l
}

// alignX returns the left x for a box of the given width between left and right
// bounds, honouring the horizontal alignment hint.
func alignX(hAlign string, left, right, w float64) float64 {
	switch hAlign {
	case "right":
		return right - w
	case "center":
		return (left + right - w) / 2
	default: // left / ""
		return left
	}
}

// BoxFor returns the placed box for an element id, or false.
func (l Layout) BoxFor(id string) (LayoutBox, bool) {
	for _, b := range l.Boxes {
		if b.ID == id {
			return b, true
		}
	}
	return LayoutBox{}, false
}
