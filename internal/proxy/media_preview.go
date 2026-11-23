package proxy

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"strings"
	"sync"
)

type imagePreviewMode string

const (
	imagePreviewNone  imagePreviewMode = ""
	imagePreviewThumb imagePreviewMode = "thumb"
	imagePreviewFull  imagePreviewMode = "hd"
)

type imagePreviewSpec struct {
	maxLongSide         int
	tallRatioThreshold  float64
	tallMaxShortSide    int
	maxTotalPixels      int64
	passThroughMaxBytes int64
	primaryQuality      int
	fallbackQuality     int
	softMaxBytes        int
}

var imagePreviewSpecs = map[imagePreviewMode]imagePreviewSpec{
	imagePreviewThumb: {
		maxLongSide:         720,
		tallRatioThreshold:  2.5,
		tallMaxShortSide:    480,
		maxTotalPixels:      1_200_000,
		passThroughMaxBytes: 80 * 1024,
		primaryQuality:      78,
		fallbackQuality:     68,
		softMaxBytes:        140 * 1024,
	},
	imagePreviewFull: {
		maxLongSide:         1600,
		tallRatioThreshold:  2.5,
		tallMaxShortSide:    1080,
		maxTotalPixels:      4_000_000,
		passThroughMaxBytes: 350 * 1024,
		primaryQuality:      82,
		fallbackQuality:     72,
		softMaxBytes:        500 * 1024,
	},
}

const (
	maxPreviewSourceBytes   = 30 * 1024 * 1024 // 30 MiB ceiling before falling back to passthrough
	maxPreviewDecodePixels  = 36 * 1000 * 1000 // 36 MP ceiling against decompression bombs
	maxPreviewCacheEntries  = 96
	maxPreviewCacheBytes    = 24 * 1024 * 1024 // 24 MiB total RAM budget for preview cache
	maxPreviewCacheItemSize = 1 * 1024 * 1024  // do not cache unusually huge single items
)

type mediaPreviewEntry struct {
	data        []byte
	passthrough bool
}

type mediaPreviewFlight struct {
	done  chan struct{}
	entry mediaPreviewEntry
}

var mediaPreviewCache = struct {
	sync.Mutex
	m          map[string]mediaPreviewEntry
	order      []string
	totalBytes int
	flights    map[string]*mediaPreviewFlight
}{
	m:       make(map[string]mediaPreviewEntry, maxPreviewCacheEntries),
	flights: make(map[string]*mediaPreviewFlight),
}

// parseImagePreviewMode maps client query values to a fixed allowlisted preview tier.
// Unknown values return imagePreviewNone so arbitrary query strings cannot bust cache or DoS CPU.
func parseImagePreviewMode(raw string) imagePreviewMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "thumb", "bubble":
		return imagePreviewThumb
	case "1", "true", "hd", "full", "preview":
		return imagePreviewFull
	default:
		return imagePreviewNone
	}
}

func isTranscodablePreviewExt(ext string) bool {
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif":
		return true
	default:
		return false
	}
}

// computePreviewDimensions returns target (dw, dh) never upscaling src.
// Normal aspect ratios fit within maxLongSide; very tall/wide screenshots (ratio > 2.5)
// preserve readability by capping the short side plus a total pixel budget.
func computePreviewDimensions(sw, sh int, spec imagePreviewSpec) (int, int) {
	if sw <= 0 || sh <= 0 {
		return sw, sh
	}
	shortSide := sw
	longSide := sh
	if sw > sh {
		shortSide = sh
		longSide = sw
	}

	var scale float64 = 1.0
	if float64(longSide)/float64(shortSide) <= spec.tallRatioThreshold {
		if longSide > spec.maxLongSide {
			scale = float64(spec.maxLongSide) / float64(longSide)
		}
	} else {
		if shortSide > spec.tallMaxShortSide {
			scale = float64(spec.tallMaxShortSide) / float64(shortSide)
		}
		scaledPixels := float64(sw) * scale * float64(sh) * scale
		if spec.maxTotalPixels > 0 && scaledPixels > float64(spec.maxTotalPixels) {
			scale = math.Sqrt(float64(spec.maxTotalPixels) / (float64(sw) * float64(sh)))
		}
	}
	if scale >= 1.0 {
		return sw, sh
	}
	dw := int(math.Round(float64(sw) * scale))
	dh := int(math.Round(float64(sh) * scale))
	if dw < 1 {
		dw = 1
	}
	if dh < 1 {
		dh = 1
	}
	if dw > sw {
		dw = sw
	}
	if dh > sh {
		dh = sh
	}
	return dw, dh
}

