package proxy

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"math/rand"
	"testing"
)

// noisyJPEGBase64 returns a w×h JPEG with random pixels (so it does not compress below the threshold).
func noisyJPEGBase64(t *testing.T, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	rng := rand.New(rand.NewSource(1))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(rng.Intn(256)), uint8(rng.Intn(256)), uint8(rng.Intn(256)), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func decodeThumbBounds(t *testing.T, b64 string) image.Rectangle {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("thumbnail is not base64: %v", err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("thumbnail is not a JPEG: %v", err)
	}
	return image.Rect(0, 0, cfg.Width, cfg.Height)
}

func trajectoryWithUserInput(t *testing.T, userInput string) *upstreamTrajectoryResp {
	t.Helper()
	var data upstreamTrajectoryResp
	body := `{"trajectory":{"steps":[{"type":"CORTEX_STEP_TYPE_USER_INPUT","userInput":` + userInput + `}]}}`
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatal(err)
	}
	return &data
}

func TestCompactTrajectoryMediaReplacesLargeInlineData(t *testing.T) {
	big := noisyJPEGBase64(t, 800, 600)
	if len(big) <= inlineMediaCompactThreshold {
		t.Fatalf("test image too small: %d", len(big))
	}
	bigNoURI := noisyJPEGBase64(t, 640, 640)
	small := noisyJPEGBase64(t, 40, 30)
	ui := map[string]any{
		"items": []any{map[string]any{"text": "look"}},
		"media": []any{
			map[string]any{"mimeType": "image/jpeg", "inlineData": big, "uri": "/tmp/brain/x/.user_uploaded/a.jpg"},
			map[string]any{"mimeType": "image/jpeg", "inlineData": bigNoURI}, // no uri: only copy, keep it
			map[string]any{"mimeType": "image/jpeg", "inlineData": small, "uri": "/tmp/brain/x/.user_uploaded/b.jpg"},
			map[string]any{"mimeType": "image/jpeg", "thumbnail": "desk", "uri": "/tmp/brain/x/c.jpg"},
		},
	}
	uiJSON, _ := json.Marshal(ui)
	data := trajectoryWithUserInput(t, string(uiJSON))

	compactTrajectoryMedia(data)
	media := data.Trajectory.Steps[0].UserInput.Media

	if media[0].InlineData != "" {
		t.Error("large inline data with a uri should be dropped")
	}
	if media[0].URI == "" {
		t.Error("uri must be kept so clients can load the original")
	}
	if r := decodeThumbBounds(t, media[0].Thumbnail); r.Dx() != mediaThumbMaxDim || r.Dy() != 240 {
		t.Errorf("thumbnail should fit %dpx keeping aspect, got %v", mediaThumbMaxDim, r)
	}
	if len(media[0].Thumbnail) > 64*1024 {
		t.Errorf("thumbnail unexpectedly large: %d bytes", len(media[0].Thumbnail))
	}
	if media[1].InlineData != bigNoURI || media[1].Thumbnail != "" {
		t.Error("inline data without a uri must be left untouched")
	}
	if media[2].InlineData != small {
		t.Error("small inline data should be left untouched")
	}
	if media[3].Thumbnail != "desk" {
		t.Error("existing desktop thumbnails must be left untouched")
	}

	// Parsed messages now carry the thumbnail plus the uri instead of the original.
	details := (&Proxy{}).ParseTrajectoryDetails(data)
	if len(details.AllMessages) == 0 {
		t.Fatal("expected a user message")
	}
	for _, m := range details.AllMessages[0].Media {
		if m == big {
			t.Error("message media still carries the full original of an image that has a uri")
		}
	}
}

func TestCompactTrajectoryMediaLegacyImages(t *testing.T) {
	big := noisyJPEGBase64(t, 800, 600)
	ui, _ := json.Marshal(map[string]any{
		"images": []any{
			map[string]any{"base64Data": big, "mimeType": "image/jpeg"},
			map[string]any{"base64Data": base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("not an image"), 8000))},
		},
	})
	data := trajectoryWithUserInput(t, string(ui))
	compactTrajectoryMedia(data)
	imgs := data.Trajectory.Steps[0].UserInput.Images
	if len(imgs[0].Base64Data) >= len(big) {
		t.Error("large legacy image should be replaced by a thumbnail")
	}
	decodeThumbBounds(t, imgs[0].Base64Data)
	if len(imgs[1].Base64Data) <= inlineMediaCompactThreshold {
		t.Error("undecodable legacy image data must be kept as is")
	}
}

func TestCachedMediaThumbnailReusesResult(t *testing.T) {
	big := noisyJPEGBase64(t, 400, 400)
	first := cachedMediaThumbnail("test-key-reuse", big)
	if first == "" {
		t.Fatal("expected a thumbnail")
	}
	// A different payload under the same key must come from the cache, not be re-decoded.
	if got := cachedMediaThumbnail("test-key-reuse", "!!not base64!!"); got != first {
		t.Error("expected cached thumbnail for a repeated key")
	}
}
