package auth

import (
	"strconv"
	"strings"
	"testing"

	"github.com/skip2/go-qrcode"
)

type ansiColor struct {
	kind string // "16" or "rgb" or ""
	code int    // ANSI 16-color code when kind=="16"
	r, g, b int
}

func (c ansiColor) isBlack() bool {
	if c.kind == "16" {
		return c.code == 30 || c.code == 40
	}
	if c.kind == "rgb" {
		return c.r <= 8 && c.g <= 8 && c.b <= 8
	}
	return false
}

func (c ansiColor) isWhite() bool {
	if c.kind == "16" {
		return c.code == 97 || c.code == 107
	}
	if c.kind == "rgb" {
		return c.r >= 247 && c.g >= 247 && c.b >= 247
	}
	return false
}

type decodedCell struct {
	ch     rune
	fg, bg ansiColor
}

// parseANSIHalfBlockCells parses each line into cells with their active (fg, bg) colors.
func parseANSIHalfBlockCells(t *testing.T, rendered string) [][]decodedCell {
	t.Helper()
	lines := strings.Split(strings.TrimSuffix(rendered, "\n"), "\n")
	var out [][]decodedCell
	for li, line := range lines {
		if !strings.HasSuffix(line, "\x1b[0m") {
			t.Fatalf("line %d missing trailing ANSI reset \\x1b[0m: %q", li, line)
		}
		var row []decodedCell
		var fg, bg ansiColor
		i := 0
		for i < len(line) {
			if line[i] == '\x1b' {
				end := strings.IndexByte(line[i:], 'm')
				if end < 0 {
					t.Fatalf("line %d has unterminated ANSI escape at %d", li, i)
				}
				codes := strings.Split(line[i+2:i+end], ";")
				for ci := 0; ci < len(codes); ci++ {
					switch codes[ci] {
					case "0":
						fg, bg = ansiColor{}, ansiColor{}
					case "30":
						fg = ansiColor{kind: "16", code: 30}
					case "97":
						fg = ansiColor{kind: "16", code: 97}
					case "40":
						bg = ansiColor{kind: "16", code: 40}
					case "107":
						bg = ansiColor{kind: "16", code: 107}
					case "38", "48":
						if ci+4 >= len(codes) || codes[ci+1] != "2" {
							t.Fatalf("line %d invalid TrueColor SGR sequence %v", li, codes[ci:])
						}
						r, errR := strconv.Atoi(codes[ci+2])
						g, errG := strconv.Atoi(codes[ci+3])
						b, errB := strconv.Atoi(codes[ci+4])
						if errR != nil || errG != nil || errB != nil {
							t.Fatalf("line %d invalid RGB values %v", li, codes[ci:ci+5])
						}
						col := ansiColor{kind: "rgb", r: r, g: g, b: b}
						if codes[ci] == "38" {
							fg = col
						} else {
							bg = col
						}
						ci += 4
					default:
						t.Fatalf("line %d unexpected SGR code %q", li, codes[ci])
					}
				}
				i += end + 1
				continue
			}

			rest := line[i:]
			if strings.HasPrefix(rest, " ") {
				row = append(row, decodedCell{ch: ' ', fg: fg, bg: bg})
				i++
			} else if strings.HasPrefix(rest, "▄") {
				row = append(row, decodedCell{ch: '▄', fg: fg, bg: bg})
				i += len("▄")
			} else {
				t.Fatalf("line %d unexpected character at byte %d: %q", li, i, rest)
			}
		}
		out = append(out, row)
	}
	return out
}

