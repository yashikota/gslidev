package google

import (
	"google.golang.org/api/slides/v1"
	"testing"
)

func TestPageSizeNormalizesUnitsForImagePlacement(t *testing.T) {
	for _, unit := range []string{"EMU", "PT"} {
		t.Run(unit, func(t *testing.T) {
			factor := 1.0
			if unit == "EMU" {
				factor = 12700
			}
			presentation := &slides.Presentation{PageSize: &slides.Size{
				Width:  &slides.Dimension{Magnitude: 720 * factor, Unit: unit},
				Height: &slides.Dimension{Magnitude: 405 * factor, Unit: unit},
			}}
			width, height := pageSize(presentation)
			image := imageRequest("page", "https://example.com/image.png", 0, 0, width, height).CreateImage.ElementProperties
			if image.Size.Width.Unit != "PT" || image.Size.Height.Unit != "PT" || image.Size.Width.Magnitude != 720 || image.Size.Height.Magnitude != 405 {
				t.Fatalf("image must fill a 720×405 point page, got %+v", image.Size)
			}
			gif := imageRequest("page", "https://example.com/animation.gif", .1*width, .2*height, .3*width, .4*height).CreateImage.ElementProperties
			if gif.Transform.TranslateX != 72 || gif.Transform.TranslateY != 81 || gif.Size.Width.Magnitude != 216 || gif.Size.Height.Magnitude != 162 {
				t.Fatalf("relative GIF placement must use the same unit: %+v", gif)
			}
		})
	}
}
