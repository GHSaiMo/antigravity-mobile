package cockpit

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
)

const (
	sqliteMagicHeader = "SQLite format 3\x00"
	walMagicLE        = 0x377f0682
	walMagicBE        = 0x377f0683
	maxSQLitePayload  = 16 << 20 // 16 MiB safety cap per row
	maxSQLitePages    = 1 << 20  // 1M pages guard
	maxOverflowHops   = 65536
)

type sqliteRow struct {
	RowID  int64
	Values []any // nil, int64, float64, []byte, string
}

type pureSQLiteDB struct {
	file       *os.File
	fileSize   int64
	pageSize   int
	usableSize int
	walPages   map[uint32][]byte
}

func openPureSQLiteDB(dbPath string) (*pureSQLiteDB, error) {
	f, err := os.Open(dbPath)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if st.IsDir() || st.Size() < 100 {
		_ = f.Close()
		return nil, errors.New("file too small to be sqlite3")
	}

	var hdr [100]byte
	if _, err := f.ReadAt(hdr[:], 0); err != nil {
		_ = f.Close()
		return nil, err
	}
	if string(hdr[:16]) != sqliteMagicHeader {
		_ = f.Close()
		return nil, errors.New("invalid sqlite3 magic header")
	}

	rawPageSize := int(binary.BigEndian.Uint16(hdr[16:18]))
	pageSize := rawPageSize
	if pageSize == 1 {
		pageSize = 65536
	}
	if pageSize < 512 || pageSize > 65536 || (pageSize&(pageSize-1)) != 0 {
		_ = f.Close()
		return nil, fmt.Errorf("invalid sqlite3 page size: %d", rawPageSize)
	}

	reserved := int(hdr[20])
	usableSize := pageSize - reserved
	if usableSize < 480 {
		_ = f.Close()
		return nil, fmt.Errorf("invalid sqlite3 usable size: %d", usableSize)
	}

	db := &pureSQLiteDB{
		file:       f,
		fileSize:   st.Size(),
		pageSize:   pageSize,
		usableSize: usableSize,
	}

	// Overlay committed frames from companion -wal file when present.
	if walPages := loadSQLiteWALPages(dbPath+"-wal", pageSize); len(walPages) > 0 {
		db.walPages = walPages
	}
	return db, nil
}

func (db *pureSQLiteDB) Close() {
	if db != nil && db.file != nil {
		_ = db.file.Close()
	}
}

func (db *pureSQLiteDB) readPage(pageNum uint32) ([]byte, error) {
	if pageNum == 0 || pageNum > maxSQLitePages {
		return nil, fmt.Errorf("invalid page number %d", pageNum)
	}
	if db.walPages != nil {
		if p, ok := db.walPages[pageNum]; ok {
			return p, nil
		}
	}
	offset := int64(pageNum-1) * int64(db.pageSize)
	if offset < 0 || offset+int64(db.pageSize) > db.fileSize {
		return nil, fmt.Errorf("page %d out of bounds (fileSize=%d)", pageNum, db.fileSize)
	}
	buf := make([]byte, db.pageSize)
	if _, err := db.file.ReadAt(buf, offset); err != nil {
		return nil, err
	}
	return buf, nil
}

func walChecksum(data []byte, bigEndian bool, s0, s1 uint32) (uint32, uint32) {
	for i := 0; i+8 <= len(data); i += 8 {
		var x0, x1 uint32
		if bigEndian {
			x0 = binary.BigEndian.Uint32(data[i : i+4])
			x1 = binary.BigEndian.Uint32(data[i+4 : i+8])
		} else {
			x0 = binary.LittleEndian.Uint32(data[i : i+4])
			x1 = binary.LittleEndian.Uint32(data[i+4 : i+8])
		}
		s0 += x0 + s1
		s1 += x1 + s0
	}
	return s0, s1
}

