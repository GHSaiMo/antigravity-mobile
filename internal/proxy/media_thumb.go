package proxy

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	_ "image/gif" // register decoders for image.Decode
	"image/jpeg"
	_ "image/png"
	"strconv"
	"sync"
)

// Images sent from the mobile/web apps reach language_server as media[].inlineData, and it keeps the
// full original base64 in the trajectory step (desktop uploads only keep a small thumbnail plus the
// file uri). Every trajectory fetch, stream frame and cached session then carries megabytes of image
// data. Since the original is also saved at media[].uri (served by /api/v1/files/raw, which clients
// already prefer), we reshape such items like desktop ones: a gateway-made thumbnail plus the uri.

const (
	// inlineMediaCompactThreshold is the base64 length above which inline image data is replaced.
	inlineMediaCompactThreshold = 64 * 1024
	mediaThumbMaxDim            = 320
	mediaThumbJPEGQuality       = 70
	maxMediaThumbCacheEntries   = 128
)

var mediaThumbCache = struct {
	sync.Mutex
	m     map[string]string // key -> base64 JPEG ("" when the image could not be decoded)
	order []string
}{m: make(map[string]string)}

// compactTrajectoryMedia replaces large inline image payloads in user input steps. It mutates data,
// so it must run on a freshly decoded response before it is shared.
func compactTrajectoryMedia(data *upstreamTrajectoryResp) {
	if data == nil {
		return
	}
	for i := range data.Trajectory.Steps {
		ui := data.Trajectory.Steps[i].UserInput
		if ui == nil {
			continue
		}
		for j := range ui.Media {
			m := &ui.Media[j]
			// Without a uri the inline data is the only copy, so it has to stay.
			if m.URI == "" || len(m.InlineData) <= inlineMediaCompactThreshold {
				continue
			}
			if m.Thumbnail == "" {
				m.Thumbnail = cachedMediaThumbnail(m.URI+"#"+strconv.Itoa(len(m.InlineData)), m.InlineData)
			}
			m.InlineData = ""
		}
		for j := range ui.Images {
			img := &ui.Images[j]
			if len(img.Base64Data) <= inlineMediaCompactThreshold {
				continue
			}
			// Legacy images carry no uri; only swap in a thumbnail when one can be made.
			b64 := img.Base64Data
			key := "img#" + strconv.Itoa(len(b64)) + "#" + b64[:64] + b64[len(b64)-64:]
			if thumb := cachedMediaThumbnail(key, b64); thumb != "" {
				img.Base64Data = thumb
				img.MimeType = "image/jpeg"
			}
		}
	}
}

func cachedMediaThumbnail(key, b64 string) string {
	mediaThumbCache.Lock()
	thumb, ok := mediaThumbCache.m[key]
	mediaThumbCache.Unlock()
	if ok {
		return thumb
	}

	thumb = makeMediaThumbnail(b64)

	mediaThumbCache.Lock()
	if _, exists := mediaThumbCache.m[key]; !exists {
		mediaThumbCache.m[key] = thumb
		mediaThumbCache.order = append(mediaThumbCache.order, key)
		for len(mediaThumbCache.order) > maxMediaThumbCacheEntries {
			delete(mediaThumbCache.m, mediaThumbCache.order[0])
			mediaThumbCache.order = mediaThumbCache.order[1:]
		}
	}
	mediaThumbCache.Unlock()
	return thumb
}

// makeMediaThumbnail decodes a base64 image and returns a base64 JPEG no larger than
// mediaThumbMaxDim on either side, or "" if the image cannot be decoded.
func makeMediaThumbnail(b64 string) string {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		if raw, err = base64.RawStdEncoding.DecodeString(b64); err != nil {
			return ""
		}
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, downscaleImage(src, mediaThumbMaxDim), &jpeg.Options{Quality: mediaThumbJPEGQuality}); err != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// downscaleImage fits src into maxDim×maxDim, averaging a 2×2 sample grid per output pixel.
func downscaleImage(src image.Image, maxDim int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return src
	}
	dw, dh := sw, sh
	if sw > maxDim || sh > maxDim {
		if sw >= sh {
			dw, dh = maxDim, sh*maxDim/sw
		} else {
			dw, dh = sw*maxDim/sh, maxDim
		}
	}
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			var r, g, bl, a uint32
			for _, oy := range [2]int{1, 3} {
				for _, ox := range [2]int{1, 3} {
					sx := b.Min.X + (x*4+ox)*sw/(dw*4)
					sy := b.Min.Y + (y*4+oy)*sh/(dh*4)
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					r, g, bl, a = r+cr, g+cg, bl+cb, a+ca
				}
			}
			dst.Set(x, y, color.RGBA64{uint16(r / 4), uint16(g / 4), uint16(bl / 4), uint16(a / 4)})
		}
	}
	return dst
}