// getOrBuildImagePreview returns transcoded JPEG preview bytes for a local image file,
// or (nil, false) when the caller should transparently serve the original file instead.
func getOrBuildImagePreview(filePath, ext string, fi os.FileInfo, f *os.File, mode imagePreviewMode) ([]byte, bool) {
	spec, ok := imagePreviewSpecs[mode]
	if !ok || !isTranscodablePreviewExt(ext) {
		return nil, false
	}
	if fi.Size() <= 0 || fi.Size() > maxPreviewSourceBytes {
		return nil, false
	}

	cacheKey := fmt.Sprintf("%s|%d|%d|%s", filePath, fi.ModTime().UnixNano(), fi.Size(), mode)

	mediaPreviewCache.Lock()
	if entry, hit := mediaPreviewCache.m[cacheKey]; hit {
		mediaPreviewCache.Unlock()
		if entry.passthrough || len(entry.data) == 0 {
			return nil, false
		}
		return entry.data, true
	}
	if fl, inFlight := mediaPreviewCache.flights[cacheKey]; inFlight {
		mediaPreviewCache.Unlock()
		<-fl.done
		if fl.entry.passthrough || len(fl.entry.data) == 0 {
			return nil, false
		}
		return fl.entry.data, true
	}
	fl := &mediaPreviewFlight{done: make(chan struct{})}
	mediaPreviewCache.flights[cacheKey] = fl
	mediaPreviewCache.Unlock()

	mediaThumbSem <- struct{}{}
	entry := buildImagePreviewFromFile(ext, fi.Size(), f, spec)
	<-mediaThumbSem

	mediaPreviewCache.Lock()
	fl.entry = entry
	delete(mediaPreviewCache.flights, cacheKey)
	if entry.passthrough || (len(entry.data) > 0 && len(entry.data) <= maxPreviewCacheItemSize) {
		putPreviewCacheLocked(cacheKey, entry)
	}
	mediaPreviewCache.Unlock()
	close(fl.done)

	if entry.passthrough || len(entry.data) == 0 {
		return nil, false
	}
	return entry.data, true
}

func putPreviewCacheLocked(key string, entry mediaPreviewEntry) {
	if _, exists := mediaPreviewCache.m[key]; exists {
		return
	}
	itemSize := len(entry.data)
	for len(mediaPreviewCache.order) >= maxPreviewCacheEntries ||
		(mediaPreviewCache.totalBytes+itemSize > maxPreviewCacheBytes && len(mediaPreviewCache.order) > 0) {
		evictKey := mediaPreviewCache.order[0]
		mediaPreviewCache.order = mediaPreviewCache.order[1:]
		if old, ok := mediaPreviewCache.m[evictKey]; ok {
			mediaPreviewCache.totalBytes -= len(old.data)
			delete(mediaPreviewCache.m, evictKey)
		}
	}
	mediaPreviewCache.m[key] = entry
	mediaPreviewCache.order = append(mediaPreviewCache.order, key)
	mediaPreviewCache.totalBytes += itemSize
}