// loadSQLiteWALPages reads committed frames from a SQLite WAL file without acquiring locks.
func loadSQLiteWALPages(walPath string, expectedPageSize int) map[uint32][]byte {
	f, err := os.Open(walPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil || st.IsDir() || st.Size() < 32 {
		return nil
	}

	var hdr [32]byte
	if _, err := f.ReadAt(hdr[:], 0); err != nil {
		return nil
	}

	magic := binary.BigEndian.Uint32(hdr[0:4])
	var bigEndian bool
	switch magic {
	case walMagicLE:
		bigEndian = false
	case walMagicBE:
		bigEndian = true
	default:
		return nil
	}

	walPageSize := int(binary.BigEndian.Uint32(hdr[8:12]))
	if walPageSize != expectedPageSize {
		return nil
	}

	salt1 := binary.BigEndian.Uint32(hdr[16:20])
	salt2 := binary.BigEndian.Uint32(hdr[20:24])
	wantS0 := binary.BigEndian.Uint32(hdr[24:28])
	wantS1 := binary.BigEndian.Uint32(hdr[28:32])

	s0, s1 := walChecksum(hdr[:24], bigEndian, 0, 0)
	if s0 != wantS0 || s1 != wantS1 {
		return nil
	}

	frameSize := int64(24 + expectedPageSize)
	committed := make(map[uint32][]byte)
	pending := make(map[uint32][]byte)
	frameBuf := make([]byte, frameSize)

	for offset := int64(32); offset+frameSize <= st.Size(); offset += frameSize {
		if _, err := f.ReadAt(frameBuf, offset); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
			break
		}
		fSalt1 := binary.BigEndian.Uint32(frameBuf[8:12])
		fSalt2 := binary.BigEndian.Uint32(frameBuf[12:16])
		if fSalt1 != salt1 || fSalt2 != salt2 {
			break
		}

		fCksum0 := binary.BigEndian.Uint32(frameBuf[16:20])
		fCksum1 := binary.BigEndian.Uint32(frameBuf[20:24])
		s0, s1 = walChecksum(frameBuf[0:8], bigEndian, s0, s1)
		s0, s1 = walChecksum(frameBuf[24:], bigEndian, s0, s1)
		if s0 != fCksum0 || s1 != fCksum1 {
			break
		}

		pageNum := binary.BigEndian.Uint32(frameBuf[0:4])
		dbSizeAfterCommit := binary.BigEndian.Uint32(frameBuf[4:8])
		pageCopy := make([]byte, expectedPageSize)
		copy(pageCopy, frameBuf[24:])
		pending[pageNum] = pageCopy

		if dbSizeAfterCommit > 0 {
			for pNum, pBytes := range pending {
				committed[pNum] = pBytes
			}
			clear(pending)
		}
	}

	if len(committed) == 0 {
		return nil
	}
	return committed
}

// readSQLiteVarint decodes a big-endian SQLite 1..9 byte varint.
func readSQLiteVarint(b []byte, pos int) (uint64, int, bool) {
	if pos < 0 || pos >= len(b) {
		return 0, pos, false
	}
	var v uint64
	for i := 0; i < 8; i++ {
		if pos >= len(b) {
			return 0, pos, false
		}
		byt := b[pos]
		pos++
		v = (v << 7) | uint64(byt&0x7f)
		if byt&0x80 == 0 {
			return v, pos, true
		}
	}
	if pos >= len(b) {
		return 0, pos, false
	}
	v = (v << 8) | uint64(b[pos])
	pos++
	return v, pos, true
}

