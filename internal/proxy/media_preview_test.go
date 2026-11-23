package proxy

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeNoisyPNG(t *testing.T, w, h int, alpha uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(42))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, color.NRGBA{
				R: uint8(rng.Intn(256)),
				G: uint8(rng.Intn(256)),
				B: uint8(rng.Intn(256)),
				A: alpha,
			})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeSolidPNG(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// injectEXIFOrientation inserts a minimal APP1 Exif segment carrying Orientation=orient right after JPEG SOI.
func injectEXIFOrientation(t *testing.T, rawJPEG []byte, orient uint16) []byte {
	t.Helper()
	if len(rawJPEG) < 2 || rawJPEG[0] != 0xFF || rawJPEG[1] != 0xD8 {
		t.Fatal("not a JPEG")
	}
	// Little-endian TIFF header (8B) + 1 IFD entry (2 + 12 + 4 = 18B) = 26B
	var tiff bytes.Buffer
	tiff.WriteString("II")
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0x002A))
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(8))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(1))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0x0112)) // Orientation tag
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(3))      // SHORT
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(1))      // count
	_ = binary.Write(&tiff, binary.LittleEndian, orient)
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(0))
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(0)) // next IFD

	payload := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	var out bytes.Buffer
	out.Write(rawJPEG[:2])
	out.Write([]byte{0xFF, 0xE1})
	_ = binary.Write(&out, binary.BigEndian, uint16(len(payload)+2))
	out.Write(payload)
	out.Write(rawJPEG[2:])
	return out.Bytes()
}

func TestHandleFileRawPreviewDualTier(t *testing.T) {
	resetMediaPreviewCacheForTest()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	// Large desktop-like 2400x1600 noisy PNG (~several MB)
	bigPNGPath := filepath.Join(tmpHome, "desktop_shot.png")
	bigPNG := makeNoisyPNG(t, 2400, 1600, 255)
	if len(bigPNG) < 1024*1024 {
		t.Fatalf("expected >1MB noisy PNG, got %d", len(bigPNG))
	}
	if err := os.WriteFile(bigPNGPath, bigPNG, 0644); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}

	// 1. Original (no preview param) -> untouched PNG
	reqOrig := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(bigPNGPath), nil)
	recOrig := httptest.NewRecorder()
	p.HandleFileRaw(recOrig, reqOrig)
	if recOrig.Code != http.StatusOK {
		t.Fatalf("orig status = %d", recOrig.Code)
	}
	if ct := recOrig.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("orig Content-Type = %q, want image/png", ct)
	}
	if recOrig.Body.Len() != len(bigPNG) {
		t.Fatalf("orig body len = %d, want %d", recOrig.Body.Len(), len(bigPNG))
	}

	// 2. Bubble tier (?preview=thumb) -> <=720px JPEG, well under 150KB
	reqThumb := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(bigPNGPath)+"&preview=thumb", nil)
	recThumb := httptest.NewRecorder()
	p.HandleFileRaw(recThumb, reqThumb)
	if recThumb.Code != http.StatusOK {
		t.Fatalf("thumb status = %d", recThumb.Code)
	}
	if ct := recThumb.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("thumb Content-Type = %q, want image/jpeg", ct)
	}
	thumbCfg, err := jpeg.DecodeConfig(bytes.NewReader(recThumb.Body.Bytes()))
	if err != nil {
		t.Fatalf("thumb not valid JPEG: %v", err)
	}
	if thumbCfg.Width != 720 || thumbCfg.Height != 480 {
		t.Fatalf("thumb dims = %dx%d, want 720x480", thumbCfg.Width, thumbCfg.Height)
	}
	if recThumb.Body.Len() >= len(bigPNG)/10 {
		t.Fatalf("thumb should be much smaller than original: %d vs %d", recThumb.Body.Len(), len(bigPNG))
	}
	thumbETag := recThumb.Header().Get("ETag")
	if !strings.Contains(thumbETag, "-p-thumb") {
		t.Fatalf("thumb ETag = %q, expected -p-thumb suffix", thumbETag)
	}

	// 3. Conditional GET with preview ETag -> 304 Not Modified
	req304 := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(bigPNGPath)+"&preview=thumb", nil)
	req304.Header.Set("If-None-Match", thumbETag)
	rec304 := httptest.NewRecorder()
	p.HandleFileRaw(rec304, req304)
	if rec304.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want 304", rec304.Code)
	}

	// 4. Fullscreen preview tier (?preview=1) -> 1600x1067 JPEG, few hundred KB
	reqFull := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(bigPNGPath)+"&preview=1", nil)
	recFull := httptest.NewRecorder()
	p.HandleFileRaw(recFull, reqFull)
	if recFull.Code != http.StatusOK {
		t.Fatalf("full preview status = %d", recFull.Code)
	}
	if ct := recFull.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("full preview Content-Type = %q, want image/jpeg", ct)
	}
	fullCfg, err := jpeg.DecodeConfig(bytes.NewReader(recFull.Body.Bytes()))
	if err != nil {
		t.Fatalf("full preview not valid JPEG: %v", err)
	}
	if fullCfg.Width != 1600 || fullCfg.Height != 1067 {
		t.Fatalf("full preview dims = %dx%d, want 1600x1067", fullCfg.Width, fullCfg.Height)
	}
	if recFull.Body.Len() > 800*1024 {
		t.Fatalf("full preview unexpectedly large: %d bytes", recFull.Body.Len())
	}
	if recFull.Header().Get("ETag") == thumbETag || recFull.Header().Get("ETag") == recOrig.Header().Get("ETag") {
		t.Fatalf("full preview ETag must differ from thumb and orig ETags")
	}
}