func buildImagePreviewFromFile(ext string, fileSize int64, f *os.File, spec imagePreviewSpec) mediaPreviewEntry {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return mediaPreviewEntry{passthrough: true}
	}

	// 1. Decode image header only to verify dimensions and guard against decompression bombs.
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxPreviewDecodePixels {
		return mediaPreviewEntry{passthrough: true}
	}

	// 2. Preserve animated GIFs untouched.
	if format == "gif" || strings.EqualFold(ext, ".gif") {
		if _, err := f.Seek(0, io.SeekStart); err != nil || isAnimatedGIF(f) {
			return mediaPreviewEntry{passthrough: true}
		}
	}

	// 3. Read EXIF orientation for JPEG files so rotated photos stay upright after re-encoding.
	orient := 1
	if format == "jpeg" || strings.EqualFold(ext, ".jpg") || strings.EqualFold(ext, ".jpeg") {
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			orient = readJPEGEXIFOrientation(f)
		}
	}

	effW, effH := cfg.Width, cfg.Height
	if orient >= 5 && orient <= 8 {
		effW, effH = effH, effW
	}
	dw, dh := computePreviewDimensions(effW, effH, spec)

	// 4. Small already-within-bounds images pass through with zero re-encoding loss or CPU cost.
	if fileSize <= spec.passThroughMaxBytes && dw == effW && dh == effH {
		return mediaPreviewEntry{passthrough: true}
	}

	// 5. Decode full image.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return mediaPreviewEntry{passthrough: true}
	}
	src, _, err := image.Decode(f)
	if err != nil {
		return mediaPreviewEntry{passthrough: true}
	}
	if orient > 1 && orient <= 8 {
		src = applyEXIFOrientation(src, orient)
	}

	outImg := downscaleImageArea(src, dw, dh)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, outImg, &jpeg.Options{Quality: spec.primaryQuality}); err != nil {
		return mediaPreviewEntry{passthrough: true}
	}
	if spec.softMaxBytes > 0 && buf.Len() > spec.softMaxBytes && spec.fallbackQuality > 0 && spec.fallbackQuality < spec.primaryQuality {
		var smaller bytes.Buffer
		if err := jpeg.Encode(&smaller, outImg, &jpeg.Options{Quality: spec.fallbackQuality}); err == nil && smaller.Len() < buf.Len() {
			buf = smaller
		}
	}

	// If re-encoding did not resize and didn't meaningfully shrink the file, serve original instead.
	if dw == effW && dh == effH && int64(buf.Len()) >= fileSize {
		return mediaPreviewEntry{passthrough: true}
	}

	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return mediaPreviewEntry{data: out}
}

// downscaleImageArea resizes src to dw×dh using a separable 2-pass area-averaging box filter
// and composites alpha onto an opaque white background for crisp text & transparent PNG safety.
func downscaleImageArea(src image.Image, dw, dh int) image.Image {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return src
	}
	if dw <= 0 {
		dw = 1
	}
	if dh <= 0 {
		dh = 1
	}

	// Fast path: same dimensions — only flatten alpha onto white into RGBA.
	if dw == sw && dh == sh {
		dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
		pix := dst.Pix
		stride := dst.Stride
		for y := 0; y < sh; y++ {
			rowOff := y * stride
			sy := b.Min.Y + y
			for x := 0; x < sw; x++ {
				r, g, bl, a := src.At(b.Min.X+x, sy).RGBA()
				invA := uint32(0xffff) - a
				i := rowOff + x*4
				pix[i+0] = uint8(min(r+invA, uint32(0xffff)) >> 8)
				pix[i+1] = uint8(min(g+invA, uint32(0xffff)) >> 8)
				pix[i+2] = uint8(min(bl+invA, uint32(0xffff)) >> 8)
				pix[i+3] = 255
			}
		}
		return dst
	}

	// Pass 1: Horizontal area-average from sw×sh -> dw×sh (flattened onto white, 3 float32/pixel).
	horiz := make([]float32, sh*dw*3)
	xScale := float64(sw) / float64(dw)

	type span struct {
		start, end int
		wFirst     float32
		wLast      float32
		invTotal   float32
	}
	xSpans := make([]span, dw)
	for dx := 0; dx < dw; dx++ {
		left := float64(dx) * xScale
		right := float64(dx+1) * xScale
		s := int(math.Floor(left))
		e := int(math.Ceil(right)) - 1
		if s < 0 {
			s = 0
		}
		if e >= sw {
			e = sw - 1
		}
		if e < s {
			e = s
		}
		wFirst := float32(float64(s+1) - left)
		wLast := float32(right - float64(e))
		if s == e {
			wFirst = float32(right - left)
			wLast = 0
		}
		total := float32(right - left)
		if total <= 0 {
			total = 1
		}
		xSpans[dx] = span{start: s, end: e, wFirst: wFirst, wLast: wLast, invTotal: 1.0 / total}
	}

	for y := 0; y < sh; y++ {
		sy := b.Min.Y + y
		rowBase := y * dw * 3
		for dx := 0; dx < dw; dx++ {
			sp := xSpans[dx]
			var sr, sg, sb float32
			for sx := sp.start; sx <= sp.end; sx++ {
				w := float32(1.0)
				if sx == sp.start {
					w = sp.wFirst
				} else if sx == sp.end {
					w = sp.wLast
				}
				r, g, bl, a := src.At(b.Min.X+sx, sy).RGBA()
				invA := uint32(0xffff) - a
				sr += float32(min(r+invA, uint32(0xffff))) * w
				sg += float32(min(g+invA, uint32(0xffff))) * w
				sb += float32(min(bl+invA, uint32(0xffff))) * w
			}
			off := rowBase + dx*3
			horiz[off+0] = sr * sp.invTotal
			horiz[off+1] = sg * sp.invTotal
			horiz[off+2] = sb * sp.invTotal
		}
	}

	// Pass 2: Vertical area-average from dw×sh -> dw×dh into destination RGBA.
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	pix := dst.Pix
	stride := dst.Stride
	yScale := float64(sh) / float64(dh)

	for dy := 0; dy < dh; dy++ {
		top := float64(dy) * yScale
		bottom := float64(dy+1) * yScale
		s := int(math.Floor(top))
		e := int(math.Ceil(bottom)) - 1
		if s < 0 {
			s = 0
		}
		if e >= sh {
			e = sh - 1
		}
		if e < s {
			e = s
		}
		wFirst := float32(float64(s+1) - top)
		wLast := float32(bottom - float64(e))
		if s == e {
			wFirst = float32(bottom - top)
			wLast = 0
		}
		total := float32(bottom - top)
		if total <= 0 {
			total = 1
		}
		invTotal := 1.0 / total
		rowOff := dy * stride

		for dx := 0; dx < dw; dx++ {
			var sr, sg, sb float32
			for sy := s; sy <= e; sy++ {
				w := float32(1.0)
				if sy == s {
					w = wFirst
				} else if sy == e {
					w = wLast
				}
				off := (sy*dw + dx) * 3
				sr += horiz[off+0] * w
				sg += horiz[off+1] * w
				sb += horiz[off+2] * w
			}
			i := rowOff + dx*4
			pix[i+0] = clampUint8From16(sr * invTotal)
			pix[i+1] = clampUint8From16(sg * invTotal)
			pix[i+2] = clampUint8From16(sb * invTotal)
			pix[i+3] = 255
		}
	}
	return dst
}

