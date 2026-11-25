package auth

import (
	"strings"
	"testing"

	"github.com/skip2/go-qrcode"
)

// decodeANSIHalfBlockQR parses ANSI half-block output back into a 2D pixel bitmap
// (true = black, false = white) so tests verify exact visual equivalence with qr.Bitmap().
func decodeANSIHalfBlockQR(t *testing.T, rendered string) [][]bool {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
	var out [][]bool
	for li, line := range lines {
		if !strings.HasSuffix(line, "\x1b[0m") {
			t.Fatalf("line %d missing trailing ANSI reset \\x1b[0m: %q", li, line)
		}
		var topRow, botRow []bool
		var fg, bg int // 30=black, 97=white, 40=black, 107=white
		i := 0
		for i < len(line) {
			if line[i] == '\x1b' {
				end := strings.IndexByte(line[i:], 'm')
				if end < 0 {
					t.Fatalf("line %d has unterminated ANSI escape at %d", li, i)
				}
				codes := strings.Split(line[i+2:i+end], ";")
				for _, c := range codes {
					switch c {
					case "0":
						fg, bg = 0, 0
					case "30":
						fg = 30
					case "97":
						fg = 97
					case "40":
						bg = 40
					case "107":
						bg = 107
					default:
						t.Fatalf("line %d unexpected SGR code %q", li, c)
					}
				}
				i += end + 1
				continue
			}

			// Decode next UTF-8 rune
			rest := line[i:]
			if strings.HasPrefix(rest, " ") {
				switch bg {
				case 40:
					topRow = append(topRow, true)
					botRow = append(botRow, true)
				case 107:
					topRow = append(topRow, false)
					botRow = append(botRow, false)
				default:
					t.Fatalf("line %d space rendered with invalid bg=%d", li, bg)
				}
				i++
			} else if strings.HasPrefix(rest, "▄") {
				switch {
				case bg == 40 && fg == 97:
					topRow = append(topRow, true)
					botRow = append(botRow, false)
				case bg == 107 && fg == 30:
					topRow = append(topRow, false)
					botRow = append(botRow, true)
				default:
					t.Fatalf("line %d '▄' rendered with invalid bg=%d fg=%d", li, bg, fg)
				}
				i += len("▄")
			} else {
				t.Fatalf("line %d unexpected character at byte %d: %q", li, i, rest)
			}
		}
		out = append(out, topRow, botRow)
	}
	return out
}

func TestRenderANSIHalfBlockQR_RoundTripAndNoUpperHalfBlock(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")

	uris := []string{
		"agy://pair?code=abc123&host=192.168.50.9&port=58900&platform=windows",
		"agy://pair?code=3a3949e219588f0c64650cfd1e20ae319a00ecaffc6ac39277798d1d7cdcc744&host=r-e97c2daa.la1.jiuge.space&lan=192.168.31.148&platform=windows&pk=BEjz1xwpQCV4TLnneMyI6hWLyEHzEc1num8K_bZ30123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJK",
	}

	for _, disableBorder := range []bool{false, true} {
		for _, uri := range uris {
			qr, err := qrcode.New(uri, qrcode.Medium)
			if err != nil {
				t.Fatalf("qrcode.New(%q): %v", uri, err)
			}
			qr.DisableBorder = disableBorder

			rendered := renderTerminalQRCode(qr)
			if strings.ContainsRune(rendered, '▀') {
				t.Fatalf("rendered QR must not contain U+2580 ('▀'), which is missing in GBK/NSimSun")
			}
			if strings.ContainsRune(rendered, '█') {
				t.Fatalf("rendered QR should use ANSI background space instead of '█'")
			}
			if !strings.ContainsRune(rendered, '▄') {
				t.Fatalf("expected rendered QR to use GBK-safe '▄' (U+2584)")
			}

			orig := qr.Bitmap()
			decoded := decodeANSIHalfBlockQR(t, rendered)

			wantRows := len(orig)
			if wantRows%2 == 1 {
				wantRows++
			}
			if len(decoded) != wantRows {
				t.Fatalf("decoded row count = %d, want %d (orig=%d)", len(decoded), wantRows, len(orig))
			}

			for y := 0; y < len(orig); y++ {
				if len(decoded[y]) != len(orig[y]) {
					t.Fatalf("row %d width = %d, want %d", y, len(decoded[y]), len(orig[y]))
				}
				for x := range orig[y] {
					if decoded[y][x] != orig[y][x] {
						t.Fatalf("pixel mismatch at (x=%d, y=%d) disableBorder=%v: got %v, want %v",
							x, y, disableBorder, decoded[y][x], orig[y][x])
					}
				}
			}

			// If original bitmap had an odd height, the virtual bottom half of the final terminal line
			// must be padded as white (quiet zone).
			if len(orig)%2 == 1 {
				padRow := decoded[len(decoded)-1]
				for x, isBlack := range padRow {
					if isBlack {
						t.Fatalf("odd-row bottom padding at x=%d should be white, got black", x)
					}
				}
			}
		}
	}
}