func TestHandleFileRawPreviewPassThroughSmallImage(t *testing.T) {
	resetMediaPreviewCacheForTest()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	// Mobile-uploaded 739x1600 JPEG under 350KB should pass through untouched on preview=1
	img := image.NewRGBA(image.Rect(0, 0, 739, 1600))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 70}); err != nil {
		t.Fatal(err)
	}
	smallPath := filepath.Join(tmpHome, "mobile_shot.jpg")
	if err := os.WriteFile(smallPath, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(smallPath)+"&preview=1", nil)
	rec := httptest.NewRecorder()
	p.HandleFileRaw(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), buf.Bytes()) {
		t.Fatalf("small in-bounds mobile JPEG should pass through untouched without re-encoding")
	}
}

func TestHandleFileRawPreviewTallScreenshotPreservesReadability(t *testing.T) {
	// 1290x6000 tall screenshot (ratio ~4.65)
	specThumb := imagePreviewSpecs[imagePreviewThumb]
	tw, th := computePreviewDimensions(1290, 6000, specThumb)
	if tw < 400 || tw > 480 {
		t.Fatalf("tall thumb width = %d (height=%d), expected readable ~400..480px", tw, th)
	}
	if int64(tw)*int64(th) > specThumb.maxTotalPixels+2000 {
		t.Fatalf("tall thumb pixels %dx%d exceed budget", tw, th)
	}

	specFull := imagePreviewSpecs[imagePreviewFull]
	fw, fh := computePreviewDimensions(1290, 6000, specFull)
	if fw < 800 || fw > 1080 {
		t.Fatalf("tall full preview width = %d (height=%d), expected readable ~800..1080px", fw, fh)
	}
	if int64(fw)*int64(fh) > specFull.maxTotalPixels+5000 {
		t.Fatalf("tall full preview pixels %dx%d exceed budget", fw, fh)
	}
}