func clampUint8From16(v float32) uint8 {
	x := int(v/256.0 + 0.5)
	if x < 0 {
		return 0
	}
	if x > 255 {
		return 255
	}
	return uint8(x)
}

// isAnimatedGIF scans GIF block headers without decoding pixels and returns true if >=2 image frames exist.
func isAnimatedGIF(r io.Reader) bool {
	br := bufio.NewReader(io.LimitReader(r, 8*1024*1024))
	var header [13]byte
	if _, err := io.ReadFull(br, header[:]); err != nil {
		return false
	}
	if !bytes.HasPrefix(header[:6], []byte("GIF87a")) && !bytes.HasPrefix(header[:6], []byte("GIF89a")) {
		return false
	}
	packed := header[10]
	if packed&0x80 != 0 {
		gctSize := 3 * (1 << ((packed & 0x07) + 1))
		if _, err := io.CopyN(io.Discard, br, int64(gctSize)); err != nil {
			return false
		}
	}

	frames := 0
	for {
		b, err := br.ReadByte()
		if err != nil || b == 0x3B { // Trailer
			return false
		}
		switch b {
		case 0x2C: // Image Descriptor
			frames++
			if frames > 1 {
				return true
			}
			var desc [9]byte
			if _, err := io.ReadFull(br, desc[:]); err != nil {
				return false
			}
			if desc[8]&0x80 != 0 {
				lctSize := 3 * (1 << ((desc[8] & 0x07) + 1))
				if _, err := io.CopyN(io.Discard, br, int64(lctSize)); err != nil {
					return false
				}
			}
			// LZW minimum code size byte
			if _, err := br.ReadByte(); err != nil {
				return false
			}
			if !skipGIFSubBlocks(br) {
				return false
			}
		case 0x21: // Extension block
			if _, err := br.ReadByte(); err != nil { // label
				return false
			}
			if !skipGIFSubBlocks(br) {
				return false
			}
		default:
			return false
		}
	}
}