func (db *pureSQLiteDB) readCellPayload(page []byte, pos int, payloadLen uint64) ([]byte, error) {
	if payloadLen > maxSQLitePayload {
		return nil, fmt.Errorf("sqlite payload too large: %d", payloadLen)
	}
	pLen := int(payloadLen)
	u := db.usableSize
	x := u - 35
	m := ((u - 12) * 32 / 255) - 23

	var inlineLen int
	var hasOverflow bool
	if pLen <= x {
		inlineLen = pLen
	} else {
		k := m + ((pLen - m) % (u - 4))
		if k <= x {
			inlineLen = k
		} else {
			inlineLen = m
		}
		hasOverflow = true
	}

	if pos < 0 || pos+inlineLen > len(page) {
		return nil, errors.New("inline payload out of page bounds")
	}

	if !hasOverflow {
		out := make([]byte, inlineLen)
		copy(out, page[pos:pos+inlineLen])
		return out, nil
	}

	if pos+inlineLen+4 > len(page) {
		return nil, errors.New("overflow pointer out of page bounds")
	}
	out := make([]byte, 0, pLen)
	out = append(out, page[pos:pos+inlineLen]...)
	nextPage := binary.BigEndian.Uint32(page[pos+inlineLen : pos+inlineLen+4])

	remaining := pLen - inlineLen
	chunkCap := u - 4
	visited := make(map[uint32]bool)
	for hops := 0; remaining > 0 && nextPage != 0; hops++ {
		if hops >= maxOverflowHops || visited[nextPage] {
			return nil, errors.New("overflow page cycle or hop limit exceeded")
		}
		visited[nextPage] = true
		ovPage, err := db.readPage(nextPage)
		if err != nil {
			return nil, err
		}
		if len(ovPage) < 4+chunkCap {
			return nil, errors.New("overflow page truncated")
		}
		nextPage = binary.BigEndian.Uint32(ovPage[0:4])
		take := chunkCap
		if take > remaining {
			take = remaining
		}
		out = append(out, ovPage[4:4+take]...)
		remaining -= take
	}
	if remaining != 0 {
		return nil, errors.New("incomplete overflow payload chain")
	}
	return out, nil
}

func decodeSQLiteRecord(payload []byte) ([]any, error) {
	hdrSizeU64, pos, ok := readSQLiteVarint(payload, 0)
	if !ok || hdrSizeU64 > uint64(len(payload)) || hdrSizeU64 < uint64(pos) {
		return nil, errors.New("invalid sqlite record header size")
	}
	hdrEnd := int(hdrSizeU64)
	var serialTypes []uint64
	for pos < hdrEnd {
		st, next, ok := readSQLiteVarint(payload, pos)
		if !ok || next > hdrEnd {
			return nil, errors.New("invalid sqlite serial type")
		}
		serialTypes = append(serialTypes, st)
		pos = next
	}

	vals := make([]any, len(serialTypes))
	valPos := hdrEnd
	for i, st := range serialTypes {
		switch st {
		case 0:
			vals[i] = nil
		case 1:
			if valPos+1 > len(payload) {
				return nil, errors.New("truncated int8")
			}
			vals[i] = int64(int8(payload[valPos]))
			valPos++
		case 2:
			if valPos+2 > len(payload) {
				return nil, errors.New("truncated int16")
			}
			vals[i] = int64(int16(binary.BigEndian.Uint16(payload[valPos : valPos+2])))
			valPos += 2
		case 3:
			if valPos+3 > len(payload) {
				return nil, errors.New("truncated int24")
			}
			v := int32(payload[valPos])<<16 | int32(payload[valPos+1])<<8 | int32(payload[valPos+2])
			if payload[valPos]&0x80 != 0 {
				v |= ^0xffffff
			}
			vals[i] = int64(v)
			valPos += 3
		case 4:
			if valPos+4 > len(payload) {
				return nil, errors.New("truncated int32")
			}
			vals[i] = int64(int32(binary.BigEndian.Uint32(payload[valPos : valPos+4])))
			valPos += 4
		case 5:
			if valPos+6 > len(payload) {
				return nil, errors.New("truncated int48")
			}
			var u uint64
			for j := 0; j < 6; j++ {
				u = (u << 8) | uint64(payload[valPos+j])
			}
			if payload[valPos]&0x80 != 0 {
				u |= ^uint64(0xffffffffffff)
			}
			vals[i] = int64(u)
			valPos += 6
		case 6:
			if valPos+8 > len(payload) {
				return nil, errors.New("truncated int64")
			}
			vals[i] = int64(binary.BigEndian.Uint64(payload[valPos : valPos+8]))
			valPos += 8
		case 7:
			if valPos+8 > len(payload) {
				return nil, errors.New("truncated float64")
			}
			vals[i] = math.Float64frombits(binary.BigEndian.Uint64(payload[valPos : valPos+8]))
			valPos += 8
		case 8:
			vals[i] = int64(0)
		case 9:
			vals[i] = int64(1)
		case 10, 11:
			return nil, fmt.Errorf("reserved serial type %d", st)
		default:
			if st >= 12 {
				n := int((st - 12) / 2)
				if n < 0 || valPos+n > len(payload) {
					return nil, errors.New("truncated blob/text column")
				}
				if st%2 == 0 {
					b := make([]byte, n)
					copy(b, payload[valPos:valPos+n])
					vals[i] = b
				} else {
					vals[i] = string(payload[valPos : valPos+n])
				}
				valPos += n
			}
		}
	}
	return vals, nil
}