func TestHandleFileRawPreviewTransparentPNGFlattensToWhite(t *testing.T) {
	resetMediaPreviewCacheForTest()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	// 2000x2000 fully transparent PNG -> resized & flattened to white, not black
	transPath := filepath.Join(tmpHome, "transparent.png")
	if err := os.WriteFile(transPath, makeSolidPNG(t, 2000, 2000, color.NRGBA{0, 0, 0, 0}), 0644); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(transPath)+"&preview=thumb", nil)
	rec := httptest.NewRecorder()
	p.HandleFileRaw(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	decoded, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	r, g, b, _ := decoded.At(100, 100).RGBA()
	if r < 0xF000 || g < 0xF000 || b < 0xF000 {
		t.Fatalf("transparent PNG should composite onto white, got RGB=(%d,%d,%d)", r>>8, g>>8, b>>8)
	}
}

func TestHandleFileRawPreviewEXIFOrientation(t *testing.T) {
	resetMediaPreviewCacheForTest()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	// Create 2000x1000 landscape JPEG with EXIF Orientation=6 (rotate 90° CW -> 1000x2000 portrait)
	img := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 2000; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x % 256), uint8(y % 256), 120, 255})
		}
	}
	var rawBuf bytes.Buffer
	if err := jpeg.Encode(&rawBuf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	oriented := injectEXIFOrientation(t, rawBuf.Bytes(), 6)
	exifPath := filepath.Join(tmpHome, "portrait_exif.jpg")
	if err := os.WriteFile(exifPath, oriented, 0644); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(exifPath)+"&preview=thumb", nil)
	rec := httptest.NewRecorder()
	p.HandleFileRaw(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatalf("decode config failed: %v", err)
	}
	if cfg.Width != 360 || cfg.Height != 720 {
		t.Fatalf("EXIF orientation=6 preview dims = %dx%d, want 360x720 portrait", cfg.Width, cfg.Height)
	}
}

func TestHandleFileRawPreviewPreservesAnimatedGIFAndSVG(t *testing.T) {
	resetMediaPreviewCacheForTest()
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	resetWorkspaceRootsForTest()
	t.Cleanup(resetWorkspaceRootsForTest)

	// 2-frame animated GIF > 80KB (large enough not to trigger small passthrough by size alone)
	pal := color.Palette{color.Black, color.White, color.RGBA{255, 0, 0, 255}}
	f1 := image.NewPaletted(image.Rect(0, 0, 900, 900), pal)
	f2 := image.NewPaletted(image.Rect(0, 0, 900, 900), pal)
	f2.SetColorIndex(10, 10, 2)
	var gifBuf bytes.Buffer
	if err := gif.EncodeAll(&gifBuf, &gif.GIF{
		Image: []*image.Paletted{f1, f2},
		Delay: []int{10, 10},
	}); err != nil {
		t.Fatal(err)
	}
	gifPath := filepath.Join(tmpHome, "anim.gif")
	if err := os.WriteFile(gifPath, gifBuf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}

	svgPath := filepath.Join(tmpHome, "diagram.svg")
	svgBytes := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100"></svg>`)
	if err := os.WriteFile(svgPath, svgBytes, 0644); err != nil {
		t.Fatal(err)
	}

	p := &Proxy{}

	reqGif := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(gifPath)+"&preview=thumb", nil)
	recGif := httptest.NewRecorder()
	p.HandleFileRaw(recGif, reqGif)
	if recGif.Header().Get("Content-Type") != "image/gif" {
		t.Fatalf("animated gif Content-Type = %q, want image/gif passthrough", recGif.Header().Get("Content-Type"))
	}

	reqSvg := httptest.NewRequest(http.MethodGet, "/api/v1/files/raw?uri="+url.QueryEscape(svgPath)+"&preview=thumb", nil)
	recSvg := httptest.NewRecorder()
	p.HandleFileRaw(recSvg, reqSvg)
	if recSvg.Header().Get("Content-Type") != "image/svg+xml" {
		t.Fatalf("svg Content-Type = %q, want image/svg+xml passthrough", recSvg.Header().Get("Content-Type"))
	}
}