func skipGIFSubBlocks(br *bufio.Reader) bool {
	for {
		n, err := br.ReadByte()
		if err != nil {
			return false
		}
		if n == 0 {
			return true
		}
		if _, err := io.CopyN(io.Discard, br, int64(n)); err != nil {
			return false
		}
	}
}

// readJPEGEXIFOrientation parses the EXIF Orientation tag (1..8) from JPEG APP1 metadata.
// Returns 1 when absent or unreadable.
func readJPEGEXIFOrientation(r io.Reader) int {
	br := bufio.NewReader(io.LimitReader(r, 128*1024))
	var soi [2]byte
	if _, err := io.ReadFull(br, soi[:]); err != nil || soi[0] != 0xFF || soi[1] != 0xD8 {
		return 1
	}

	for i := 0; i < 64; i++ {
		b, err := br.ReadByte()
		if err != nil {
			return 1
		}
		for b != 0xFF {
			b, err = br.ReadByte()
			if err != nil {
				return 1
			}
		}
		marker, err := br.ReadByte()
		for err == nil && marker == 0xFF {
			marker, err = br.ReadByte()
		}
		if err != nil || marker == 0xDA || marker == 0xD9 { // SOS or EOI
			return 1
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}
		var lenBuf [2]byte
		if _, err := io.ReadFull(br, lenBuf[:]); err != nil {
			return 1
		}
		segLen := int(binary.BigEndian.Uint16(lenBuf[:]))
		if segLen < 2 {
			return 1
		}
		payloadLen := segLen - 2
		if marker == 0xE1 && payloadLen >= 14 {
			payload := make([]byte, payloadLen)
			if _, err := io.ReadFull(br, payload); err != nil {
				return 1
			}
			if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				return parseTIFFOrientation(payload[6:])
			}
			continue
		}
		if _, err := io.CopyN(io.Discard, br, int64(payloadLen)); err != nil {
			return 1
		}
	}
	return 1
}

func parseTIFFOrientation(tiff []byte) int {
	if len(tiff) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch {
	case tiff[0] == 'I' && tiff[1] == 'I':
		bo = binary.LittleEndian
	case tiff[0] == 'M' && tiff[1] == 'M':
		bo = binary.BigEndian
	default:
		return 1
	}
	if bo.Uint16(tiff[2:4]) != 0x002A {
		return 1
	}
	ifdOff := int(bo.Uint32(tiff[4:8]))
	if ifdOff < 8 || ifdOff+2 > len(tiff) {
		return 1
	}
	numEntries := int(bo.Uint16(tiff[ifdOff : ifdOff+2]))
	pos := ifdOff + 2
	for i := 0; i < numEntries; i++ {
		if pos+12 > len(tiff) {
			break
		}
		tag := bo.Uint16(tiff[pos : pos+2])
		if tag == 0x0112 { // Orientation
			val := int(bo.Uint16(tiff[pos+8 : pos+10]))
			if val >= 1 && val <= 8 {
				return val
			}
			return 1
		}
		pos += 12
	}
	return 1
}

// applyEXIFOrientation rotates/flips src according to EXIF Orientation values 2..8.
func applyEXIFOrientation(src image.Image, orient int) image.Image {
	if orient <= 1 || orient > 8 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if orient >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := src.At(b.Min.X+x, b.Min.Y+y)
			var dx, dy int
			switch orient {
			case 2: // flip horizontal
				dx, dy = w-1-x, y
			case 3: // rotate 180
				dx, dy = w-1-x, h-1-y
			case 4: // flip vertical
				dx, dy = x, h-1-y
			case 5: // transpose
				dx, dy = y, x
			case 6: // rotate 90 CW
				dx, dy = h-1-y, x
			case 7: // transverse
				dx, dy = h-1-y, w-1-x
			case 8: // rotate 270 CW
				dx, dy = y, w-1-x
			}
			dst.Set(dx, dy, c)
		}
	}
	return dst
}

// resetMediaPreviewCacheForTest clears cached preview entries in tests.
func resetMediaPreviewCacheForTest() {
	mediaPreviewCache.Lock()
	mediaPreviewCache.m = make(map[string]mediaPreviewEntry, maxPreviewCacheEntries)
	mediaPreviewCache.order = nil
	mediaPreviewCache.totalBytes = 0
	mediaPreviewCache.flights = make(map[string]*mediaPreviewFlight)
	mediaPreviewCache.Unlock()
}
