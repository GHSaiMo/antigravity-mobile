package proxy

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif" // register decoders for image.Decode
	"image/jpeg"
	_ "image/png"
	"io"
	"strconv"
	"strings"
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
	maxMediaDecodePixels        = 20 * 1000 * 1000 // 20 MP ceiling against decompression bombs
)

type mediaThumbFlight struct {
	done  chan struct{}
	thumb string
}

var (
	mediaThumbSem   = make(chan struct{}, 2) // Cap concurrent image decodes across goroutines
	mediaThumbCache = struct {
		sync.Mutex
		m       map[string]string // key -> base64 JPEG ("" when the image could not be decoded)
		ring    [maxMediaThumbCacheEntries]string
		head    int
		size    int
		flights map[string]*mediaThumbFlight
	}{
		m:       make(map[string]string, maxMediaThumbCacheEntries),
		flights: make(map[string]*mediaThumbFlight),
	}
)

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
	if thumb, ok := mediaThumbCache.m[key]; ok {
		mediaThumbCache.Unlock()
		return thumb
	}
	if fl, inFlight := mediaThumbCache.flights[key]; inFlight {
		mediaThumbCache.Unlock()
		<-fl.done
		return fl.thumb
	}
	fl := &mediaThumbFlight{done: make(chan struct{})}
	mediaThumbCache.flights[key] = fl
	mediaThumbCache.Unlock()

	mediaThumbSem <- struct{}{}
	thumb := makeMediaThumbnail(b64)
	<-mediaThumbSem

	mediaThumbCache.Lock()
	fl.thumb = thumb
	delete(mediaThumbCache.flights, key)
	if _, exists := mediaThumbCache.m[key]; !exists {
		if mediaThumbCache.size == maxMediaThumbCacheEntries {
			evictKey := mediaThumbCache.ring[mediaThumbCache.head]
			delete(mediaThumbCache.m, evictKey)
			mediaThumbCache.ring[mediaThumbCache.head] = key
			mediaThumbCache.head = (mediaThumbCache.head + 1) % maxMediaThumbCacheEntries
		} else {
			idx := (mediaThumbCache.head + mediaThumbCache.size) % maxMediaThumbCacheEntries
			mediaThumbCache.ring[idx] = key
			mediaThumbCache.size++
		}
		mediaThumbCache.m[key] = thumb
	}
	mediaThumbCache.Unlock()
	close(fl.done)
	return thumb
}

func newBase64Reader(b64 string) io.Reader {
	trimmed := strings.TrimRight(b64, "\r\n\t ")
	if len(trimmed)%4 != 0 && !strings.HasSuffix(trimmed, "=") {
		return base64.NewDecoder(base64.RawStdEncoding, strings.NewReader(trimmed))
	}
	return base64.NewDecoder(base64.StdEncoding, strings.NewReader(trimmed))
}

// makeMediaThumbnail decodes a base64 image and returns a base64 JPEG no larger than
// mediaThumbMaxDim on either side, or "" if the image cannot be decoded or exceeds maxMediaDecodePixels.
func makeMediaThumbnail(b64 string) string {
	// 1. Stream header only through DecodeConfig to block decompression bombs without allocating raw []byte
	cfg, _, err := image.DecodeConfig(newBase64Reader(b64))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxMediaDecodePixels {
		return ""
	}
	// 2. Stream full image decode without intermediate raw []byte copy
	src, _, err := image.Decode(newBase64Reader(b64))
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
	pix := dst.Pix
	stride := dst.Stride
	for y := 0; y < dh; y++ {
		rowOff := y * stride
		for x := 0; x < dw; x++ {
			var r, g, bl, a uint32
			for _, oy := range [2]int{1, 3} {
				sy := b.Min.Y + (y*4+oy)*sh/(dh*4)
				for _, ox := range [2]int{1, 3} {
					sx := b.Min.X + (x*4+ox)*sw/(dw*4)
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					r, g, bl, a = r+cr, g+cg, bl+cb, a+ca
				}
			}
			// Average of 4 16-bit premultiplied RGBA samples -> 8-bit: (sum / 4) >> 8 == sum >> 10
			i := rowOff + x*4
			pix[i+0] = uint8(r >> 10)
			pix[i+1] = uint8(g >> 10)
			pix[i+2] = uint8(bl >> 10)
			pix[i+3] = uint8(a >> 10)
		}
	}
	return dst
}