func (db *pureSQLiteDB) walkTableBTree(pageNum uint32, visited map[uint32]bool, onRow func(sqliteRow) error) error {
	if visited[pageNum] {
		return errors.New("sqlite btree cycle detected")
	}
	visited[pageNum] = true

	page, err := db.readPage(pageNum)
	if err != nil {
		return err
	}

	hdrOff := 0
	if pageNum == 1 {
		hdrOff = 100
	}
	if hdrOff+8 > len(page) {
		return errors.New("page header out of bounds")
	}

	pageType := page[hdrOff]
	cellCount := int(binary.BigEndian.Uint16(page[hdrOff+3 : hdrOff+5]))

	switch pageType {
	case 0x05: // Table B-Tree Interior Page
		if hdrOff+12+cellCount*2 > len(page) {
			return errors.New("interior page cell array out of bounds")
		}
		rightMost := binary.BigEndian.Uint32(page[hdrOff+8 : hdrOff+12])
		ptrBase := hdrOff + 12
		for i := 0; i < cellCount; i++ {
			cellOff := int(binary.BigEndian.Uint16(page[ptrBase+i*2 : ptrBase+i*2+2]))
			if cellOff+4 > len(page) {
				return errors.New("interior cell out of bounds")
			}
			leftChild := binary.BigEndian.Uint32(page[cellOff : cellOff+4])
			if err := db.walkTableBTree(leftChild, visited, onRow); err != nil {
				return err
			}
		}
		return db.walkTableBTree(rightMost, visited, onRow)

	case 0x0d: // Table B-Tree Leaf Page
		if hdrOff+8+cellCount*2 > len(page) {
			return errors.New("leaf page cell array out of bounds")
		}
		ptrBase := hdrOff + 8
		for i := 0; i < cellCount; i++ {
			cellOff := int(binary.BigEndian.Uint16(page[ptrBase+i*2 : ptrBase+i*2+2]))
			payloadLen, p1, ok := readSQLiteVarint(page, cellOff)
			if !ok {
				return errors.New("invalid leaf cell payload length")
			}
			rowIDU64, p2, ok := readSQLiteVarint(page, p1)
			if !ok {
				return errors.New("invalid leaf cell rowid")
			}
			payload, err := db.readCellPayload(page, p2, payloadLen)
			if err != nil {
				return err
			}
			vals, err := decodeSQLiteRecord(payload)
			if err != nil {
				return err
			}
			if err := onRow(sqliteRow{RowID: int64(rowIDU64), Values: vals}); err != nil {
				return err
			}
		}
		return nil

	default:
		return fmt.Errorf("unexpected table btree page type 0x%02x on page %d", pageType, pageNum)
	}
}

func (db *pureSQLiteDB) findTableSchema(tableName string) (rootPage uint32, createSQL string, found bool, err error) {
	visited := make(map[uint32]bool)
	err = db.walkTableBTree(1, visited, func(row sqliteRow) error {
		if len(row.Values) < 5 {
			return nil
		}
		objType, _ := row.Values[0].(string)
		name, _ := row.Values[1].(string)
		if objType != "table" || !strings.EqualFold(name, tableName) {
			return nil
		}
		if rp, ok := row.Values[3].(int64); ok && rp > 0 {
			rootPage = uint32(rp)
			createSQL, _ = row.Values[4].(string)
			found = true
		}
		return nil
	})
	return rootPage, createSQL, found, err
}