func TestRenderANSIHalfBlockQR_AllFourCombinations(t *testing.T) {
	bits := [][]bool{
		{false, true, true, false},
		{false, true, false, true},
		{true, false, true, false}, // odd last row -> paired with white
	}
	rendered := renderANSIHalfBlockQR(bits)
	if strings.ContainsRune(rendered, '▀') {
		t.Fatalf("unexpected U+2580 in %q", rendered)
	}
	decoded := decodeANSIHalfBlockQR(t, rendered)
	want := [][]bool{
		{false, true, true, false},
		{false, true, false, true},
		{true, false, true, false},
		{false, false, false, false},
	}
	if len(decoded) != len(want) {
		t.Fatalf("len(decoded)=%d, want %d", len(decoded), len(want))
	}
	for y := range want {
		for x := range want[y] {
			if decoded[y][x] != want[y][x] {
				t.Fatalf("mismatch at (%d,%d): got %v want %v", x, y, decoded[y][x], want[y][x])
			}
		}
	}
}

func TestRenderTerminalQRCode_DumbTermFallback(t *testing.T) {
	t.Setenv("TERM", "dumb")
	qr, err := qrcode.New("agy://pair?code=abc&host=127.0.0.1&port=58900", qrcode.Medium)
	if err != nil {
		t.Fatalf("qrcode.New: %v", err)
	}
	out := renderTerminalQRCode(qr)
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("TERM=dumb should not emit ANSI escape sequences")
	}
	if out != qr.ToSmallString(false) {
		t.Fatalf("TERM=dumb should fall back to ToSmallString(false)")
	}
}

func TestFormatPairingAndRawQRCode_NoUpperHalfBlock(t *testing.T) {
	t.Setenv("TERM", "xterm-256color")

	out := FormatPairingQRCode("r-e97c2daa.la1.jiuge.space", 443, "3a3949e219588f0c64650cfd1e20ae31", true, "192.168.31.148")
	if strings.ContainsRune(out, '▀') {
		t.Fatalf("FormatPairingQRCode output must not contain U+2580 ('▀')")
	}
	if !strings.Contains(out, "\x1b[107;30m▄") || !strings.Contains(out, "\x1b[40;97m▄") {
		t.Fatalf("expected ANSI half-block sequences in FormatPairingQRCode output")
	}

	rawOut := formatRawPairingQRCode("3a3949e219588f0c64650cfd1e20ae31", "agy://pair?code=3a3949e219588f0c64650cfd1e20ae31&host=r-e97c2daa.la1.jiuge.space&lan=192.168.31.148&platform=windows")
	if strings.ContainsRune(rawOut, '▀') {
		t.Fatalf("formatRawPairingQRCode output must not contain U+2580 ('▀')")
	}
	if !strings.Contains(rawOut, "\x1b[107;30m▄") || !strings.Contains(rawOut, "\x1b[40;97m▄") {
		t.Fatalf("expected ANSI half-block sequences in formatRawPairingQRCode output")
	}
}
