package imagex

import (
    "image"
    "image/color"
)

func IsImageEqual(img1, img2 image.Image) bool {
    if img1.Bounds() != img2.Bounds() {
        return false
    }
    rgba1, ok1 := img1.(*image.NRGBA)
    rgba2, ok2 := img2.(*image.NRGBA)
    if ok1 && ok2 {
        for i := 0; i < len(rgba1.Pix); i++ {
            if rgba1.Pix[i] != rgba2.Pix[i] {
                return false
            }
        }
        return true
    }

    model := color.NRGBAModel
    rect1 := img1.Bounds()
    offX, offY := rect1.Min.X, rect1.Min.Y
    w, h := rect1.Dx(), rect1.Dy()
    for y := 0; y < h; y++ {
        for x := 0; x < w; x++ {
            c1 := model.Convert(img1.At(offX+x, offY+y)).(color.NRGBA)
            c2 := model.Convert(img2.At(offX+x, offY+y)).(color.NRGBA)
            if c1 != c2 {
                return false
            }
        }
    }
    return true
}