// decodeANSIHalfBlockQR parses ANSI half-block output back into a 2D pixel bitmap
// (true = black, false = white) so tests verify exact visual equivalence with qr.Bitmap().
func decodeANSIHalfBlockQR(t *testing.T, rendered string) [][]bool {
	t.Helper()
	cellRows := parseANSIHalfBlockCells(t, rendered)
	var out [][]bool
	for li, row := range cellRows {
		var topRow, botRow []bool
		for _, c := range row {
			switch c.ch {
			case ' ':
				switch {
				case c.bg.isBlack():
					topRow = append(topRow, true)
					botRow = append(botRow, true)
				case c.bg.isWhite():
					topRow = append(topRow, false)
					botRow = append(botRow, false)
				default:
					t.Fatalf("line %d space rendered with invalid bg=%+v", li, c.bg)
				}
			case '▄':
				switch {
				case c.bg.isBlack() && c.fg.isWhite():
					topRow = append(topRow, true)
					botRow = append(botRow, false)
				case c.bg.isWhite() && c.fg.isBlack():
					topRow = append(topRow, false)
					botRow = append(botRow, true)
				default:
					t.Fatalf("line %d '▄' rendered with invalid bg=%+v fg=%+v", li, c.bg, c.fg)
				}
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

	for _, distinctSpaceBG := range []bool{false, true} {
		for _, disableBorder := range []bool{false, true} {
			for _, uri := range uris {
				qr, err := qrcode.New(uri, qrcode.Medium)
				if err != nil {
					t.Fatalf("qrcode.New(%q): %v", uri, err)
				}
				qr.DisableBorder = disableBorder

				rendered := renderANSIHalfBlockQRWithPalette(qr.Bitmap(), distinctSpaceBG)
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
							t.Fatalf("pixel mismatch at (x=%d, y=%d) distinctSpaceBG=%v disableBorder=%v: got %v, want %v",
								x, y, distinctSpaceBG, disableBorder, decoded[y][x], orig[y][x])
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
}

func TestRenderANSIHalfBlockQR_BreaksConhostSpaceCoalescing(t *testing.T) {
	// Simulate the exact transitions that broke legacy Windows conhost.exe + NSimSun:
	// - Finder pattern row 0/1: BW ('▄' white on black) -> BB (' ' black)
	// - Right quiet zone edge:  WB ('▄' black on white) -> WW (' ' white)
	bits := [][]bool{
		{true, true, true, false, false}, // top:    B B B W W
		{true, false, true, true, false}, // bottom: B W B B W
	}
	// Resulting cells: [0]=BB, [1]=BW, [2]=BB, [3]=WB, [4]=WW

	for _, distinctSpaceBG := range []bool{false, true} {
		rendered := renderANSIHalfBlockQRWithPalette(bits, distinctSpaceBG)
		rows := parseANSIHalfBlockCells(t, rendered)
		if len(rows) != 1 || len(rows[0]) != 5 {
			t.Fatalf("distinctSpaceBG=%v: unexpected cell grid %+v", distinctSpaceBG, rows)
		}
		cells := rows[0]

		// BW -> BB must NOT share (fg, bg); otherwise conhost batches them into one ExtTextOutW
		// call and NSimSun's double-width '▄' erases the right border of all 3 finder patterns.
		if cells[1].fg == cells[2].fg && cells[1].bg == cells[2].bg {
			t.Fatalf("distinctSpaceBG=%v: BW and trailing BB share identical (fg,bg): %+v vs %+v",
				distinctSpaceBG, cells[1], cells[2])
		}
		// WB -> WW must NOT share (fg, bg); otherwise NSimSun's black '▄' leaks into quiet zone.
		if cells[3].fg == cells[4].fg && cells[3].bg == cells[4].bg {
			t.Fatalf("distinctSpaceBG=%v: WB and trailing WW share identical (fg,bg): %+v vs %+v",
				distinctSpaceBG, cells[3], cells[4])
		}

		// On Windows distinct-BG palette, background colors themselves must ALSO differ so even
		// background-only blank-space coalescing cannot merge '▄' with a trailing ' '.
		if distinctSpaceBG {
			if cells[1].bg == cells[2].bg {
				t.Fatalf("distinctSpaceBG=true: BW and trailing BB must have distinct bg colors: %+v vs %+v",
					cells[1].bg, cells[2].bg)
			}
			if cells[3].bg == cells[4].bg {
				t.Fatalf("distinctSpaceBG=true: WB and trailing WW must have distinct bg colors: %+v vs %+v",
					cells[3].bg, cells[4].bg)
			}
		}
	}
}

func TestIsLegacyCJKConsoleFontName(t *testing.T) {
	legacy := []string{"", "NSimSun", "nsimsun", "新宋体", "SimSun", "宋体", "MS Gothic", "ＭＳ ゴシック", "MingLiU", "GulimChe", "Terminal"}
	for _, f := range legacy {
		if !isLegacyCJKConsoleFontName(f) {
			t.Errorf("isLegacyCJKConsoleFontName(%q) = false, want true", f)
		}
	}
	modern := []string{"Consolas", "Cascadia Mono", "Cascadia Code", "Lucida Console", "JetBrains Mono"}
	for _, f := range modern {
		if isLegacyCJKConsoleFontName(f) {
			t.Errorf("isLegacyCJKConsoleFontName(%q) = true, want false", f)
		}
	}
}

func TestRenderANSIHalfBlockQR_AllFourCombinations(t *testing.T) {
	bits := [][]bool{
		{false, true, true, false},
		{false, true, false, true},
		{true, false, true, false}, // odd last row -> paired with white
	}
	for _, distinctSpaceBG := range []bool{false, true} {
		rendered := renderANSIHalfBlockQRWithPalette(bits, distinctSpaceBG)
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
					t.Fatalf("distinctSpaceBG=%v mismatch at (%d,%d): got %v want %v",
						distinctSpaceBG, x, y, decoded[y][x], want[y][x])
				}
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

	pal := qrPalette16
	if consoleUsesDistinctSpaceBG() {
		pal = qrPaletteDistinctSpaceBG
	}

	out := FormatPairingQRCode("r-e97c2daa.la1.jiuge.space", 443, "3a3949e219588f0c64650cfd1e20ae31", true, "192.168.31.148")
	if strings.ContainsRune(out, '▀') {
		t.Fatalf("FormatPairingQRCode output must not contain U+2580 ('▀')")
	}
	if !strings.Contains(out, pal.wb+"▄") || !strings.Contains(out, pal.bw+"▄") {
		t.Fatalf("expected ANSI half-block sequences in FormatPairingQRCode output")
	}
	if !strings.Contains(out, pal.ww+" ") || !strings.Contains(out, pal.bb+" ") {
		t.Fatalf("expected explicit ANSI space sequences in FormatPairingQRCode output")
	}

	rawOut := formatRawPairingQRCode("3a3949e219588f0c64650cfd1e20ae31", "agy://pair?code=3a3949e219588f0c64650cfd1e20ae31&host=r-e97c2daa.la1.jiuge.space&lan=192.168.31.148&platform=windows")
	if strings.ContainsRune(rawOut, '▀') {
		t.Fatalf("formatRawPairingQRCode output must not contain U+2580 ('▀')")
	}
	if !strings.Contains(rawOut, pal.wb+"▄") || !strings.Contains(rawOut, pal.bw+"▄") {
		t.Fatalf("expected ANSI half-block sequences in formatRawPairingQRCode output")
	}
	if !strings.Contains(rawOut, pal.ww+" ") || !strings.Contains(rawOut, pal.bb+" ") {
		t.Fatalf("expected explicit ANSI space sequences in formatRawPairingQRCode output")
	}
}