// parseSQLiteTableColumnIndex resolves a 0-based column index from a CREATE TABLE statement.
func parseSQLiteTableColumnIndex(createSQL, targetCol string) int {
	openIdx := strings.IndexByte(createSQL, '(')
	closeIdx := strings.LastIndexByte(createSQL, ')')
	if openIdx < 0 || closeIdx <= openIdx {
		return -1
	}
	body := createSQL[openIdx+1 : closeIdx]

	var cols []string
	depth := 0
	inQuote := byte(0)
	start := 0
	for i := 0; i < len(body); i++ {
		c := body[i]
		if inQuote != 0 {
			if c == inQuote {
				inQuote = 0
			}
			continue
		}
		switch c {
		case '\'', '"', '`':
			inQuote = c
		case '[':
			inQuote = ']'
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				cols = append(cols, strings.TrimSpace(body[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(body[start:]); tail != "" {
		cols = append(cols, tail)
	}

	colIdx := 0
	for _, def := range cols {
		if def == "" {
			continue
		}
		fields := strings.Fields(strings.ToUpper(def))
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "PRIMARY", "FOREIGN", "UNIQUE", "CHECK", "CONSTRAINT":
			continue
		}
		name := extractFirstSQLIdentifier(def)
		if strings.EqualFold(name, targetCol) {
			return colIdx
		}
		colIdx++
	}
	return -1
}

func extractFirstSQLIdentifier(def string) string {
	s := strings.TrimSpace(def)
	if s == "" {
		return ""
	}
	switch s[0] {
	case '`', '"', '\'':
		q := s[0]
		if end := strings.IndexByte(s[1:], q); end >= 0 {
			return s[1 : 1+end]
		}
	case '[':
		if end := strings.IndexByte(s[1:], ']'); end >= 0 {
			return s[1 : 1+end]
		}
	}
	end := strings.IndexAny(s, " \t\r\n(")
	if end >= 0 {
		return s[:end]
	}
	return s
}

// readGenMetadataBlobsPureGo reads SELECT data FROM gen_metadata ORDER BY idx without sqlite3/python.
func readGenMetadataBlobsPureGo(dbPath string) ([][]byte, error) {
	db, err := openPureSQLiteDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rootPage, createSQL, found, err := db.findTableSchema("gen_metadata")
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	dataColIdx := parseSQLiteTableColumnIndex(createSQL, "data")
	if dataColIdx < 0 {
		dataColIdx = 1 // fallback to canonical (idx, data, size) layout
	}

	var blobs [][]byte
	visited := make(map[uint32]bool)
	err = db.walkTableBTree(rootPage, visited, func(row sqliteRow) error {
		if dataColIdx >= len(row.Values) {
			return nil
		}
		switch v := row.Values[dataColIdx].(type) {
		case []byte:
			if len(v) > 0 {
				blobs = append(blobs, bytes.Clone(v))
			}
		case string:
			if len(v) > 0 {
				blobs = append(blobs, []byte(v))
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return blobs, nil
}

// readSummaryParentsPureGo reads child->parent conversation links from conversation_summaries.db without sqlite3/python.
func readSummaryParentsPureGo(dbPath string) (map[string]string, error) {
	db, err := openPureSQLiteDB(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	result := make(map[string]string)
	readPairs := func(table, cidCol, pidCol string) {
		rootPage, createSQL, found, sErr := db.findTableSchema(table)
		if sErr != nil || !found {
			return
		}
		cIdx := parseSQLiteTableColumnIndex(createSQL, cidCol)
		pIdx := parseSQLiteTableColumnIndex(createSQL, pidCol)
		if cIdx < 0 || pIdx < 0 {
			return
		}
		visited := make(map[uint32]bool)
		_ = db.walkTableBTree(rootPage, visited, func(row sqliteRow) error {
			if cIdx >= len(row.Values) || pIdx >= len(row.Values) {
				return nil
			}
			cid, _ := row.Values[cIdx].(string)
			pid, _ := row.Values[pIdx].(string)
			cid = strings.TrimSpace(cid)
			pid = strings.TrimSpace(pid)
			if !isSafeConvID(cid) || !isSafeConvID(pid) || cid == pid {
				return nil
			}
			if _, exists := result[cid]; !exists {
				result[cid] = pid
			}
			return nil
		})
	}

	// Preserve COALESCE(conversation_parents, conversation_summaries) precedence.
	readPairs("conversation_parents", "conversation_id", "parent_id")
	readPairs("conversation_summaries", "conversation_id", "parent_conversation_id")
	return result, nil
}
